// Package ports defines the interfaces the app layer needs from the outside world.
// Adapters (memory, postgres, clients, rabbitmq) implement them; tests use fakes.
package ports

import (
	"context"

	"github.com/Swiftmeds-bht/Swiftmeds/services/feedback/internal/domain"
)

// FeedbackRepository stores feedback. Every implementation must pass the shared
// contract tests in internal/adapters/repotest.
type FeedbackRepository interface {
	// Save stores f. It returns domain.ErrAlreadySubmitted if the hold already
	// has feedback; two saves for one hold at the same moment cannot both succeed.
	//
	// If beforeCommit is not nil it runs after f is stored and before the store is
	// final. If it returns an error, nothing is stored and Save returns that error
	// unchanged. This lets the caller send an event that must not be sent for
	// feedback that was never saved, and not save feedback whose event failed.
	// beforeCommit runs only when the hold had no feedback yet. It must not call
	// the repository.
	Save(ctx context.Context, f domain.Feedback, beforeCommit func(context.Context) error) error
	// ListByPharmacy returns the pharmacy's feedback, newest first (by created_at,
	// then by ID), starting after f.After and at most f.Limit of it.
	ListByPharmacy(ctx context.Context, f domain.ListFilter) ([]domain.Feedback, error)
}

// HoldReader reads a hold from the Holds service.
type HoldReader interface {
	// GetHold returns the hold, domain.ErrHoldNotFound if there is none, or
	// domain.ErrHoldsUnavailable if Holds could not answer.
	GetHold(ctx context.Context, holdID string) (domain.Hold, error)
}

// EventPublisher sends feedback events to the other services.
type EventPublisher interface {
	// PublishFeedbackSubmitted sends feedback.submitted. The event carries a new
	// event_id and no patient ID.
	PublishFeedbackSubmitted(ctx context.Context, f domain.Feedback) error
}
