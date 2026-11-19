package ports

import (
	"context"

	userprofilev1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/user-profile/v1"
)

// ProfileClient reads and saves a user's own profile (SCRUM-137).
type ProfileClient interface {
	GetProfile(context.Context, *userprofilev1.GetProfileRequest) (*userprofilev1.GetProfileResponse, error)
	UpdateProfile(context.Context, *userprofilev1.UpdateProfileRequest) (*userprofilev1.UpdateProfileResponse, error)
}
