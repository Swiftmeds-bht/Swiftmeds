package grpc

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	authv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/auth/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/auth/internal/app"
	"github.com/Swiftmeds-bht/Swiftmeds/services/auth/internal/domain"
)

type Handler struct {
	authv1.UnimplementedAuthServiceServer
	svc *app.Service
}

func NewHandler(svc *app.Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) Register(ctx context.Context, req *authv1.RegisterRequest) (*authv1.RegisterResponse, error) {
	role, ok := fromProtoRole(req.GetRole())
	if !ok {
		return nil, toStatus(domain.ErrInvalidRole)
	}
	user, err := h.svc.Register(ctx, req.GetEmail(), req.GetPassword(), role)
	if err != nil {
		return nil, toStatus(err)
	}
	return &authv1.RegisterResponse{UserId: user.ID}, nil
}

func (h *Handler) Login(ctx context.Context, req *authv1.LoginRequest) (*authv1.LoginResponse, error) {
	user, tokens, err := h.svc.Login(ctx, req.GetEmail(), req.GetPassword())
	if err != nil {
		return nil, toStatus(err)
	}
	return &authv1.LoginResponse{
		UserId: user.ID, Role: toProtoRole(user.Role), Tokens: toProtoTokens(tokens),
	}, nil
}

func (h *Handler) RefreshToken(ctx context.Context, req *authv1.RefreshTokenRequest) (*authv1.RefreshTokenResponse, error) {
	tokens, err := h.svc.Refresh(ctx, req.GetRefreshToken())
	if err != nil {
		return nil, toStatus(err)
	}
	return &authv1.RefreshTokenResponse{Tokens: toProtoTokens(tokens)}, nil
}

func (h *Handler) ValidateToken(ctx context.Context, req *authv1.ValidateTokenRequest) (*authv1.ValidateTokenResponse, error) {
	identity, err := h.svc.ValidateToken(ctx, req.GetAccessToken())
	if err != nil {
		return nil, toStatus(err)
	}
	return &authv1.ValidateTokenResponse{
		UserId: identity.UserID, Role: toProtoRole(identity.Role), PharmacyId: identity.PharmacyID,
		ExpiresAt: timestamppb.New(identity.ExpiresAt),
	}, nil
}

func fromProtoRole(role authv1.Role) (domain.Role, bool) {
	switch role {
	case authv1.Role_ROLE_PATIENT:
		return domain.RolePatient, true
	case authv1.Role_ROLE_PHARMACY_STAFF:
		return domain.RolePharmacist, true
	case authv1.Role_ROLE_ADMIN:
		return domain.RoleAdmin, true
	default:
		return "", false
	}
}

func toProtoRole(role domain.Role) authv1.Role {
	switch role {
	case domain.RolePatient:
		return authv1.Role_ROLE_PATIENT
	case domain.RolePharmacist:
		return authv1.Role_ROLE_PHARMACY_STAFF
	case domain.RoleAdmin:
		return authv1.Role_ROLE_ADMIN
	default:
		return authv1.Role_ROLE_UNSPECIFIED
	}
}

func toProtoTokens(tokens app.TokenPair) *authv1.TokenPair {
	return &authv1.TokenPair{
		AccessToken: tokens.AccessToken, RefreshToken: tokens.RefreshToken,
		AccessTokenExpiresAt: timestamppb.New(tokens.AccessTokenExpiresAt),
	}
}

func toStatus(err error) error {
	switch {
	case errors.Is(err, domain.ErrInvalidEmail), errors.Is(err, domain.ErrInvalidPassword),
		errors.Is(err, domain.ErrInvalidRole), errors.Is(err, domain.ErrAdminRegistration):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, domain.ErrUserExists):
		return status.Error(codes.AlreadyExists, err.Error())
	case errors.Is(err, domain.ErrInvalidCredentials), errors.Is(err, domain.ErrInvalidToken),
		errors.Is(err, domain.ErrRefreshRevoked):
		return status.Error(codes.Unauthenticated, err.Error())
	default:
		return status.Error(codes.Internal, "internal error")
	}
}
