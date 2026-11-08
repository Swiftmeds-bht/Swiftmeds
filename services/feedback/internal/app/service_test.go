package app

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Swiftmeds-bht/Swiftmeds/services/feedback/internal/domain"
)

var ctx = context.Background()

func submit(f *fixture, holdID, userID string) (domain.Feedback, error) {
	return f.svc.Submit(ctx, SubmitInput{HoldID: holdID, UserID: userID, WasInStock: true, Rating: 4, Comment: "Quick"})
}

func TestSubmitStoresFeedbackForACollectedHold(t *testing.T) {
	f := newFixture(t)

	got, err := submit(f, "h-1", "u-1")
	if err != nil {
		t.Fatal(err)
	}

	want := domain.Feedback{
		ID: "f-1", HoldID: "h-1", PharmacyID: "ph-1", ProductID: "med-1",
		WasInStock: true, Rating: 4, Comment: "Quick", CreatedAt: t0,
	}
	if got != want {
		t.Errorf("feedback = %+v, want %+v", got, want)
	}
	if stored := f.stored(t); len(stored) != 1 || stored[0] != want {
		t.Errorf("stored = %+v, want [%+v]", stored, want)
	}
}

// "Event published with event_id": the app asks the publisher to send exactly one
// event for the stored feedback. The adapter test checks the event_id itself.
func TestSubmitPublishesOneEventForTheStoredFeedback(t *testing.T) {
	f := newFixture(t)

	got, err := submit(f, "h-1", "u-1")
	if err != nil {
		t.Fatal(err)
	}

	if len(f.events.published) != 1 || f.events.published[0] != got {
		t.Errorf("published = %+v, want exactly [%+v]", f.events.published, got)
	}
}

// "Feedback without a collected hold is rejected."
func TestSubmitRejectsAHoldThatWasNotCollected(t *testing.T) {
	f := newFixture(t)

	_, err := submit(f, "h-2", "u-1")

	is(t, err, domain.ErrHoldNotCollected)
	if len(f.stored(t)) != 0 || len(f.events.published) != 0 {
		t.Errorf("stored %d, published %d, want nothing", len(f.stored(t)), len(f.events.published))
	}
}

func TestSubmitRejectsAnUnknownHold(t *testing.T) {
	f := newFixture(t)

	_, err := submit(f, "h-999", "u-1")

	is(t, err, domain.ErrHoldNotFound)
	if len(f.events.published) != 0 {
		t.Errorf("published %d events, want 0", len(f.events.published))
	}
}

func TestSubmitRejectsAnotherPatientsHold(t *testing.T) {
	f := newFixture(t)

	_, err := submit(f, "h-1", "u-2")

	is(t, err, domain.ErrNotHoldOwner)
	if len(f.stored(t)) != 0 || len(f.events.published) != 0 {
		t.Errorf("stored %d, published %d, want nothing", len(f.stored(t)), len(f.events.published))
	}
}

func TestSubmitTwiceForOneHoldIsRejectedAndPublishesOnce(t *testing.T) {
	f := newFixture(t)
	if _, err := submit(f, "h-1", "u-1"); err != nil {
		t.Fatal(err)
	}

	_, err := f.svc.Submit(ctx, SubmitInput{HoldID: "h-1", UserID: "u-1", WasInStock: false, Rating: 1})

	is(t, err, domain.ErrAlreadySubmitted)
	if len(f.stored(t)) != 1 {
		t.Errorf("stored %d feedback, want 1", len(f.stored(t)))
	}
	if len(f.events.published) != 1 {
		t.Errorf("published %d events, want 1: a refused second feedback sends nothing", len(f.events.published))
	}
	if !f.stored(t)[0].WasInStock {
		t.Error("the first feedback was overwritten by the second")
	}
}

func TestSubmitStoresNothingWhenThePublishFails(t *testing.T) {
	f := newFixture(t)
	f.events.err = errors.New("rabbitmq: connection closed")

	_, err := submit(f, "h-1", "u-1")

	is(t, err, domain.ErrPublishFailed)
	if len(f.stored(t)) != 0 {
		t.Errorf("stored %d feedback, want 0: an event that failed must not leave feedback behind", len(f.stored(t)))
	}

	// The patient can simply try again.
	f.events.err = nil
	if _, err := submit(f, "h-1", "u-1"); err != nil {
		t.Fatalf("retry after a failed publish: %v", err)
	}
	if len(f.stored(t)) != 1 || len(f.events.published) != 1 {
		t.Errorf("after the retry: stored %d, published %d, want 1 and 1", len(f.stored(t)), len(f.events.published))
	}
}

func TestSubmitPassesOnWhenHoldsIsUnavailable(t *testing.T) {
	f := newFixture(t)
	f.holds.err = domain.ErrHoldsUnavailable

	_, err := submit(f, "h-1", "u-1")

	is(t, err, domain.ErrHoldsUnavailable)
	if len(f.events.published) != 0 {
		t.Errorf("published %d events, want 0", len(f.events.published))
	}
}

func TestSubmitValidatesTheRequestBeforeCallingHolds(t *testing.T) {
	cases := []struct {
		name string
		in   SubmitInput
		want error
	}{
		{"no hold id", SubmitInput{UserID: "u-1"}, domain.ErrMissingHoldID},
		{"no user id", SubmitInput{HoldID: "h-1"}, domain.ErrMissingUserID},
		{"rating below zero", SubmitInput{HoldID: "h-1", UserID: "u-1", Rating: -1}, domain.ErrInvalidRating},
		{"rating above five", SubmitInput{HoldID: "h-1", UserID: "u-1", Rating: 6}, domain.ErrInvalidRating},
		{"comment too long", SubmitInput{HoldID: "h-1", UserID: "u-1", Comment: strings.Repeat("a", 501)}, domain.ErrCommentTooLong},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)

			_, err := f.svc.Submit(ctx, tc.in)

			is(t, err, tc.want)
			if f.holds.calls != 0 {
				t.Errorf("Holds was called %d times, want 0", f.holds.calls)
			}
		})
	}
}

func TestSubmitAcceptsNoRatingAndTheLongestComment(t *testing.T) {
	f := newFixture(t)

	got, err := f.svc.Submit(ctx, SubmitInput{HoldID: "h-1", UserID: "u-1", Comment: strings.Repeat("a", 500)})
	if err != nil {
		t.Fatal(err)
	}
	if got.Rating != 0 || len(got.Comment) != 500 {
		t.Errorf("rating %d, comment length %d, want 0 and 500", got.Rating, len(got.Comment))
	}
}

func TestSubmitFailsWhenNoIDCanBeMade(t *testing.T) {
	f := newFixture(t)
	boom := errors.New("no randomness")
	f.svc = NewService(f.repo, f.holds, f.events, func() time.Time { return t0 },
		WithIDGenerator(func() (string, error) { return "", boom }))

	_, err := submit(f, "h-1", "u-1")

	is(t, err, boom)
}

func TestSubmitWithTheDefaultIDGeneratorMakesUUIDs(t *testing.T) {
	f := newFixture(t)
	svc := NewService(f.repo, f.holds, f.events, func() time.Time { return t0 })

	got, err := svc.Submit(ctx, SubmitInput{HoldID: "h-1", UserID: "u-1"})
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(got.ID, "-")
	if len(got.ID) != 36 || len(parts) != 5 || got.ID[14] != '4' {
		t.Errorf("feedback id %q is not a version 4 UUID", got.ID)
	}
}

// --- ListPharmacyFeedback ---

// seed stores n feedback for ph-1, one minute apart, oldest first: f-1 .. f-n.
func seed(t *testing.T, f *fixture, n int) {
	t.Helper()
	for i := 1; i <= n; i++ {
		fb := domain.Feedback{
			ID: "f-" + string(rune('0'+i)), HoldID: "h-" + string(rune('0'+i)),
			PharmacyID: "ph-1", ProductID: "med-1", WasInStock: true,
			CreatedAt: t0.Add(time.Duration(i) * time.Minute),
		}
		if err := f.repo.Save(ctx, fb, nil); err != nil {
			t.Fatal(err)
		}
	}
}

func ids(items []domain.Feedback) []string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.ID
	}
	return out
}

func TestListPharmacyFeedbackNeedsAPharmacy(t *testing.T) {
	f := newFixture(t)

	_, err := f.svc.ListPharmacyFeedback(ctx, ListQuery{})

	is(t, err, domain.ErrMissingPharmacyID)
}

func TestListPharmacyFeedbackPagesNewestFirst(t *testing.T) {
	f := newFixture(t)
	seed(t, f, 5)

	page1, err := f.svc.ListPharmacyFeedback(ctx, ListQuery{PharmacyID: "ph-1", PageSize: 2})
	if err != nil {
		t.Fatal(err)
	}
	if got := ids(page1.Feedback); strings.Join(got, ",") != "f-5,f-4" || page1.NextPageToken == "" {
		t.Fatalf("page 1 = %v token %q, want [f-5 f-4] and a token", got, page1.NextPageToken)
	}

	page2, err := f.svc.ListPharmacyFeedback(ctx, ListQuery{PharmacyID: "ph-1", PageSize: 2, PageToken: page1.NextPageToken})
	if err != nil {
		t.Fatal(err)
	}
	if got := ids(page2.Feedback); strings.Join(got, ",") != "f-3,f-2" || page2.NextPageToken == "" {
		t.Fatalf("page 2 = %v token %q, want [f-3 f-2] and a token", got, page2.NextPageToken)
	}

	page3, err := f.svc.ListPharmacyFeedback(ctx, ListQuery{PharmacyID: "ph-1", PageSize: 2, PageToken: page2.NextPageToken})
	if err != nil {
		t.Fatal(err)
	}
	if got := ids(page3.Feedback); strings.Join(got, ",") != "f-1" || page3.NextPageToken != "" {
		t.Errorf("page 3 = %v token %q, want [f-1] and no token", got, page3.NextPageToken)
	}
}

func TestListPharmacyFeedbackUsesTheDefaultAndMaximumPageSize(t *testing.T) {
	f := newFixture(t)
	seed(t, f, 5)

	res, err := f.svc.ListPharmacyFeedback(ctx, ListQuery{PharmacyID: "ph-1", PageSize: 0})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Feedback) != 5 || res.NextPageToken != "" {
		t.Errorf("default page: got %d items and token %q, want all 5 and no token", len(res.Feedback), res.NextPageToken)
	}
	if DefaultPageSize != 20 || MaxPageSize != 100 {
		t.Errorf("page sizes are %d and %d, want 20 and 100", DefaultPageSize, MaxPageSize)
	}
	if _, err := f.svc.ListPharmacyFeedback(ctx, ListQuery{PharmacyID: "ph-1", PageSize: 100000}); err != nil {
		t.Errorf("a huge page size should be capped, got %v", err)
	}
}

func TestListPharmacyFeedbackOnlyShowsThatPharmacy(t *testing.T) {
	f := newFixture(t)
	seed(t, f, 2)
	other := domain.Feedback{ID: "f-x", HoldID: "h-x", PharmacyID: "ph-2", ProductID: "med-1", CreatedAt: t0}
	if err := f.repo.Save(ctx, other, nil); err != nil {
		t.Fatal(err)
	}

	res, err := f.svc.ListPharmacyFeedback(ctx, ListQuery{PharmacyID: "ph-1"})
	if err != nil {
		t.Fatal(err)
	}
	if got := ids(res.Feedback); strings.Join(got, ",") != "f-2,f-1" {
		t.Errorf("feedback = %v, want [f-2 f-1]", got)
	}
}

func TestListPharmacyFeedbackRejectsABadPageToken(t *testing.T) {
	f := newFixture(t)

	for _, token := range []string{"not-base64!!", "bm9waXBl", "YWJjfGRlZg"} {
		_, err := f.svc.ListPharmacyFeedback(ctx, ListQuery{PharmacyID: "ph-1", PageToken: token})
		is(t, err, domain.ErrInvalidPageToken)
	}
}
