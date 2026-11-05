package httpx

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	authv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/auth/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/gateway/internal/ports"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type fakeAuthenticator struct {
	identity ports.Identity
	err      error
	token    string
}

func (f *fakeAuthenticator) ValidateToken(_ context.Context, token string) (ports.Identity, error) {
	f.token = token
	return f.identity, f.err
}

func TestAuthRejectsMissingAndInvalidTokens(t *testing.T) {
	auth := &fakeAuthenticator{err: status.Error(codes.Unauthenticated, "invalid")}
	handler := Auth(auth, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("protected handler was called")
	}))

	for _, header := range []string{"", "Basic token", "Bearer"} {
		request := httptest.NewRequest(http.MethodGet, "/api/v1/profile", nil)
		request.Header.Set("Authorization", header)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusUnauthorized {
			t.Errorf("header %q: status = %d, want 401", header, response.Code)
		}
	}

	request := httptest.NewRequest(http.MethodGet, "/api/v1/profile", nil)
	request.Header.Set("Authorization", "Bearer invalid")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("invalid token status = %d, want 401", response.Code)
	}
}

func TestAuthAllowsPublicRoutesWithoutToken(t *testing.T) {
	auth := &fakeAuthenticator{}
	called := false
	handler := Auth(auth, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))

	for _, path := range []string{"/api/v1/search", "/api/v1/nearby/pharmacies", "/api/v1/auth/login"} {
		called = false
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if !called || response.Code != http.StatusOK {
			t.Errorf("%s was not allowed through", path)
		}
	}
}

func TestAuthChecksRoleAndAddsIdentityToContext(t *testing.T) {
	auth := &fakeAuthenticator{identity: ports.Identity{UserID: "user-1", Role: authv1.Role_ROLE_PHARMACY_STAFF}}
	handler := Auth(auth, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if UserIDFromContext(r.Context()) != "user-1" || RoleFromContext(r.Context()) != authv1.Role_ROLE_PHARMACY_STAFF {
			t.Error("identity was not added to request context")
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	request := httptest.NewRequest(http.MethodGet, "/api/v1/pharmacies/pharmacy-1/stock", nil)
	request.Header.Set("Authorization", "Bearer valid")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", response.Code)
	}
	if auth.token != "valid" {
		t.Fatalf("token = %q, want valid", auth.token)
	}

	auth.identity.Role = authv1.Role_ROLE_PATIENT
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("wrong role status = %d, want 403", response.Code)
	}
}

func TestAuthFailsClosedWhenAuthIsDown(t *testing.T) {
	auth := &fakeAuthenticator{err: status.Error(codes.Unavailable, "down")}
	handler := Auth(auth, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("protected handler was called")
	}))
	request := httptest.NewRequest(http.MethodGet, "/api/v1/profile", nil)
	request.Header.Set("Authorization", "Bearer valid")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", response.Code)
	}
}
