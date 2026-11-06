package httpx

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// The Cloudflare Worker routes use the shared secret, not a user token, so the
// user-token check must let them through. Their handlers check the secret.
func TestAuthLetsWorkerRoutesThroughWithoutAUserToken(t *testing.T) {
	for _, path := range []string{"/api/v1/internal/freshness/decay", "/api/v1/internal/expiry/sweep"} {
		t.Run(path, func(t *testing.T) {
			called := false
			auth := &fakeAuthenticator{err: status.Error(codes.Unauthenticated, "invalid")}
			handler := Auth(auth, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))

			handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, path, nil))

			if !called {
				t.Error("worker route was blocked by the user-token check")
			}
			if auth.token != "" {
				t.Errorf("ValidateToken was called with %q, want no call", auth.token)
			}
		})
	}
}

// Only the exact worker paths are exempt. Anything else under /internal/ still needs a user token.
func TestAuthStillProtectsOtherInternalRoutes(t *testing.T) {
	for _, path := range []string{
		"/api/v1/internal/expiry/sweep/",
		"/api/v1/internal/expiry/sweep/extra",
		"/api/v1/internal/other",
		"/api/v1/internal",
	} {
		t.Run(path, func(t *testing.T) {
			auth := &fakeAuthenticator{err: status.Error(codes.Unauthenticated, "invalid")}
			handler := Auth(auth, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				t.Fatal("protected handler was called")
			}))
			response := httptest.NewRecorder()

			handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, path, nil))

			if response.Code != http.StatusUnauthorized {
				t.Errorf("status = %d, want 401", response.Code)
			}
		})
	}
}
