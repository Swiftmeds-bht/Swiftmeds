package app

import (
	"fmt"
	"strconv"

	"github.com/Swiftmeds-bht/Swiftmeds/pkg/config"
	"github.com/Swiftmeds-bht/Swiftmeds/services/ranking/internal/domain"
)

// Environment variables that override the default weights. Leave all unset
// to use domain.DefaultWeights; if you set any, the four must still sum to 1.
const (
	EnvWeightDistance  = "RANKING_WEIGHT_DISTANCE"
	EnvWeightFreshness = "RANKING_WEIGHT_FRESHNESS"
	EnvWeightTrust     = "RANKING_WEIGHT_TRUST"
	EnvWeightPrice     = "RANKING_WEIGHT_PRICE"
)

// WeightsFromConfig reads the default weights, reporting every unparsable
// value at once.
func WeightsFromConfig(cfg *config.Loader) (domain.Weights, error) {
	d := domain.DefaultWeights
	var errs []error
	parse := func(key string, def float64) float64 {
		raw := cfg.String(key, "")
		if raw == "" {
			return def
		}
		v, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s=%q is not a number", key, raw))
			return def
		}
		return v
	}
	w := domain.Weights{
		Distance:  parse(EnvWeightDistance, d.Distance),
		Freshness: parse(EnvWeightFreshness, d.Freshness),
		Trust:     parse(EnvWeightTrust, d.Trust),
		Price:     parse(EnvWeightPrice, d.Price),
	}
	if len(errs) > 0 {
		return domain.Weights{}, fmt.Errorf("ranking weights: %v", errs)
	}
	if err := w.Validate(); err != nil {
		return domain.Weights{}, fmt.Errorf("ranking weights %+v: %w", w, err)
	}
	return w, nil
}
