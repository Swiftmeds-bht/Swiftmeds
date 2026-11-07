package clients

import (
	"context"

	authv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/auth/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/gateway/internal/ports"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var _ ports.Authenticator = (*AuthClient)(nil)

// AuthClient calls the Auth service.
type AuthClient struct {
	client authv1.AuthServiceClient
}

func NewAuthClient(client authv1.AuthServiceClient) *AuthClient {
	return &AuthClient{client: client}
}

func (a *AuthClient) ValidateToken(ctx context.Context, accessToken string) (ports.Identity, error) {
	resp, err := a.client.ValidateToken(ctx, &authv1.ValidateTokenRequest{AccessToken: accessToken})
	if err != nil {
		return ports.Identity{}, err
	}
	if resp == nil || resp.GetUserId() == "" || resp.GetRole() == authv1.Role_ROLE_UNSPECIFIED {
		return ports.Identity{}, status.Error(codes.Internal, "auth returned an incomplete identity")
	}
	return ports.Identity{UserID: resp.GetUserId(), Role: resp.GetRole(), PharmacyID: resp.GetPharmacyId()}, nil
}
