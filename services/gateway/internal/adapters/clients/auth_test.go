package clients

import (
	"context"
	"testing"

	authv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/auth/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type fakeAuth struct {
	authv1.AuthServiceClient
	request  *authv1.ValidateTokenRequest
	response *authv1.ValidateTokenResponse
	err      error
}

func (f *fakeAuth) ValidateToken(_ context.Context, req *authv1.ValidateTokenRequest, _ ...grpc.CallOption) (*authv1.ValidateTokenResponse, error) {
	f.request = req
	return f.response, f.err
}

func TestAuthClientValidatesToken(t *testing.T) {
	fake := &fakeAuth{response: &authv1.ValidateTokenResponse{
		UserId: "user-1", Role: authv1.Role_ROLE_PHARMACY_STAFF,
	}}

	got, err := NewAuthClient(fake).ValidateToken(context.Background(), "token-1")
	if err != nil {
		t.Fatalf("ValidateToken() error = %v", err)
	}
	if got.UserID != "user-1" || got.Role != authv1.Role_ROLE_PHARMACY_STAFF {
		t.Fatalf("identity = %+v", got)
	}
	if fake.request.GetAccessToken() != "token-1" {
		t.Fatalf("access token = %q, want token-1", fake.request.GetAccessToken())
	}
}

func TestAuthClientKeepsGRPCStatus(t *testing.T) {
	fake := &fakeAuth{err: status.Error(codes.Unavailable, "auth is down")}
	_, err := NewAuthClient(fake).ValidateToken(context.Background(), "token-1")
	if status.Code(err) != codes.Unavailable {
		t.Fatalf("status code = %v, want Unavailable", status.Code(err))
	}
}
