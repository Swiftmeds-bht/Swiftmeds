package ports

import (
	"context"

	"github.com/Swiftmeds-bht/Swiftmeds/services/user-profile/internal/domain"
)

type ProfileRepository interface {
	Create(ctx context.Context, profile domain.Profile) error
	Get(ctx context.Context, userID string) (domain.Profile, error)
	Update(ctx context.Context, profile domain.Profile) error
}
