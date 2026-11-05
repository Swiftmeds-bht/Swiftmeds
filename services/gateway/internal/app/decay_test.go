package app

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	testSecret = "test-worker-secret-0123456789"
	validBody  = `{"asOf":"2026-11-05T07:00:00.123Z"}`
)

type fakeApplier struct {
	updated     int
	err         error
	calls       int
	gotAsOf     time.Time
	hadDeadline bool
}

func (f *fakeApplier) ApplyDecay(ctx context.Context, asOf time.Time) (int, error) {
	f.calls++
	f.gotAsOf = asOf
	_, f.hadDeadline = ctx.Deadline()
	return f.updated, f.err
}

func newDecayServer(t *testing.T, secret string, applier *fakeApplier) *httptest.Server {
	t.Helper()
	cfg := Config{CORSOrigins: defaultCORSOrigins, WorkerSecret: secret}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	server := httptest.NewServer(NewRouter(cfg, logger, WithFreshness(applier)))
	t.Cleanup(server.Close)
	return server
}

// postDecay sends the request. An empty secret means the header is left out.
func postDecay(t *testing.T, server *httptest.Server, secret, body string) (int, string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, server.URL+DecayPath, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	if secret != "" {
		req.Header.Set(WorkerSecretHeader, secret)
	}
	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, string(respBody)
}

func TestDecayAcceptsTheSharedSecret(t *testing.T) {
	applier := &fakeApplier{updated: 3}
	server := newDecayServer(t, testSecret, applier)

	code, body := postDecay(t, server, testSecret, validBody)

	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", code, body)
	}
	var got map[string]int
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatalf("body %q is not JSON: %v", body, err)
	}
	if got["updatedCount"] != 3 {
		t.Errorf("updatedCount = %d, want 3 (body %s)", got["updatedCount"], body)
	}
	if applier.calls != 1 {
		t.Errorf("ApplyDecay calls = %d, want 1", applier.calls)
	}
	if !applier.hadDeadline {
		t.Error("ApplyDecay context has no deadline, want a timeout")
	}
}

func TestDecayReturnsZeroWhenNothingChanged(t *testing.T) {
	server := newDecayServer(t, testSecret, &fakeApplier{updated: 0})

	code, body := postDecay(t, server, testSecret, validBody)

	if code != http.StatusOK || !strings.Contains(body, `"updatedCount":0`) {
		t.Errorf("got %d %s, want 200 with updatedCount 0", code, body)
	}
}

// "Gateway rejects calls without the secret": none of these may reach Freshness,
// and a bad body must not matter because the secret is checked first.
func TestDecayRejectsCallsWithoutTheSecret(t *testing.T) {
	cases := []struct{ name, secret, body string }{
		{"no header", "", validBody},
		{"wrong secret", "wrong-secret-0123456789", validBody},
		{"secret plus one char", testSecret + "x", validBody},
		{"secret minus one char", testSecret[:len(testSecret)-1], validBody},
		{"no header and a bad body", "", "not json"},
		{"wrong secret and no body", "wrong-secret-0123456789", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			applier := &fakeApplier{updated: 3}
			server := newDecayServer(t, testSecret, applier)

			code, body := postDecay(t, server, tc.secret, tc.body)

			if code != http.StatusUnauthorized {
				t.Errorf("status = %d, want 401", code)
			}
			if applier.calls != 0 {
				t.Errorf("ApplyDecay calls = %d, want 0", applier.calls)
			}
			if strings.Contains(body, testSecret) {
				t.Errorf("body %q leaks the secret", body)
			}
		})
	}
}

func TestDecayIgnoresTheAuthorizationHeader(t *testing.T) {
	applier := &fakeApplier{}
	server := newDecayServer(t, testSecret, applier)
	req, err := http.NewRequest(http.MethodPost, server.URL+DecayPath, strings.NewReader(validBody))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+testSecret)
	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized || applier.calls != 0 {
		t.Errorf("status = %d, calls = %d, want 401 and 0: a user token header must not unlock this route", resp.StatusCode, applier.calls)
	}
}

func TestDecayRejectsEverythingWhenNoSecretIsConfigured(t *testing.T) {
	for _, secret := range []string{"", "anything-at-all-123456"} {
		applier := &fakeApplier{}
		server := newDecayServer(t, "", applier)

		code, _ := postDecay(t, server, secret, validBody)

		if code != http.StatusUnauthorized {
			t.Errorf("header %q: status = %d, want 401", secret, code)
		}
		if applier.calls != 0 {
			t.Errorf("header %q: ApplyDecay calls = %d, want 0", secret, applier.calls)
		}
	}
}

func TestDecayValidatesAsOf(t *testing.T) {
	cases := []struct{ name, body string }{
		{"empty body", ""},
		{"not JSON", "not json"},
		{"empty object", `{}`},
		{"empty asOf", `{"asOf":""}`},
		{"blank asOf", `{"asOf":"   "}`},
		{"asOf is not a string", `{"asOf":123}`},
		{"asOf is not a timestamp", `{"asOf":"yesterday"}`},
		{"date without time", `{"asOf":"2026-11-05"}`},
		{"timestamp without a zone", `{"asOf":"2026-11-05T07:00:00"}`},
		{"zero time", `{"asOf":"0001-01-01T00:00:00Z"}`},
		{"body too large", `{"asOf":"` + strings.Repeat("x", maxDecayBytes) + `"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			applier := &fakeApplier{}
			server := newDecayServer(t, testSecret, applier)

			code, body := postDecay(t, server, testSecret, tc.body)

			if code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400 (body %s)", code, body)
			}
			if applier.calls != 0 {
				t.Errorf("ApplyDecay calls = %d, want 0", applier.calls)
			}
			if !strings.Contains(body, `"error"`) {
				t.Errorf("body %q is not a JSON error", body)
			}
		})
	}
}

func TestDecayPassesAsOfToFreshnessInUTC(t *testing.T) {
	cases := []struct {
		name string
		body string
		want time.Time
	}{
		{"UTC with milliseconds", `{"asOf":"2026-11-05T07:00:00.123Z"}`, time.Date(2026, 11, 5, 7, 0, 0, 123000000, time.UTC)},
		{"Bhutan time zone", `{"asOf":"2026-11-05T07:00:00+06:00"}`, time.Date(2026, 11, 5, 1, 0, 0, 0, time.UTC)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			applier := &fakeApplier{}
			server := newDecayServer(t, testSecret, applier)

			code, body := postDecay(t, server, testSecret, tc.body)

			if code != http.StatusOK {
				t.Fatalf("status = %d, want 200 (body %s)", code, body)
			}
			if !applier.gotAsOf.Equal(tc.want) {
				t.Errorf("asOf = %v, want %v", applier.gotAsOf, tc.want)
			}
			if applier.gotAsOf.Location() != time.UTC {
				t.Errorf("asOf location = %v, want UTC", applier.gotAsOf.Location())
			}
		})
	}
}

func TestDecayMapsFreshnessErrorsWithoutLeakingThem(t *testing.T) {
	cases := []struct {
		name    string
		err     error
		want    int
		message string
	}{
		{"unavailable", status.Error(codes.Unavailable, "dial freshness:50069 refused"), http.StatusServiceUnavailable, "service unavailable"},
		{"deadline exceeded", status.Error(codes.DeadlineExceeded, "too slow"), http.StatusGatewayTimeout, "request timed out"},
		{"internal", status.Error(codes.Internal, "pq: password authentication failed"), http.StatusInternalServerError, "internal error"},
		{"plain error", errors.New("boom"), http.StatusInternalServerError, "internal error"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := newDecayServer(t, testSecret, &fakeApplier{err: tc.err})

			code, body := postDecay(t, server, testSecret, validBody)

			if code != tc.want {
				t.Errorf("status = %d, want %d", code, tc.want)
			}
			var got map[string]string
			if err := json.Unmarshal([]byte(body), &got); err != nil {
				t.Fatalf("body %q is not JSON: %v", body, err)
			}
			if got["error"] != tc.message {
				t.Errorf("error = %q, want %q", got["error"], tc.message)
			}
		})
	}
}

func TestErrorMessage(t *testing.T) {
	cases := []struct {
		status int
		want   string
	}{
		{http.StatusInternalServerError, "internal error"},
		{http.StatusServiceUnavailable, "service unavailable"},
		{http.StatusGatewayTimeout, "request timed out"},
		{http.StatusConflict, "conflict"},
		{http.StatusBadRequest, "bad request"},
	}
	for _, tc := range cases {
		if got := errorMessage(tc.status); got != tc.want {
			t.Errorf("errorMessage(%d) = %q, want %q", tc.status, got, tc.want)
		}
	}
}

func TestDecayRouteIsAbsentWithoutAFreshnessClient(t *testing.T) {
	server := httptest.NewServer(NewRouter(Config{CORSOrigins: defaultCORSOrigins, WorkerSecret: testSecret}, slog.Default()))
	t.Cleanup(server.Close)

	code, _ := postDecay(t, server, testSecret, validBody)

	if code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", code)
	}
}

func TestDecayIsPostOnly(t *testing.T) {
	applier := &fakeApplier{}
	server := newDecayServer(t, testSecret, applier)

	req, err := http.NewRequest(http.MethodGet, server.URL+DecayPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set(WorkerSecretHeader, testSecret)
	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		t.Error("GET returned 200, want it not to trigger a decay")
	}
	if applier.calls != 0 {
		t.Errorf("ApplyDecay calls = %d, want 0", applier.calls)
	}
}

func TestSecretMatches(t *testing.T) {
	cases := []struct {
		name      string
		want, got string
		match     bool
	}{
		{"same", "abcdefghijklmnop", "abcdefghijklmnop", true},
		{"different", "abcdefghijklmnop", "abcdefghijklmnoq", false},
		{"different length", "abcdefghijklmnop", "abcdefghijklmno", false},
		{"both empty", "", "", false},
		{"empty secret", "", "abcdefghijklmnop", false},
		{"empty token", "abcdefghijklmnop", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := secretMatches(tc.want, tc.got); got != tc.match {
				t.Errorf("secretMatches(%q, %q) = %v, want %v", tc.want, tc.got, got, tc.match)
			}
		})
	}
}
