package clients

import (
	"context"

	authv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/auth/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/gateway/internal/ports"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var _ ports.Authenticator = (*AuthClient)(nil)
var _ ports.AuthRoutes = (*AuthClient)(nil)

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

func (a *AuthClient) Register(ctx context.Context, req *authv1.RegisterRequest) (*authv1.RegisterResponse, error) {
	return a.client.Register(ctx, req)
}
func (a *AuthClient) Login(ctx context.Context, req *authv1.LoginRequest) (*authv1.LoginResponse, error) {
	return a.client.Login(ctx, req)
}
func (a *AuthClient) RefreshToken(ctx context.Context, req *authv1.RefreshTokenRequest) (*authv1.RefreshTokenResponse, error) {
	return a.client.RefreshToken(ctx, req)
}
func (a *AuthClient) Logout(ctx context.Context, req *authv1.LogoutRequest) (*authv1.LogoutResponse, error) {
	return a.client.Logout(ctx, req)
}
