package ports

import (
	"context"

	authv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/auth/v1"
)

// Authenticator validates an access token and returns the authenticated identity.
type Authenticator interface {
	ValidateToken(ctx context.Context, accessToken string) (Identity, error)
}

// Identity is the authenticated user information needed by the HTTP layer.
type Identity struct {
	UserID string
	Role   authv1.Role
}
