package app

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	authv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/auth/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/gateway/internal/ports"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type fakeAuthRoutes struct{ loginErr error }

func (*fakeAuthRoutes) ValidateToken(context.Context, string) (ports.Identity, error) {
	return ports.Identity{UserID: "u1", Role: authv1.Role_ROLE_PATIENT}, nil
}
func (*fakeAuthRoutes) Register(_ context.Context, req *authv1.RegisterRequest) (*authv1.RegisterResponse, error) {
	if req.GetRole() == authv1.Role_ROLE_PHARMACY_STAFF {
		return nil, status.Error(codes.InvalidArgument, "pharmacy staff accounts cannot self-register")
	}
	if req.GetEmail() == "taken@example.com" {
		return nil, status.Error(codes.AlreadyExists, "duplicate")
	}
	return &authv1.RegisterResponse{UserId: "u1"}, nil
}
func (f *fakeAuthRoutes) Login(context.Context, *authv1.LoginRequest) (*authv1.LoginResponse, error) {
	if f.loginErr != nil {
		return nil, f.loginErr
	}
	return &authv1.LoginResponse{UserId: "u1", Role: authv1.Role_ROLE_PATIENT, Tokens: &authv1.TokenPair{AccessToken: "access", RefreshToken: "refresh"}}, nil
}
func (*fakeAuthRoutes) RefreshToken(context.Context, *authv1.RefreshTokenRequest) (*authv1.RefreshTokenResponse, error) {
	return &authv1.RefreshTokenResponse{Tokens: &authv1.TokenPair{AccessToken: "next", RefreshToken: "next-refresh"}}, nil
}
func (*fakeAuthRoutes) Logout(context.Context, *authv1.LogoutRequest) (*authv1.LogoutResponse, error) {
	return &authv1.LogoutResponse{}, nil
}

func TestAuthRoutesLoginAndRegisterErrors(t *testing.T) {
	tests := []struct {
		name, path, body string
		want             int
		contains         string
		loginErr         error
	}{
		{"login tokens", "/api/v1/auth/login", `{"email":"a@example.com","password":"password"}`, 200, `"accessToken":"access"`, nil},
		{"wrong password", "/api/v1/auth/login", `{"email":"a@example.com","password":"wrong"}`, 401, "unauthorized", status.Error(codes.Unauthenticated, "wrong")},
		{"duplicate register", "/api/v1/auth/register", `{"email":"taken@example.com","password":"password"}`, 409, "conflict", nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fake := &fakeAuthRoutes{loginErr: tc.loginErr}
			h := NewRouter(Config{CORSOrigins: defaultCORSOrigins}, slog.Default(), WithAuth(fake))
			req := httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(tc.body))
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != tc.want {
				t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
			}
			if !strings.Contains(strings.ToLower(rec.Body.String()), strings.ToLower(tc.contains)) {
				t.Fatalf("body=%s missing %q", rec.Body.String(), tc.contains)
			}
		})
	}
}

func TestAuthRoutesRegisterRefreshAndLogout(t *testing.T) {
	tests := []struct {
		name, path, body string
		want             int
		contains         string
	}{
		{"register patient", "/api/v1/auth/register", `{"email":"new@example.com","password":"password"}`, http.StatusCreated, `"userId":"u1"`},
		{"register pharmacy staff rejected by auth service", "/api/v1/auth/register", `{"email":"staff@example.com","password":"password","role":"ROLE_PHARMACY_STAFF"}`, http.StatusBadRequest, "bad request"},
		{"refresh", "/api/v1/auth/refresh", `{"refreshToken":"refresh"}`, http.StatusOK, `"accessToken":"next"`},
		{"logout", "/api/v1/auth/logout", `{"refreshToken":"refresh"}`, http.StatusNoContent, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fake := &fakeAuthRoutes{}
			h := NewRouter(Config{CORSOrigins: defaultCORSOrigins}, slog.Default(), WithAuth(fake))
			req := httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(tc.body))
			if tc.path == "/api/v1/auth/logout" {
				req.Header.Set("Authorization", "Bearer access")
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != tc.want {
				t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
			}
			if tc.contains != "" && !strings.Contains(rec.Body.String(), tc.contains) {
				t.Fatalf("body=%s missing %q", rec.Body.String(), tc.contains)
			}
		})
	}
}

func TestAuthRoutesRejectInvalidBodies(t *testing.T) {
	tests := []struct{ path, body string }{
		{"/api/v1/auth/register", `{"email":"","password":"password"}`},
		{"/api/v1/auth/register", `{"email":"a@example.com","password":"password","role":"ROLE_ADMIN"}`},
		{"/api/v1/auth/login", `{"email":"a@example.com"}`},
		{"/api/v1/auth/refresh", `{}`},
		{"/api/v1/auth/logout", `{}`},
		{"/api/v1/auth/login", `not-json`},
	}
	for _, tc := range tests {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(tc.body))
		authHandler(&fakeAuthRoutes{}, slog.Default())(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s body %q: status=%d, want 400", tc.path, tc.body, rec.Code)
		}
	}
}

// deadlineAuthRoutes records how much time Register and Login were given.
type deadlineAuthRoutes struct {
	fakeAuthRoutes
	budget time.Duration
}

func (f *deadlineAuthRoutes) record(ctx context.Context) {
	if deadline, ok := ctx.Deadline(); ok {
		f.budget = time.Until(deadline)
	}
}
func (f *deadlineAuthRoutes) Register(ctx context.Context, req *authv1.RegisterRequest) (*authv1.RegisterResponse, error) {
	f.record(ctx)
	return f.fakeAuthRoutes.Register(ctx, req)
}
func (f *deadlineAuthRoutes) Login(ctx context.Context, req *authv1.LoginRequest) (*authv1.LoginResponse, error) {
	f.record(ctx)
	return f.fakeAuthRoutes.Login(ctx, req)
}

// Register and Login run bcrypt, so they get AuthCallTimeout instead of the 2 s
// every other route gets (SCRUM-136).
func TestAuthRoutesGetTheLongerAuthBudget(t *testing.T) {
	for _, path := range []string{"/api/v1/auth/register", "/api/v1/auth/login"} {
		t.Run(path, func(t *testing.T) {
			fake := &deadlineAuthRoutes{}
			h := NewRouter(Config{CORSOrigins: defaultCORSOrigins}, slog.Default(), WithAuth(fake))
			req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"email":"a@example.com","password":"password"}`))
			h.ServeHTTP(httptest.NewRecorder(), req)

			if fake.budget <= downstreamTimeout || fake.budget > AuthCallTimeout {
				t.Fatalf("auth call budget = %v, want more than %v and at most %v", fake.budget, downstreamTimeout, AuthCallTimeout)
			}
		})
	}
}
