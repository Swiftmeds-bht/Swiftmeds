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
