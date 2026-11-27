package grpc

import (
	"context"
	"errors"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	commonv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/common/v1"
	registryv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/pharmacy-registry/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/pharmacy-registry/internal/app"
	"github.com/Swiftmeds-bht/Swiftmeds/services/pharmacy-registry/internal/domain"
)

type Handler struct {
	registryv1.UnimplementedPharmacyRegistryServiceServer
	svc *app.Service
}

func NewHandler(svc *app.Service) *Handler { return &Handler{svc: svc} }

func (h *Handler) RegisterPharmacy(ctx context.Context, req *registryv1.RegisterPharmacyRequest) (*registryv1.RegisterPharmacyResponse, error) {
	loc := req.GetLocation()
	var point domain.GeoPoint
	if loc != nil {
		point = domain.GeoPoint{Latitude: loc.GetLatitude(), Longitude: loc.GetLongitude()}
	}
	p, err := h.svc.Register(ctx, app.RegisterInput{
		OwnerID: req.GetOwnerUserId(), Name: req.GetName(), LicenseNumber: req.GetLicenseNumber(),
		Phone: req.GetPhone(), Address: req.GetAddress(), Dzongkhag: req.GetDzongkhag(), Location: point,
		Hours: hoursFromProto(req.GetHours()),
	})
	if err != nil {
		return nil, toStatus(err)
	}
	return &registryv1.RegisterPharmacyResponse{Pharmacy: toProto(p)}, nil
}

func (h *Handler) UpdatePharmacy(ctx context.Context, req *registryv1.UpdatePharmacyRequest) (*registryv1.UpdatePharmacyResponse, error) {
	loc := req.GetLocation()
	var point domain.GeoPoint
	if loc != nil {
		point = domain.GeoPoint{Latitude: loc.GetLatitude(), Longitude: loc.GetLongitude()}
	}
	p, err := h.svc.Update(ctx, req.GetPharmacyId(), req.GetOwnerUserId(), app.UpdateInput{
		Name: req.GetName(), Phone: req.GetPhone(), Address: req.GetAddress(), Dzongkhag: req.GetDzongkhag(), Location: point, Hours: hoursFromProto(req.GetHours()),
	})
	if err != nil {
		return nil, toStatus(err)
	}
	return &registryv1.UpdatePharmacyResponse{Pharmacy: toProto(p)}, nil
}

func hoursFromProto(hours []*registryv1.OpeningHours) []domain.OpeningHours {
	out := make([]domain.OpeningHours, 0, len(hours))
	for _, h := range hours {
		if h != nil {
			out = append(out, domain.OpeningHours{Day: h.GetDay(), OpensAt: h.GetOpensAt(), ClosesAt: h.GetClosesAt()})
		}
	}
	return out
}

func hoursToProto(hours []domain.OpeningHours) []*registryv1.OpeningHours {
	out := make([]*registryv1.OpeningHours, 0, len(hours))
	for _, h := range hours {
		out = append(out, &registryv1.OpeningHours{Day: h.Day, OpensAt: h.OpensAt, ClosesAt: h.ClosesAt})
	}
	return out
}

func (h *Handler) GetPharmacy(ctx context.Context, req *registryv1.GetPharmacyRequest) (*registryv1.GetPharmacyResponse, error) {
	p, err := h.svc.Get(ctx, req.GetPharmacyId())
	if err != nil {
		return nil, toStatus(err)
	}
	return &registryv1.GetPharmacyResponse{Pharmacy: toProto(p)}, nil
}

func (h *Handler) BatchGetPharmacies(ctx context.Context, req *registryv1.BatchGetPharmaciesRequest) (*registryv1.BatchGetPharmaciesResponse, error) {
	all, err := h.svc.List(ctx, false)
	if err != nil {
		return nil, toStatus(err)
	}
	ids := make(map[string]bool, len(req.GetPharmacyIds()))
	for _, id := range req.GetPharmacyIds() {
		ids[id] = true
	}
	out := make([]*registryv1.Pharmacy, 0, len(ids))
	for _, p := range all {
		if ids[p.ID] {
			out = append(out, toProto(p))
		}
	}
	return &registryv1.BatchGetPharmaciesResponse{Pharmacies: out}, nil
}

func (h *Handler) ListPharmacies(ctx context.Context, req *registryv1.ListPharmaciesRequest) (*registryv1.ListPharmaciesResponse, error) {
	verifiedOnly := req.GetStatus() == registryv1.VerificationStatus_VERIFICATION_STATUS_VERIFIED
	all, err := h.svc.List(ctx, verifiedOnly)
	if err != nil {
		return nil, toStatus(err)
	}
	out := make([]*registryv1.Pharmacy, 0, len(all))
	for _, p := range all {
		if req.GetStatus() == registryv1.VerificationStatus_VERIFICATION_STATUS_UNSPECIFIED || statusFromProto(p.Status) == req.GetStatus() {
			out = append(out, toProto(p))
		}
	}
	return &registryv1.ListPharmaciesResponse{Pharmacies: out}, nil
}

func (h *Handler) SetVerificationStatus(ctx context.Context, req *registryv1.SetVerificationStatusRequest) (*registryv1.SetVerificationStatusResponse, error) {
	p, err := h.svc.SetVerificationStatus(ctx, req.GetPharmacyId(), domainStatus(req.GetStatus()), req.GetReason())
	if err != nil {
		return nil, toStatus(err)
	}
	return &registryv1.SetVerificationStatusResponse{Pharmacy: toProto(p)}, nil
}

func toProto(p domain.Pharmacy) *registryv1.Pharmacy {
	return &registryv1.Pharmacy{PharmacyId: p.ID, OwnerUserId: p.OwnerID, Name: p.Name, LicenseNumber: p.LicenseNumber,
		Phone: p.Phone, Address: p.Address, Dzongkhag: p.Dzongkhag,
		Location: &commonv1.GeoPoint{Latitude: p.Location.Latitude, Longitude: p.Location.Longitude},
		Status:   statusFromProto(p.Status), StatusReason: p.StatusReason, CreatedAt: timestamppb.New(p.CreatedAt),
		VerifiedAt: timestampOrNil(p.VerifiedAt), Hours: hoursToProto(p.Hours)}
}

func timestampOrNil(t *time.Time) *timestamppb.Timestamp {
	if t == nil {
		return nil
	}
	return timestamppb.New(*t)
}

func statusFromProto(s domain.Status) registryv1.VerificationStatus {
	switch s {
	case domain.StatusPending:
		return registryv1.VerificationStatus_VERIFICATION_STATUS_PENDING
	case domain.StatusVerified:
		return registryv1.VerificationStatus_VERIFICATION_STATUS_VERIFIED
	case domain.StatusRejected:
		return registryv1.VerificationStatus_VERIFICATION_STATUS_REJECTED
	case domain.StatusSuspended:
		return registryv1.VerificationStatus_VERIFICATION_STATUS_SUSPENDED
	default:
		return registryv1.VerificationStatus_VERIFICATION_STATUS_UNSPECIFIED
	}
}

func domainStatus(s registryv1.VerificationStatus) domain.Status {
	switch s {
	case registryv1.VerificationStatus_VERIFICATION_STATUS_PENDING:
		return domain.StatusPending
	case registryv1.VerificationStatus_VERIFICATION_STATUS_VERIFIED:
		return domain.StatusVerified
	case registryv1.VerificationStatus_VERIFICATION_STATUS_REJECTED:
		return domain.StatusRejected
	case registryv1.VerificationStatus_VERIFICATION_STATUS_SUSPENDED:
		return domain.StatusSuspended
	default:
		return ""
	}
}

func toStatus(err error) error {
	switch {
	case errors.Is(err, domain.ErrMissingPharmacyID), errors.Is(err, domain.ErrMissingOwnerID),
		errors.Is(err, domain.ErrMissingName), errors.Is(err, domain.ErrMissingLicense),
		errors.Is(err, domain.ErrInvalidLocation), errors.Is(err, domain.ErrInvalidTransition),
		errors.Is(err, domain.ErrMissingReason):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, domain.ErrOwnerMismatch):
		return status.Error(codes.PermissionDenied, err.Error())
	case errors.Is(err, app.ErrNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, domain.ErrLicenseTaken):
		return status.Error(codes.AlreadyExists, domain.ErrLicenseTaken.Error())
	default:
		return status.Error(codes.Internal, "internal error")
	}
}
