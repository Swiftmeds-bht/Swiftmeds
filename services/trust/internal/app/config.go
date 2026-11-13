package app

import (
	"fmt"
	"strconv"

	"github.com/Swiftmeds-bht/Swiftmeds/pkg/config"
	"github.com/Swiftmeds-bht/Swiftmeds/services/trust/internal/domain"
)

// Environment variables that tune the score. Unset means domain.DefaultParams.
const (
	EnvHalfLife    = "TRUST_HALF_LIFE"
	EnvPriorWeight = "TRUST_PRIOR_WEIGHT"
)

// ParamsFromConfig reads the scoring parameters and validates them. An
// unparsable TRUST_HALF_LIFE is reported by cfg.Err(), like every other variable.
func ParamsFromConfig(cfg *config.Loader) (domain.Params, error) {
	p := domain.Params{
		HalfLife:    cfg.Duration(EnvHalfLife, domain.DefaultParams.HalfLife),
		PriorWeight: domain.DefaultParams.PriorWeight,
	}
	if raw := cfg.String(EnvPriorWeight, ""); raw != "" {
		v, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return domain.Params{}, fmt.Errorf("%s=%q is not a number", EnvPriorWeight, raw)
		}
		p.PriorWeight = v
	}
	if err := p.Validate(); err != nil {
		return domain.Params{}, fmt.Errorf("trust params (%s=%v, %s=%v): %w", EnvHalfLife, p.HalfLife, EnvPriorWeight, p.PriorWeight, err)
	}
	return p, nil
}
