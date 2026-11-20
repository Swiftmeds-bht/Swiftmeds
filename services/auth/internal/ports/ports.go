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
	// LinkPharmacy makes the user a pharmacist of pharmacyID and returns the
	// updated user. It must be atomic and idempotent for the same pharmacy, and
	// return domain.ErrUserNotFound, ErrAdminLink or ErrAlreadyLinked.
	LinkPharmacy(ctx context.Context, userID, pharmacyID string) (domain.User, error)
}
