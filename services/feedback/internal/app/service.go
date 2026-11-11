// Package app holds Feedback's use cases. It depends only on domain and ports.
package app

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"time"

	"github.com/Swiftmeds-bht/Swiftmeds/services/feedback/internal/domain"
	"github.com/Swiftmeds-bht/Swiftmeds/services/feedback/internal/ports"
)

// DefaultPageSize is the page size of ListPharmacyFeedback when the caller does not ask for one.
const DefaultPageSize = 20

// MaxPageSize is the largest page ListPharmacyFeedback returns.
const MaxPageSize = 100

// Service implements the Feedback use cases.
type Service struct {
	repo   ports.FeedbackRepository
	holds  ports.HoldReader
	events ports.EventPublisher
	now    func() time.Time
	newID  func() (string, error)
}

// Option changes how a Service is built. Tests use them to fix IDs.
type Option func(*Service)

// WithIDGenerator replaces the feedback ID generator (a random UUID by default).
func WithIDGenerator(f func() (string, error)) Option {
	return func(s *Service) { s.newID = f }
}

// NewService wires the use cases. Pass time.Now in production and a fixed clock in tests.
func NewService(repo ports.FeedbackRepository, holds ports.HoldReader, events ports.EventPublisher, now func() time.Time, opts ...Option) *Service {
	s := &Service{repo: repo, holds: holds, events: events, now: now, newID: newUUID}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// SubmitInput is what a patient sends.
type SubmitInput struct {
	HoldID     string
	UserID     string
	WasInStock bool
	// Rating is 1 to 5, or 0 if not given.
	Rating  int
	Comment string
}

// Submit stores a patient's feedback for a collected hold and publishes
// feedback.submitted. It fails with:
//
//   - domain.ErrHoldNotFound, domain.ErrNotHoldOwner or domain.ErrHoldNotCollected
//     when the hold is not one the patient collected,
//   - domain.ErrAlreadySubmitted when the hold already has its one feedback,
//   - domain.ErrPublishFailed when the event could not be sent. Nothing is
//     stored then, so the patient can submit again.
//
// The feedback and its event go together: the event is sent before the store
// becomes final. The rare case left is an event that was sent but a store that
// then failed to commit; a retry would send a second event for the same hold.
// Consumers already ignore repeated event IDs, but not a second event with a
// new ID. An outbox table would close that gap; it is not built yet.
func (s *Service) Submit(ctx context.Context, in SubmitInput) (domain.Feedback, error) {
	if in.HoldID == "" {
		return domain.Feedback{}, domain.ErrMissingHoldID
	}
	if in.UserID == "" {
		return domain.Feedback{}, domain.ErrMissingUserID
	}
	// Cheap checks first: a bad rating never costs a call to Holds.
	if err := domain.ValidateAnswer(in.Rating, in.Comment); err != nil {
		return domain.Feedback{}, err
	}

	hold, err := s.holds.GetHold(ctx, in.HoldID)
	if err != nil {
		return domain.Feedback{}, fmt.Errorf("read hold: %w", err)
	}
	id, err := s.newID()
	if err != nil {
		return domain.Feedback{}, err
	}
	f, err := domain.NewFeedback(id, hold, in.UserID, in.WasInStock, in.Rating, in.Comment, s.now())
	if err != nil {
		return domain.Feedback{}, err
	}

	err = s.repo.Save(ctx, f, func(ctx context.Context) error {
		if err := s.events.PublishFeedbackSubmitted(ctx, f); err != nil {
			return fmt.Errorf("%w: %v", domain.ErrPublishFailed, err)
		}
		return nil
	})
	if err != nil {
		return domain.Feedback{}, fmt.Errorf("submit feedback: %w", err)
	}
	return f, nil
}

// ListQuery asks for the feedback of one pharmacy.
type ListQuery struct {
	PharmacyID string
	PageSize   int
	PageToken  string
}

// ListResult is one page of feedback, newest first.
type ListResult struct {
	Feedback []domain.Feedback
	// NextPageToken is empty on the last page.
	NextPageToken string
}

// ListPharmacyFeedback lists a pharmacy's feedback, newest first.
func (s *Service) ListPharmacyFeedback(ctx context.Context, q ListQuery) (ListResult, error) {
	if q.PharmacyID == "" {
		return ListResult{}, domain.ErrMissingPharmacyID
	}
	after, err := decodeCursor(q.PageToken)
	if err != nil {
		return ListResult{}, err
	}
	size := q.PageSize
	if size <= 0 {
		size = DefaultPageSize
	}
	if size > MaxPageSize {
		size = MaxPageSize
	}

	// One more than the page, to find out whether there is a next page.
	items, err := s.repo.ListByPharmacy(ctx, domain.ListFilter{
		PharmacyID: q.PharmacyID,
		After:      after,
		Limit:      size + 1,
	})
	if err != nil {
		return ListResult{}, fmt.Errorf("list feedback: %w", err)
	}
	if len(items) <= size {
		return ListResult{Feedback: items}, nil
	}
	page := items[:size]
	last := page[size-1]
	return ListResult{
		Feedback:      page,
		NextPageToken: encodeCursor(domain.Cursor{CreatedAt: last.CreatedAt, ID: last.ID}),
	}, nil
}

func newUUID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", errors.Join(errors.New("generate feedback id"), err)
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:]), nil
}
