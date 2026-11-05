package grpc

import (
	"context"
	"errors"
	"math"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	rankingv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/ranking/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/ranking/internal/app"
	"github.com/Swiftmeds-bht/Swiftmeds/services/ranking/internal/domain"
)

func newHandler(t *testing.T) *Handler {
	t.Helper()
	svc, err := app.NewService(domain.DefaultWeights)
	if err != nil {
		t.Fatal(err)
	}
	return NewHandler(svc)
}

func TestRankOffersMapsRequestAndResponse(t *testing.T) {
	req := &rankingv1.RankOffersRequest{Candidates: []*rankingv1.Candidate{
		{PharmacyId: "near", ProductId: "p", DistanceM: 500, FreshnessScore: 0.5, TrustScore: 0.5, PriceChhertum: 600, QuantityAvailable: 3},
		{PharmacyId: "far", ProductId: "p", DistanceM: 1500, FreshnessScore: 1, TrustScore: 1, PriceChhertum: 400, QuantityAvailable: 3},
		{PharmacyId: "empty", ProductId: "p", DistanceM: 1, FreshnessScore: 1, TrustScore: 1, PriceChhertum: 1, QuantityAvailable: 0},
	}}
	resp, err := newHandler(t).RankOffers(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	offers := resp.GetOffers()
	if len(offers) != 2 || offers[0].GetPharmacyId() != "near" || offers[1].GetPharmacyId() != "far" {
		t.Fatalf("offers = %v", offers)
	}
	if math.Abs(offers[0].GetScore()-0.625) > 1e-9 || offers[0].GetProductId() != "p" {
		t.Errorf("first offer = %v", offers[0])
	}
	b := offers[0].GetBreakdown()
	if math.Abs(b.GetDistance()+b.GetFreshness()+b.GetTrust()+b.GetPrice()-offers[0].GetScore()) > 1e-9 {
		t.Errorf("breakdown %v does not add up to score %v", b, offers[0].GetScore())
	}
}

func TestRankOffersUsesRequestWeights(t *testing.T) {
	req := &rankingv1.RankOffersRequest{
		Candidates: []*rankingv1.Candidate{
			{PharmacyId: "near-dear", ProductId: "p", DistanceM: 100, PriceChhertum: 900, QuantityAvailable: 1},
			{PharmacyId: "far-cheap", ProductId: "p", DistanceM: 9000, PriceChhertum: 100, QuantityAvailable: 1},
		},
		Weights: &rankingv1.Weights{Price: 1},
	}
	resp, err := newHandler(t).RankOffers(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.GetOffers()[0].GetPharmacyId() != "far-cheap" {
		t.Errorf("first = %s, want far-cheap with price-only weights", resp.GetOffers()[0].GetPharmacyId())
	}
}

func TestRankOffersInvalidInputIsInvalidArgument(t *testing.T) {
	h := newHandler(t)
	cases := map[string]*rankingv1.RankOffersRequest{
		"bad candidate": {Candidates: []*rankingv1.Candidate{{ProductId: "p", QuantityAvailable: 1}}},
		"bad weights":   {Weights: &rankingv1.Weights{Distance: 0.5}},
	}
	for name, req := range cases {
		if _, err := h.RankOffers(context.Background(), req); status.Code(err) != codes.InvalidArgument {
			t.Errorf("%s: code = %v, want InvalidArgument", name, status.Code(err))
		}
	}
	many := &rankingv1.RankOffersRequest{}
	for i := 0; i <= domain.MaxCandidates; i++ {
		many.Candidates = append(many.Candidates, &rankingv1.Candidate{PharmacyId: "x", ProductId: "p", QuantityAvailable: 1})
	}
	if _, err := h.RankOffers(context.Background(), many); status.Code(err) != codes.InvalidArgument {
		t.Errorf("too many: code = %v", status.Code(err))
	}
}

func TestEmptyRequestReturnsNoOffers(t *testing.T) {
	resp, err := newHandler(t).RankOffers(context.Background(), &rankingv1.RankOffersRequest{})
	if err != nil || len(resp.GetOffers()) != 0 {
		t.Fatalf("got %v, %v", resp, err)
	}
}

func TestUnexpectedErrorsAreInternal(t *testing.T) {
	err := toStatus(errors.New("boom"))
	if status.Code(err) != codes.Internal || status.Convert(err).Message() != "internal error" {
		t.Errorf("toStatus = %v", err)
	}
}
