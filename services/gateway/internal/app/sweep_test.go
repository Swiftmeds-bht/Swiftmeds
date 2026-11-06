package app

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	holdA = "11111111-1111-4111-8111-111111111111"
	holdB = "22222222-2222-4222-8222-222222222222"
)

type fakeSweeper struct {
	expired     []string
	err         error
	calls       int
	gotAsOf     time.Time
	hadDeadline bool
}

func (f *fakeSweeper) RunSweep(ctx context.Context, asOf time.Time) ([]string, error) {
	f.calls++
	f.gotAsOf = asOf
	_, f.hadDeadline = ctx.Deadline()
	return f.expired, f.err
}

func newSweepServer(t *testing.T, secret string, sweeper *fakeSweeper) *httptest.Server {
	t.Helper()
	cfg := Config{CORSOrigins: defaultCORSOrigins, WorkerSecret: secret}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	server := httptest.NewServer(NewRouter(cfg, logger, WithExpiry(sweeper)))
	t.Cleanup(server.Close)
	return server
}

// postSweep sends the request. An empty secret means the header is left out.
func postSweep(t *testing.T, server *httptest.Server, secret, body string) (int, string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, server.URL+SweepPath, strings.NewReader(body))
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

func TestSweepAcceptsTheSharedSecret(t *testing.T) {
	sweeper := &fakeSweeper{expired: []string{holdA, holdB}}
	server := newSweepServer(t, testSecret, sweeper)

	code, body := postSweep(t, server, testSecret, validBody)

	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", code, body)
	}
	var got map[string][]string
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatalf("body %q is not JSON: %v", body, err)
	}
	if !reflect.DeepEqual(got["expiredHoldIds"], []string{holdA, holdB}) {
		t.Errorf("expiredHoldIds = %v, want [%s %s]", got["expiredHoldIds"], holdA, holdB)
	}
	if sweeper.calls != 1 {
		t.Errorf("RunSweep calls = %d, want 1", sweeper.calls)
	}
	if !sweeper.hadDeadline {
		t.Error("RunSweep context has no deadline, want a timeout")
	}
}

func TestSweepReturnsAnEmptyListNotNull(t *testing.T) {
	for _, expired := range [][]string{nil, {}} {
		server := newSweepServer(t, testSecret, &fakeSweeper{expired: expired})

		code, body := postSweep(t, server, testSecret, validBody)

		if code != http.StatusOK || !strings.Contains(body, `"expiredHoldIds":[]`) {
			t.Errorf("got %d %s, want 200 with an empty list", code, body)
		}
	}
}

// "Gateway rejects calls without the secret": none of these may reach Expiry,
// and a bad body must not matter because the secret is checked first.
func TestSweepRejectsCallsWithoutTheSecret(t *testing.T) {
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
			sweeper := &fakeSweeper{expired: []string{holdA}}
			server := newSweepServer(t, testSecret, sweeper)

			code, body := postSweep(t, server, tc.secret, tc.body)

			if code != http.StatusUnauthorized {
				t.Errorf("status = %d, want 401", code)
			}
			if sweeper.calls != 0 {
				t.Errorf("RunSweep calls = %d, want 0", sweeper.calls)
			}
			if strings.Contains(body, testSecret) {
				t.Errorf("body %q leaks the secret", body)
			}
		})
	}
}

func TestSweepIgnoresTheAuthorizationHeader(t *testing.T) {
	sweeper := &fakeSweeper{}
	server := newSweepServer(t, testSecret, sweeper)
	req, err := http.NewRequest(http.MethodPost, server.URL+SweepPath, strings.NewReader(validBody))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+testSecret)
	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized || sweeper.calls != 0 {
		t.Errorf("status = %d, calls = %d, want 401 and 0: a user token header must not unlock this route", resp.StatusCode, sweeper.calls)
	}
}

func TestSweepRejectsEverythingWhenNoSecretIsConfigured(t *testing.T) {
	for _, secret := range []string{"", "anything-at-all-123456"} {
		sweeper := &fakeSweeper{}
		server := newSweepServer(t, "", sweeper)

		code, _ := postSweep(t, server, secret, validBody)

		if code != http.StatusUnauthorized {
			t.Errorf("header %q: status = %d, want 401", secret, code)
		}
		if sweeper.calls != 0 {
			t.Errorf("header %q: RunSweep calls = %d, want 0", secret, sweeper.calls)
		}
	}
}

func TestSweepValidatesAsOf(t *testing.T) {
	cases := []struct{ name, body string }{
		{"empty body", ""},
		{"not JSON", "not json"},
		{"empty object", `{}`},
		{"empty asOf", `{"asOf":""}`},
		{"asOf is not a string", `{"asOf":123}`},
		{"asOf is not a timestamp", `{"asOf":"yesterday"}`},
		{"timestamp without a zone", `{"asOf":"2026-11-05T07:00:00"}`},
		{"zero time", `{"asOf":"0001-01-01T00:00:00Z"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sweeper := &fakeSweeper{}
			server := newSweepServer(t, testSecret, sweeper)

			code, body := postSweep(t, server, testSecret, tc.body)

			if code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400 (body %s)", code, body)
			}
			if sweeper.calls != 0 {
				t.Errorf("RunSweep calls = %d, want 0", sweeper.calls)
			}
			if !strings.Contains(body, `"error"`) {
				t.Errorf("body %q is not a JSON error", body)
			}
		})
	}
}

func TestSweepPassesAsOfToExpiryInUTC(t *testing.T) {
	sweeper := &fakeSweeper{}
	server := newSweepServer(t, testSecret, sweeper)

	code, body := postSweep(t, server, testSecret, `{"asOf":"2026-11-05T07:00:00+06:00"}`)

	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", code, body)
	}
	want := time.Date(2026, 11, 5, 1, 0, 0, 0, time.UTC)
	if !sweeper.gotAsOf.Equal(want) || sweeper.gotAsOf.Location() != time.UTC {
		t.Errorf("asOf = %v, want %v in UTC", sweeper.gotAsOf, want)
	}
}

func TestSweepMapsExpiryErrorsWithoutLeakingThem(t *testing.T) {
	cases := []struct {
		name    string
		err     error
		want    int
		message string
	}{
		{"unavailable", status.Error(codes.Unavailable, "dial expiry:50066 refused"), http.StatusServiceUnavailable, "service unavailable"},
		{"deadline exceeded", status.Error(codes.DeadlineExceeded, "too slow"), http.StatusGatewayTimeout, "request timed out"},
		{"internal", status.Error(codes.Internal, "pq: password authentication failed"), http.StatusInternalServerError, "internal error"},
		{"plain error", errors.New("boom"), http.StatusInternalServerError, "internal error"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := newSweepServer(t, testSecret, &fakeSweeper{err: tc.err})

			code, body := postSweep(t, server, testSecret, validBody)

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

func TestSweepRouteIsAbsentWithoutAnExpiryClient(t *testing.T) {
	server := httptest.NewServer(NewRouter(Config{CORSOrigins: defaultCORSOrigins, WorkerSecret: testSecret}, slog.Default()))
	t.Cleanup(server.Close)

	code, _ := postSweep(t, server, testSecret, validBody)

	if code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", code)
	}
}

func TestSweepIsPostOnly(t *testing.T) {
	sweeper := &fakeSweeper{}
	server := newSweepServer(t, testSecret, sweeper)

	req, err := http.NewRequest(http.MethodGet, server.URL+SweepPath, nil)
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
		t.Error("GET returned 200, want it not to trigger a sweep")
	}
	if sweeper.calls != 0 {
		t.Errorf("RunSweep calls = %d, want 0", sweeper.calls)
	}
}
