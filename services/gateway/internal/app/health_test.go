package app

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCheckHealthAgainstRouter(t *testing.T) {
	server := httptest.NewServer(NewRouter(Config{CORSOrigins: defaultCORSOrigins}, slog.Default()))
	t.Cleanup(server.Close)

	if err := CheckHealth(context.Background(), server.Client(), server.URL+"/healthz"); err != nil {
		t.Fatalf("CheckHealth(/healthz) = %v, want nil", err)
	}
	if err := CheckHealth(context.Background(), server.Client(), server.URL+"/api/v1/nothing"); err == nil {
		t.Fatal("CheckHealth on a 404 returned nil, want error")
	}
}

func TestNewRouterRegistersConfiguredHoldRoutes(t *testing.T) {
	handler := NewRouter(Config{CORSOrigins: defaultCORSOrigins}, slog.Default(), WithHolds(&fakeHoldsClient{}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/holds", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("holds route got %d, want 401 from the authentication guard", rec.Code)
	}
}

func TestCheckHealthFailsWhenNothingListens(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	url := server.URL
	server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := CheckHealth(ctx, http.DefaultClient, url+"/healthz"); err == nil {
		t.Fatal("CheckHealth on a closed server returned nil, want error")
	}
}
