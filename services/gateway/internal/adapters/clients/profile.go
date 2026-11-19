package clients

import (
	"context"

	userprofilev1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/user-profile/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/gateway/internal/ports"
)

var _ ports.ProfileClient = (*ProfileClient)(nil)

// ProfileClient calls the User Profile service.
type ProfileClient struct {
	client userprofilev1.UserProfileServiceClient
}

func NewProfileClient(client userprofilev1.UserProfileServiceClient) *ProfileClient {
	return &ProfileClient{client: client}
}

func (c *ProfileClient) GetProfile(ctx context.Context, req *userprofilev1.GetProfileRequest) (*userprofilev1.GetProfileResponse, error) {
	return c.client.GetProfile(ctx, req)
}

func (c *ProfileClient) UpdateProfile(ctx context.Context, req *userprofilev1.UpdateProfileRequest) (*userprofilev1.UpdateProfileResponse, error) {
	return c.client.UpdateProfile(ctx, req)
}
