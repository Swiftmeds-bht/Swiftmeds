package app

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/Swiftmeds-bht/Swiftmeds/services/feedback/internal/adapters/memory"
	"github.com/Swiftmeds-bht/Swiftmeds/services/feedback/internal/domain"
)

var t0 = time.Date(2026, 11, 8, 9, 0, 0, 0, time.UTC)

// fakeHolds is the Holds service: a map of holds, and an error to return instead.
type fakeHolds struct {
	holds map[string]domain.Hold
	err   error
	calls int
}

func (f *fakeHolds) GetHold(_ context.Context, id string) (domain.Hold, error) {
	f.calls++
	if f.err != nil {
		return domain.Hold{}, f.err
	}
	h, ok := f.holds[id]
	if !ok {
		return domain.Hold{}, domain.ErrHoldNotFound
	}
	return h, nil
}

// fakeEvents records the events that were published.
type fakeEvents struct {
	published []domain.Feedback
	err       error
}

func (f *fakeEvents) PublishFeedbackSubmitted(_ context.Context, fb domain.Feedback) error {
	if f.err != nil {
		return f.err
	}
	f.published = append(f.published, fb)
	return nil
}

// fixture is a Service on an in-memory repository, a fake Holds and a fake event bus.
type fixture struct {
	svc    *Service
	repo   *memory.Repository
	holds  *fakeHolds
	events *fakeEvents
	now    time.Time
}

// newFixture has two holds: h-1, collected by u-1 at ph-1, and h-2, still ACTIVE for u-1.
func newFixture(t *testing.T) *fixture {
	t.Helper()
	f := &fixture{
		repo: memory.NewRepository(),
		holds: &fakeHolds{holds: map[string]domain.Hold{
			"h-1": {ID: "h-1", UserID: "u-1", PharmacyID: "ph-1", ProductID: "med-1", Collected: true},
			"h-2": {ID: "h-2", UserID: "u-1", PharmacyID: "ph-1", ProductID: "med-2", Collected: false},
			"h-3": {ID: "h-3", UserID: "u-3", PharmacyID: "ph-1", ProductID: "med-3", Collected: true},
		}},
		events: &fakeEvents{},
		now:    t0,
	}
	n := 0
	ids := func() (string, error) {
		n++
		return fmt.Sprintf("f-%d", n), nil
	}
	f.svc = NewService(f.repo, f.holds, f.events, func() time.Time { return f.now }, WithIDGenerator(ids))
	return f
}

// stored returns every feedback of ph-1 straight from the repository.
func (f *fixture) stored(t *testing.T) []domain.Feedback {
	t.Helper()
	items, err := f.repo.ListByPharmacy(context.Background(), domain.ListFilter{PharmacyID: "ph-1"})
	if err != nil {
		t.Fatal(err)
	}
	return items
}

func is(t *testing.T, err, want error) {
	t.Helper()
	if !errors.Is(err, want) {
		t.Errorf("err = %v, want %v", err, want)
	}
}
