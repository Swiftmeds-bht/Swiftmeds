package ports

import (
	"context"

	authv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/auth/v1"
)

// Authenticator validates an access token and returns the authenticated identity.
type Authenticator interface {
	ValidateToken(ctx context.Context, accessToken string) (Identity, error)
}

// AuthRoutes exposes account operations to the REST gateway.
type AuthRoutes interface {
	Register(context.Context, *authv1.RegisterRequest) (*authv1.RegisterResponse, error)
	Login(context.Context, *authv1.LoginRequest) (*authv1.LoginResponse, error)
	RefreshToken(context.Context, *authv1.RefreshTokenRequest) (*authv1.RefreshTokenResponse, error)
	Logout(context.Context, *authv1.LogoutRequest) (*authv1.LogoutResponse, error)
}

// Identity is the authenticated user information needed by the HTTP layer.
type Identity struct {
	UserID     string
	Role       authv1.Role
	PharmacyID string
}
