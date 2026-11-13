// Package app holds Trust's use cases. It depends only on domain and ports.
package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Swiftmeds-bht/Swiftmeds/services/trust/internal/domain"
	"github.com/Swiftmeds-bht/Swiftmeds/services/trust/internal/ports"
)

// Service implements the Trust use cases.
type Service struct {
	repo   ports.TrustRepository
	params domain.Params
	now    func() time.Time
}

// NewService fails if params are invalid, so a bad configuration stops the
// service at startup instead of on the first event. Pass time.Now in
// production and a fixed clock in tests.
func NewService(repo ports.TrustRepository, params domain.Params, now func() time.Time) (*Service, error) {
	if err := params.Validate(); err != nil {
		return nil, err
	}
	return &Service{repo: repo, params: params, now: now}, nil
}

// HandleFeedbackSubmitted adds one feedback.submitted answer to the pharmacy's
// record. It returns false when this feedback was already counted, so running
// it again for the same event is safe.
func (s *Service) HandleFeedbackSubmitted(ctx context.Context, f domain.Feedback) (bool, error) {
	f.ID, f.PharmacyID = strings.TrimSpace(f.ID), strings.TrimSpace(f.PharmacyID)
	if err := f.Validate(); err != nil {
		return false, err
	}
	applied, err := s.repo.ApplyFeedback(ctx, f, func(r domain.Record) domain.Record {
		return r.Apply(f, s.params)
	})
	if err != nil {
		return false, fmt.Errorf("apply feedback %s to pharmacy %s: %w", f.ID, f.PharmacyID, err)
	}
	return applied, nil
}

// GetTrust returns one pharmacy's score; one without feedback is neutral.
func (s *Service) GetTrust(ctx context.Context, pharmacyID string) (domain.Trust, error) {
	pharmacyID = strings.TrimSpace(pharmacyID)
	if pharmacyID == "" {
		return domain.Trust{}, domain.ErrMissingPharmacyID
	}
	out, err := s.BatchGetTrust(ctx, []string{pharmacyID})
	if err != nil {
		return domain.Trust{}, err
	}
	return out[0], nil
}

// BatchGetTrust returns one score per requested pharmacy, in request order.
// Pharmacies without feedback get the neutral score.
func (s *Service) BatchGetTrust(ctx context.Context, pharmacyIDs []string) ([]domain.Trust, error) {
	if err := domain.ValidatePharmacyIDs(pharmacyIDs); err != nil {
		return nil, err
	}
	ids := make([]string, len(pharmacyIDs))
	for i, id := range pharmacyIDs {
		ids[i] = strings.TrimSpace(id)
	}
	if len(ids) == 0 {
		return []domain.Trust{}, nil
	}
	records, err := s.repo.Records(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("read trust records: %w", err)
	}
	now := s.now()
	out := make([]domain.Trust, len(ids))
	for i, id := range ids {
		r, ok := records[id]
		if !ok {
			r = domain.Record{PharmacyID: id}
		}
		out[i] = r.Trust(now, s.params)
	}
	return out, nil
}

// Params returns the scoring parameters in use.
func (s *Service) Params() domain.Params {
	return s.params
}
