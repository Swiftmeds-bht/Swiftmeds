package grpc

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	userprofilev1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/user-profile/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/user-profile/internal/app"
	"github.com/Swiftmeds-bht/Swiftmeds/services/user-profile/internal/domain"
)

type Handler struct {
	userprofilev1.UnimplementedUserProfileServiceServer
	svc *app.Service
}

func NewHandler(svc *app.Service) *Handler { return &Handler{svc: svc} }

func (h *Handler) GetProfile(ctx context.Context, req *userprofilev1.GetProfileRequest) (*userprofilev1.GetProfileResponse, error) {
	p, err := h.svc.GetProfile(ctx, req.GetUserId())
	if err != nil {
		return nil, toStatus(err)
	}
	return &userprofilev1.GetProfileResponse{Profile: toProto(p)}, nil
}

func (h *Handler) UpdateProfile(ctx context.Context, req *userprofilev1.UpdateProfileRequest) (*userprofilev1.UpdateProfileResponse, error) {
	input := app.Input{UserID: req.GetUserId(), Name: req.GetDisplayName(), Phone: req.GetPhone()}
	p, err := h.svc.UpdateProfile(ctx, input)
	if errors.Is(err, domain.ErrProfileNotFound) {
		p, err = h.svc.CreateProfile(ctx, input)
	}
	if err != nil {
		return nil, toStatus(err)
	}
	return &userprofilev1.UpdateProfileResponse{Profile: toProto(p)}, nil
}

func toProto(p domain.Profile) *userprofilev1.Profile {
	return &userprofilev1.Profile{
		UserId: p.UserID, DisplayName: p.Name, Phone: p.Phone,
		CreatedAt: timestamppb.New(p.CreatedAt), UpdatedAt: timestamppb.New(p.UpdatedAt),
	}
}

func toStatus(err error) error {
	switch {
	case errors.Is(err, domain.ErrMissingUserID), errors.Is(err, domain.ErrMissingName),
		errors.Is(err, domain.ErrInvalidPhone):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, domain.ErrProfileNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, domain.ErrProfileExists):
		return status.Error(codes.AlreadyExists, err.Error())
	default:
		return status.Error(codes.Internal, "internal error")
	}
}
