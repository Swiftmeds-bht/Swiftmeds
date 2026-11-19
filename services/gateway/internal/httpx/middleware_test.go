package httpx

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRequestIDAndCORS(t *testing.T) {
	handler := Chain(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if RequestIDFromContext(r.Context()) == "" {
			t.Fatal("request ID missing from context")
		}
		w.WriteHeader(http.StatusNoContent)
	}), RequestID, Logging(slog.Default()), CORS([]string{"http://localhost:3000"}))

	request := httptest.NewRequest(http.MethodGet, "/api/v1/example", nil)
	request.Header.Set("Origin", "http://localhost:3000")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusNoContent)
	}
	if response.Header().Get("X-Request-ID") == "" {
		t.Fatal("response request ID missing")
	}
	if response.Header().Get("Access-Control-Allow-Origin") != "http://localhost:3000" {
		t.Fatal("expected allowed CORS origin")
	}
}

func TestCORSRejectsUnknownPreflight(t *testing.T) {
	handler := CORS([]string{"http://localhost:3000"})(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	request := httptest.NewRequest(http.MethodOptions, "/", strings.NewReader(""))
	request.Header.Set("Origin", "http://evil.example")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusForbidden)
	}
}

func TestRequestIDRejectsOverlongClientValue(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set(requestIDHeader, strings.Repeat("a", maxRequestIDLength+1))
	response := httptest.NewRecorder()
	RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := RequestIDFromContext(r.Context())
		if len(got) == 0 || len(got) > maxRequestIDLength {
			t.Errorf("request ID length = %d, want 1..%d", len(got), maxRequestIDLength)
		}
	})).ServeHTTP(response, request)
	if got := response.Header().Get(requestIDHeader); len(got) == 0 || len(got) > maxRequestIDLength {
		t.Fatalf("response request ID length = %d, want 1..%d", len(got), maxRequestIDLength)
	}
}
