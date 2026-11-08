package grpc

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	commonv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/common/v1"
	feedbackv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/feedback/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/feedback/internal/adapters/memory"
	"github.com/Swiftmeds-bht/Swiftmeds/services/feedback/internal/app"
	"github.com/Swiftmeds-bht/Swiftmeds/services/feedback/internal/domain"
	"github.com/Swiftmeds-bht/Swiftmeds/services/feedback/internal/ports"
)

var t0 = time.Date(2026, 11, 8, 9, 0, 0, 0, time.UTC)

type fakeHolds struct {
	holds map[string]domain.Hold
	err   error
}

func (f fakeHolds) GetHold(_ context.Context, id string) (domain.Hold, error) {
	if f.err != nil {
		return domain.Hold{}, f.err
	}
	h, ok := f.holds[id]
	if !ok {
		return domain.Hold{}, domain.ErrHoldNotFound
	}
	return h, nil
}

type fakeEvents struct {
	count int
	err   error
}

func (f *fakeEvents) PublishFeedbackSubmitted(context.Context, domain.Feedback) error {
	if f.err != nil {
		return f.err
	}
	f.count++
	return nil
}

// brokenRepo fails like a database that is down.
type brokenRepo struct{ ports.FeedbackRepository }

func (brokenRepo) Save(context.Context, domain.Feedback, func(context.Context) error) error {
	return errors.New("pq: password authentication failed for user swiftmed")
}

func (brokenRepo) ListByPharmacy(context.Context, domain.ListFilter) ([]domain.Feedback, error) {
	return nil, errors.New("pq: connection refused")
}

type fixture struct {
	h      *Handler
	holds  *fakeHolds
	events *fakeEvents
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	f := &fixture{
		holds: &fakeHolds{holds: map[string]domain.Hold{
			"h-1": {ID: "h-1", UserID: "u-1", PharmacyID: "ph-1", ProductID: "med-1", Collected: true},
			"h-2": {ID: "h-2", UserID: "u-1", PharmacyID: "ph-1", ProductID: "med-2", Collected: false},
		}},
		events: &fakeEvents{},
	}
	f.h = NewHandler(app.NewService(memory.NewRepository(), f.holds, f.events, func() time.Time { return t0 }))
	return f
}

func submit(f *fixture, holdID, userID string, rating int32) (*feedbackv1.SubmitFeedbackResponse, error) {
	return f.h.SubmitFeedback(context.Background(), &feedbackv1.SubmitFeedbackRequest{
		HoldId: holdID, UserId: userID, WasInStock: true, Rating: rating, Comment: "Quick",
	})
}

func wantCode(t *testing.T, err error, want codes.Code) {
	t.Helper()
	if got := status.Code(err); got != want {
		t.Errorf("code = %v, want %v (err: %v)", got, want, err)
	}
}

func TestSubmitFeedbackReturnsTheStoredFeedback(t *testing.T) {
	f := newFixture(t)

	res, err := submit(f, "h-1", "u-1", 4)
	if err != nil {
		t.Fatal(err)
	}

	got := res.GetFeedback()
	if got.GetFeedbackId() == "" || got.GetHoldId() != "h-1" || got.GetPharmacyId() != "ph-1" ||
		got.GetProductId() != "med-1" || !got.GetWasInStock() || got.GetRating() != 4 ||
		got.GetComment() != "Quick" || !got.GetCreatedAt().AsTime().Equal(t0) {
		t.Errorf("feedback = %v", got)
	}
	if f.events.count != 1 {
		t.Errorf("events published = %d, want 1", f.events.count)
	}
}

func TestSubmitFeedbackMapsErrorsToStatusCodes(t *testing.T) {
	cases := []struct {
		name   string
		holdID string
		userID string
		rating int32
		prep   func(*fixture)
		want   codes.Code
	}{
		{"hold not collected", "h-2", "u-1", 3, nil, codes.FailedPrecondition},
		{"unknown hold", "h-9", "u-1", 3, nil, codes.NotFound},
		{"someone else's hold", "h-1", "u-2", 3, nil, codes.PermissionDenied},
		{"no hold id", "", "u-1", 3, nil, codes.InvalidArgument},
		{"no user id", "h-1", "", 3, nil, codes.InvalidArgument},
		{"rating out of range", "h-1", "u-1", 9, nil, codes.InvalidArgument},
		{"holds is down", "h-1", "u-1", 3, func(f *fixture) { f.holds.err = domain.ErrHoldsUnavailable }, codes.Unavailable},
		{"event could not be sent", "h-1", "u-1", 3, func(f *fixture) { f.events.err = errors.New("bus down") }, codes.Unavailable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			if tc.prep != nil {
				tc.prep(f)
			}

			_, err := submit(f, tc.holdID, tc.userID, tc.rating)

			wantCode(t, err, tc.want)
		})
	}
}

func TestSubmitFeedbackTwiceReturnsAlreadyExists(t *testing.T) {
	f := newFixture(t)
	if _, err := submit(f, "h-1", "u-1", 4); err != nil {
		t.Fatal(err)
	}

	_, err := submit(f, "h-1", "u-1", 2)

	wantCode(t, err, codes.AlreadyExists)
	if f.events.count != 1 {
		t.Errorf("events published = %d, want 1", f.events.count)
	}
}

func TestSubmitFeedbackDoesNotLeakInternalErrors(t *testing.T) {
	f := newFixture(t)
	f.h = NewHandler(app.NewService(brokenRepo{}, f.holds, f.events, func() time.Time { return t0 }))

	_, err := submit(f, "h-1", "u-1", 4)

	wantCode(t, err, codes.Internal)
	if msg := status.Convert(err).Message(); msg != "internal error" || strings.Contains(msg, "password") {
		t.Errorf("message = %q, want a plain internal error", msg)
	}
}

func TestListPharmacyFeedbackPages(t *testing.T) {
	f := newFixture(t)
	if _, err := submit(f, "h-1", "u-1", 4); err != nil {
		t.Fatal(err)
	}

	res, err := f.h.ListPharmacyFeedback(context.Background(), &feedbackv1.ListPharmacyFeedbackRequest{
		PharmacyId: "ph-1",
		Page:       &commonv1.PageRequest{PageSize: 10},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.GetFeedback()) != 1 || res.GetFeedback()[0].GetHoldId() != "h-1" || res.GetPage().GetNextPageToken() != "" {
		t.Errorf("response = %v, want one feedback and no next page", res)
	}
}

func TestListPharmacyFeedbackErrors(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	_, err := f.h.ListPharmacyFeedback(ctx, &feedbackv1.ListPharmacyFeedbackRequest{})
	wantCode(t, err, codes.InvalidArgument)

	_, err = f.h.ListPharmacyFeedback(ctx, &feedbackv1.ListPharmacyFeedbackRequest{
		PharmacyId: "ph-1", Page: &commonv1.PageRequest{PageToken: "garbage!"},
	})
	wantCode(t, err, codes.InvalidArgument)

	f.h = NewHandler(app.NewService(brokenRepo{}, f.holds, f.events, func() time.Time { return t0 }))
	_, err = f.h.ListPharmacyFeedback(ctx, &feedbackv1.ListPharmacyFeedbackRequest{PharmacyId: "ph-1"})
	wantCode(t, err, codes.Internal)
}
