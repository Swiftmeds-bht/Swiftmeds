package httpx

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRecoverReturnsJSON500AndLogs(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	panicking := http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("boom") })

	handler := Chain(panicking, RequestID, Logging(logger), Recover(logger))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/example", nil))

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", response.Code)
	}
	var body map[string]string
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("body is not JSON: %q", response.Body.String())
	}
	if body["error"] != "internal error" || strings.Contains(response.Body.String(), "boom") {
		t.Errorf("body = %q, want a generic error without panic details", response.Body.String())
	}
	if !strings.Contains(logs.String(), "boom") || !strings.Contains(logs.String(), "stack") {
		t.Errorf("panic not logged with a stack: %s", logs.String())
	}
	if !strings.Contains(logs.String(), `"status":500`) {
		t.Errorf("request log does not record the 500: %s", logs.String())
	}
}

func TestLoggingSkipsSuccessfulHealthChecksOnly(t *testing.T) {
	cases := []struct {
		path    string
		status  int
		wantLog bool
	}{
		{"/healthz", http.StatusOK, false},
		{"/healthz", http.StatusServiceUnavailable, true},
		{"/api/v1/search", http.StatusOK, true},
	}
	for _, tc := range cases {
		var logs bytes.Buffer
		handler := Logging(slog.New(slog.NewJSONHandler(&logs, nil)))(
			http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(tc.status) }),
		)
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, tc.path, nil))

		if logged := logs.Len() > 0; logged != tc.wantLog {
			t.Errorf("%s -> %d: logged = %v, want %v", tc.path, tc.status, logged, tc.wantLog)
		}
	}
}

func TestRecoverPassesThroughNormalRequests(t *testing.T) {
	handler := Recover(slog.Default())(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil))

	if response.Code != http.StatusTeapot {
		t.Errorf("status = %d, want 418", response.Code)
	}
}

func TestRecoverRepanicsErrAbortHandler(t *testing.T) {
	handler := Recover(slog.Default())(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic(http.ErrAbortHandler)
	}))
	defer func() {
		if rec := recover(); rec != http.ErrAbortHandler {
			t.Errorf("recovered %v, want http.ErrAbortHandler to propagate", rec)
		}
	}()
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
}
