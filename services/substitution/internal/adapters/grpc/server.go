package grpc

import (
	"context"
	"errors"

	substitutionv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/substitution/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/substitution/internal/app"
	"github.com/Swiftmeds-bht/Swiftmeds/services/substitution/internal/domain"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Handler struct {
	substitutionv1.UnimplementedSubstitutionServiceServer
	service *app.Service
}

func NewHandler(service *app.Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) GetSubstitutes(
	ctx context.Context,
	request *substitutionv1.GetSubstitutesRequest,
) (*substitutionv1.GetSubstitutesResponse, error) {
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	if h == nil || h.service == nil {
		return nil, status.Error(codes.Internal, "substitution service is unavailable")
	}

	result, err := h.service.GetSubstitutes(
		ctx,
		request.GetProductId(),
		int(request.GetLimit()),
	)
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrMissingProductID):
			return nil, status.Error(codes.InvalidArgument, err.Error())
		case errors.Is(err, domain.ErrMissingMoleculeID):
			return nil, status.Error(codes.Internal, err.Error())
		default:
			if upstream, ok := status.FromError(err); ok {
				return nil, upstream.Err()
			}
			return nil, status.Error(codes.Internal, "get substitutes failed")
		}
	}

	substitutes := make([]*substitutionv1.Substitute, 0, len(result.Substitutes))
	for _, item := range result.Substitutes {
		substitutes = append(substitutes, &substitutionv1.Substitute{
			ProductId:   item.ProductID,
			BrandName:   item.BrandName,
			GenericName: item.GenericName,
			Strength:    item.Strength,
			MatchType:   toProtoMatchType(item.MatchType),
		})
	}

	return &substitutionv1.GetSubstitutesResponse{
		MoleculeId:  result.MoleculeID,
		Substitutes: substitutes,
	}, nil
}

func toProtoMatchType(matchType domain.MatchType) substitutionv1.SubstituteMatchType {
	switch matchType {
	case domain.MatchTypeExact:
		return substitutionv1.SubstituteMatchType_SUBSTITUTE_MATCH_TYPE_EXACT
	case domain.MatchTypeNeedsPharmacistCheck:
		return substitutionv1.SubstituteMatchType_SUBSTITUTE_MATCH_TYPE_NEEDS_PHARMACIST_CHECK
	default:
		return substitutionv1.SubstituteMatchType_SUBSTITUTE_MATCH_TYPE_UNSPECIFIED
	}
}

var _ substitutionv1.SubstitutionServiceServer = (*Handler)(nil)
