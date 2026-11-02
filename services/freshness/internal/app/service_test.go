package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Swiftmeds-bht/Swiftmeds/services/freshness/internal/domain"
)

var errDB = errors.New("db down")

// fakeRepo is an in-memory RatingRepository for tests.
type fakeRepo struct {
	rows        map[domain.Key]domain.Rating
	updateCalls int
	err         error // returned by every method when set
}

func newFakeRepo() *fakeRepo { return &fakeRepo{rows: map[domain.Key]domain.Rating{}} }

func (f *fakeRepo) Save(_ context.Context, r domain.Rating) (domain.Rating, error) {
	if f.err != nil {
		return domain.Rating{}, f.err
	}
	if old, ok := f.rows[r.Key]; ok && !r.ConfirmedAt.After(old.ConfirmedAt) {
		return old, nil // an older confirmation never replaces a newer one
	}
	f.rows[r.Key] = r
	return r, nil
}

func (f *fakeRepo) GetMany(_ context.Context, keys []domain.Key) ([]domain.Rating, error) {
	if f.err != nil {
		return nil, f.err
	}
	var out []domain.Rating
	for _, k := range keys {
		if r, ok := f.rows[k]; ok {
			out = append(out, r)
		}
	}
	return out, nil
}

func (f *fakeRepo) All(context.Context) ([]domain.Rating, error) {
	if f.err != nil {
		return nil, f.err
	}
	var out []domain.Rating
	for _, r := range f.rows {
		out = append(out, r)
	}
	return out, nil
}

func (f *fakeRepo) UpdateConfidence(_ context.Context, ratings []domain.Rating) error {
	f.updateCalls++
	if f.err != nil {
		return f.err
	}
	for _, r := range ratings {
		f.rows[r.Key] = r
	}
	return nil
}

var t0 = time.Date(2026, 10, 25, 9, 0, 0, 0, time.UTC)

// newTestService returns a service whose clock starts at t0 and can be moved by changing *now.
func newTestService() (*Service, *fakeRepo, *time.Time) {
	repo := newFakeRepo()
	now := t0
	svc := NewService(repo, domain.DefaultThresholds(), func() time.Time { return now })
	return svc, repo, &now
}

func TestRecordConfirmationSetsConfidenceFromAge(t *testing.T) {
	cases := []struct {
		name string
		age  time.Duration
		want domain.Confidence
	}{
		{"confirmed now", 0, domain.High},
		{"just under the high edge", 6*time.Hour - time.Nanosecond, domain.High},
		{"on the high edge", 6 * time.Hour, domain.Medium},
		{"on the medium edge", 24 * time.Hour, domain.Low},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			svc, _, _ := newTestService()

			got, err := svc.RecordConfirmation(context.Background(), "ph-1", "med-1", t0.Add(-c.age))

			if err != nil {
				t.Fatalf("err = %v", err)
			}
			if got.Confidence != c.want {
				t.Errorf("confidence = %s, want %s", got.Confidence, c.want)
			}
		})
	}
}

func TestRecordConfirmationRejectsInvalidInput(t *testing.T) {
	svc, repo, _ := newTestService()

	_, err := svc.RecordConfirmation(context.Background(), "", "med-1", t0)

	if !errors.Is(err, domain.ErrMissingPharmacyID) {
		t.Fatalf("err = %v, want ErrMissingPharmacyID", err)
	}
	if len(repo.rows) != 0 {
		t.Error("an invalid confirmation was saved")
	}
}

func TestRecordConfirmationKeepsTheNewerTime(t *testing.T) {
	svc, _, _ := newTestService()
	ctx := context.Background()
	_, _ = svc.RecordConfirmation(ctx, "ph-1", "med-1", t0)

	got, err := svc.RecordConfirmation(ctx, "ph-1", "med-1", t0.Add(-time.Hour))

	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if !got.ConfirmedAt.Equal(t0) {
		t.Errorf("confirmed_at = %v, want %v (the older one must not win)", got.ConfirmedAt, t0)
	}
}

func TestRecordConfirmationWrapsRepositoryErrors(t *testing.T) {
	svc, repo, _ := newTestService()
	repo.err = errDB

	_, err := svc.RecordConfirmation(context.Background(), "ph-1", "med-1", t0)

	if !errors.Is(err, errDB) {
		t.Fatalf("err = %v, want errDB", err)
	}
}

func TestGetConfidence(t *testing.T) {
	svc, _, _ := newTestService()
	ctx := context.Background()
	_, _ = svc.RecordConfirmation(ctx, "ph-1", "med-1", t0)

	got, err := svc.GetConfidence(ctx, "ph-1", "med-1")
	if err != nil || got.Confidence != domain.High {
		t.Fatalf("got (%+v, %v), want HIGH and no error", got, err)
	}

	if _, err := svc.GetConfidence(ctx, "ph-1", "other"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
	if _, err := svc.GetConfidence(ctx, "", "med-1"); !errors.Is(err, domain.ErrMissingPharmacyID) {
		t.Fatalf("err = %v, want ErrMissingPharmacyID", err)
	}
}

func TestGetConfidenceWrapsRepositoryErrors(t *testing.T) {
	svc, repo, _ := newTestService()
	repo.err = errDB

	if _, err := svc.GetConfidence(context.Background(), "ph-1", "med-1"); !errors.Is(err, errDB) {
		t.Fatalf("err = %v, want errDB", err)
	}
}

func TestBatchGetConfidenceSkipsKeysThatWereNeverConfirmed(t *testing.T) {
	svc, _, _ := newTestService()
	ctx := context.Background()
	_, _ = svc.RecordConfirmation(ctx, "ph-1", "med-1", t0)
	_, _ = svc.RecordConfirmation(ctx, "ph-2", "med-1", t0.Add(-7*time.Hour))

	got, err := svc.BatchGetConfidence(ctx, []domain.Key{
		{PharmacyID: "ph-1", ProductID: "med-1"},
		{PharmacyID: "ph-9", ProductID: "med-1"}, // never confirmed
		{PharmacyID: "ph-2", ProductID: "med-1"},
	})

	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(got) != 2 || got[0].Confidence != domain.High || got[1].Confidence != domain.Medium {
		t.Fatalf("got %+v, want 2 ratings: HIGH then MEDIUM", got)
	}
}

func TestBatchGetConfidenceWithNoKeys(t *testing.T) {
	svc, _, _ := newTestService()

	got, err := svc.BatchGetConfidence(context.Background(), nil)

	if err != nil || len(got) != 0 {
		t.Fatalf("got (%v, %v), want (empty, nil)", got, err)
	}
}

func TestBatchGetConfidenceRejectsAnInvalidKey(t *testing.T) {
	svc, _, _ := newTestService()

	_, err := svc.BatchGetConfidence(context.Background(), []domain.Key{
		{PharmacyID: "ph-1", ProductID: "med-1"},
		{PharmacyID: "ph-1", ProductID: ""},
	})

	if !errors.Is(err, domain.ErrMissingProductID) {
		t.Fatalf("err = %v, want ErrMissingProductID", err)
	}
}

func TestBatchGetConfidenceWrapsRepositoryErrors(t *testing.T) {
	svc, repo, _ := newTestService()
	repo.err = errDB

	_, err := svc.BatchGetConfidence(context.Background(), []domain.Key{{PharmacyID: "ph-1", ProductID: "med-1"}})

	if !errors.Is(err, errDB) {
		t.Fatalf("err = %v, want errDB", err)
	}
}

func TestDecayMovesConfidenceDownAtEachEdge(t *testing.T) {
	svc, _, _ := newTestService()
	ctx := context.Background()
	_, _ = svc.RecordConfirmation(ctx, "ph-1", "med-1", t0)

	steps := []struct {
		asOf        time.Time
		wantChanged int
		want        domain.Confidence
	}{
		{t0.Add(6*time.Hour - time.Nanosecond), 0, domain.High},
		{t0.Add(6 * time.Hour), 1, domain.Medium},
		{t0.Add(24*time.Hour - time.Nanosecond), 0, domain.Medium},
		{t0.Add(24 * time.Hour), 1, domain.Low},
	}
	for _, s := range steps {
		changed, err := svc.Decay(ctx, s.asOf)
		if err != nil || changed != s.wantChanged {
			t.Fatalf("Decay(%v) = (%d, %v), want (%d, nil)", s.asOf, changed, err, s.wantChanged)
		}
		got, _ := svc.GetConfidence(ctx, "ph-1", "med-1")
		if got.Confidence != s.want {
			t.Fatalf("after Decay(%v) confidence = %s, want %s", s.asOf, got.Confidence, s.want)
		}
	}
}

func TestDecayIsIdempotent(t *testing.T) {
	svc, repo, _ := newTestService()
	ctx := context.Background()
	_, _ = svc.RecordConfirmation(ctx, "ph-1", "med-1", t0)
	_, _ = svc.RecordConfirmation(ctx, "ph-2", "med-1", t0)
	asOf := t0.Add(10 * time.Hour)

	first, _ := svc.Decay(ctx, asOf)
	second, err := svc.Decay(ctx, asOf)

	if first != 2 || second != 0 || err != nil {
		t.Fatalf("first = %d, second = %d, err = %v; want 2, 0, nil", first, second, err)
	}
	if repo.updateCalls != 1 {
		t.Errorf("update calls = %d, want 1 (nothing to write the second time)", repo.updateCalls)
	}
}

func TestDecayUsesTheClockWhenAsOfIsZero(t *testing.T) {
	svc, _, now := newTestService()
	ctx := context.Background()
	_, _ = svc.RecordConfirmation(ctx, "ph-1", "med-1", t0)
	*now = t0.Add(25 * time.Hour)

	changed, err := svc.Decay(ctx, time.Time{})

	if err != nil || changed != 1 {
		t.Fatalf("Decay = (%d, %v), want (1, nil)", changed, err)
	}
	got, _ := svc.GetConfidence(ctx, "ph-1", "med-1")
	if got.Confidence != domain.Low {
		t.Errorf("confidence = %s, want LOW", got.Confidence)
	}
}

func TestDecayWithNoRowsDoesNothing(t *testing.T) {
	svc, repo, _ := newTestService()

	changed, err := svc.Decay(context.Background(), t0)

	if err != nil || changed != 0 || repo.updateCalls != 0 {
		t.Fatalf("got (%d, %v), updateCalls = %d; want (0, nil), 0", changed, err, repo.updateCalls)
	}
}

func TestDecayWrapsRepositoryErrors(t *testing.T) {
	svc, repo, _ := newTestService()
	_, _ = svc.RecordConfirmation(context.Background(), "ph-1", "med-1", t0)

	repo.err = errDB
	if _, err := svc.Decay(context.Background(), t0.Add(10*time.Hour)); !errors.Is(err, errDB) {
		t.Fatalf("listing: err = %v, want errDB", err)
	}
}

// The listing works but the write fails.
type failingUpdateRepo struct{ *fakeRepo }

func (failingUpdateRepo) UpdateConfidence(context.Context, []domain.Rating) error { return errDB }

func TestDecayWrapsUpdateErrors(t *testing.T) {
	repo := failingUpdateRepo{newFakeRepo()}
	svc := NewService(repo, domain.DefaultThresholds(), func() time.Time { return t0 })
	_, _ = svc.RecordConfirmation(context.Background(), "ph-1", "med-1", t0)

	_, err := svc.Decay(context.Background(), t0.Add(10*time.Hour))

	if !errors.Is(err, errDB) {
		t.Fatalf("err = %v, want errDB", err)
	}
}

func TestServiceUsesTheThresholdsItWasGiven(t *testing.T) {
	th, _ := domain.NewThresholds(time.Hour, 2*time.Hour)
	svc := NewService(newFakeRepo(), th, func() time.Time { return t0 })

	got, _ := svc.RecordConfirmation(context.Background(), "ph-1", "med-1", t0.Add(-time.Hour))

	if got.Confidence != domain.Medium {
		t.Errorf("confidence = %s, want MEDIUM with a 1 hour high threshold", got.Confidence)
	}
}
