package ports

import (
	"context"
	"time"

	"github.com/Swiftmeds-bht/Swiftmeds/services/auth/internal/domain"
)

type UserRepository interface {
	CreateUser(context.Context, domain.User) error
	FindUserByEmail(context.Context, string) (domain.User, error)
	FindUserByID(context.Context, string) (domain.User, error)
	SaveRefreshToken(context.Context, string, string, time.Time) error
	ConsumeRefreshToken(context.Context, string) (string, error)
	// RevokeRefreshToken marks the token revoked. Revoking an unknown or
	// already revoked token is not an error.
	RevokeRefreshToken(context.Context, string) error
}
