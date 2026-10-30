package app

import (
	"errors"
	"strings"
	"testing"

	"github.com/Swiftmeds-bht/Swiftmeds/pkg/config"
	"github.com/Swiftmeds-bht/Swiftmeds/services/ranking/internal/domain"
)

func TestNewServiceRejectsInvalidDefaults(t *testing.T) {
	if _, err := NewService(domain.Weights{}); !errors.Is(err, domain.ErrInvalidWeights) {
		t.Fatalf("err = %v, want ErrInvalidWeights", err)
	}
}

func TestRankOffersUsesDefaultsUnlessWeightsGiven(t *testing.T) {
	onlyDistance := domain.Weights{Distance: 1}
	svc, err := NewService(onlyDistance)
	if err != nil {
		t.Fatal(err)
	}
	cs := []domain.Candidate{
		{PharmacyID: "far-but-cheap", ProductID: "p", DistanceM: 9000, PriceChhertum: 100, Quantity: 1},
		{PharmacyID: "near-but-dear", ProductID: "p", DistanceM: 100, PriceChhertum: 900, Quantity: 1},
	}

	got, err := svc.RankOffers(cs, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got[0].PharmacyID != "near-but-dear" {
		t.Errorf("defaults (distance only): first = %s", got[0].PharmacyID)
	}

	got, err = svc.RankOffers(cs, &domain.Weights{Price: 1})
	if err != nil {
		t.Fatal(err)
	}
	if got[0].PharmacyID != "far-but-cheap" {
		t.Errorf("request weights (price only): first = %s", got[0].PharmacyID)
	}
	if svc.DefaultWeights() != onlyDistance {
		t.Errorf("DefaultWeights() = %+v", svc.DefaultWeights())
	}
}

func TestRankOffersPassesValidationErrorsThrough(t *testing.T) {
	svc, _ := NewService(domain.DefaultWeights)
	if _, err := svc.RankOffers(nil, &domain.Weights{Distance: 2}); !errors.Is(err, domain.ErrInvalidWeights) {
		t.Fatalf("err = %v", err)
	}
}

func TestWeightsFromConfig(t *testing.T) {
	cases := []struct {
		name    string
		env     map[string]string
		want    domain.Weights
		wantErr string
	}{
		{"nothing set uses the defaults", nil, domain.DefaultWeights, ""},
		{"all four set", map[string]string{
			EnvWeightDistance: "0.5", EnvWeightFreshness: "0.2", EnvWeightTrust: "0.2", EnvWeightPrice: "0.1",
		}, domain.Weights{Distance: 0.5, Freshness: 0.2, Trust: 0.2, Price: 0.1}, ""},
		{"partial override no longer sums to 1", map[string]string{EnvWeightDistance: "0.9"}, domain.Weights{}, "sum to 1"},
		{"not a number", map[string]string{EnvWeightPrice: "cheap", EnvWeightTrust: "high"}, domain.Weights{}, "RANKING_WEIGHT_PRICE"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := WeightsFromConfig(config.NewFromMap(tc.env))
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want it to mention %q", err, tc.wantErr)
				}
				if tc.name == "not a number" && !strings.Contains(err.Error(), "RANKING_WEIGHT_TRUST") {
					t.Errorf("only the first bad value was reported: %v", err)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Fatalf("got %+v, %v; want %+v", got, err, tc.want)
			}
		})
	}
}
