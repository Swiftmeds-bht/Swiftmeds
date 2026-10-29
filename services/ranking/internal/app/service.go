// Package app holds Ranking's use case. It depends only on domain.
package app

import (
	"github.com/Swiftmeds-bht/Swiftmeds/services/ranking/internal/domain"
)

// Service ranks offers with the configured default weights unless a request
// brings its own.
type Service struct {
	defaults domain.Weights
}

// NewService fails if the configured default weights are invalid, so a bad
// configuration stops the service at startup instead of on the first request.
func NewService(defaults domain.Weights) (*Service, error) {
	if err := defaults.Validate(); err != nil {
		return nil, err
	}
	return &Service{defaults: defaults}, nil
}

// RankOffers ranks candidates. weights nil means "use the defaults".
func (s *Service) RankOffers(candidates []domain.Candidate, weights *domain.Weights) ([]domain.Ranked, error) {
	w := s.defaults
	if weights != nil {
		w = *weights
	}
	return domain.Rank(candidates, w)
}

// DefaultWeights returns the weights used when a request brings none.
func (s *Service) DefaultWeights() domain.Weights {
	return s.defaults
}
