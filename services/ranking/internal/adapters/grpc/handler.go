// Package grpc is the inbound adapter: RankingService <-> app, domain errors
// -> gRPC status codes. No business rules here.
package grpc

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	rankingv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/ranking/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/ranking/internal/app"
	"github.com/Swiftmeds-bht/Swiftmeds/services/ranking/internal/domain"
)

type Handler struct {
	rankingv1.UnimplementedRankingServiceServer
	svc *app.Service
}

func NewHandler(svc *app.Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) RankOffers(_ context.Context, req *rankingv1.RankOffersRequest) (*rankingv1.RankOffersResponse, error) {
	cands := make([]domain.Candidate, 0, len(req.GetCandidates()))
	for _, c := range req.GetCandidates() {
		cands = append(cands, domain.Candidate{
			PharmacyID:    c.GetPharmacyId(),
			ProductID:     c.GetProductId(),
			DistanceM:     c.GetDistanceM(),
			Freshness:     c.GetFreshnessScore(),
			Trust:         c.GetTrustScore(),
			PriceChhertum: c.GetPriceChhertum(),
			Quantity:      c.GetQuantityAvailable(),
		})
	}

	// An absent Weights message means "use the service defaults".
	var weights *domain.Weights
	if w := req.GetWeights(); w != nil {
		weights = &domain.Weights{Distance: w.GetDistance(), Freshness: w.GetFreshness(), Trust: w.GetTrust(), Price: w.GetPrice()}
	}

	ranked, err := h.svc.RankOffers(cands, weights)
	if err != nil {
		return nil, toStatus(err)
	}

	out := make([]*rankingv1.RankedOffer, 0, len(ranked))
	for _, r := range ranked {
		out = append(out, &rankingv1.RankedOffer{
			PharmacyId: r.PharmacyID,
			ProductId:  r.ProductID,
			Score:      r.Score,
			Breakdown: &rankingv1.ScoreBreakdown{
				Distance:  r.Breakdown.Distance,
				Freshness: r.Breakdown.Freshness,
				Trust:     r.Breakdown.Trust,
				Price:     r.Breakdown.Price,
			},
		})
	}
	return &rankingv1.RankOffersResponse{Offers: out}, nil
}

func toStatus(err error) error {
	switch {
	case errors.Is(err, domain.ErrInvalidCandidate),
		errors.Is(err, domain.ErrInvalidWeights),
		errors.Is(err, domain.ErrTooManyCandidates):
		return status.Error(codes.InvalidArgument, err.Error())
	default:
		return status.Error(codes.Internal, "internal error")
	}
}
