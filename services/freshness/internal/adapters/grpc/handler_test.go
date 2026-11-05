package grpc

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	commonv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/common/v1"
	freshnessv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/freshness/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/freshness/internal/app"
	"github.com/Swiftmeds-bht/Swiftmeds/services/freshness/internal/domain"
)

// memRepo is a small in-memory RatingRepository for the handler tests.
type memRepo struct {
	rows map[domain.Key]domain.Rating
	err  error // returned by every method when set
}

func (m *memRepo) Save(_ context.Context, r domain.Rating) (domain.Rating, error) {
	if m.err != nil {
		return domain.Rating{}, m.err
	}
	if old, ok := m.rows[r.Key]; ok && !r.ConfirmedAt.After(old.ConfirmedAt) {
		return old, nil
	}
	m.rows[r.Key] = r
	return r, nil
}

func (m *memRepo) GetMany(_ context.Context, keys []domain.Key) ([]domain.Rating, error) {
	if m.err != nil {
		return nil, m.err
	}
	var out []domain.Rating
	for _, k := range keys {
		if r, ok := m.rows[k]; ok {
			out = append(out, r)
		}
	}
	return out, nil
}

func (m *memRepo) All(context.Context) ([]domain.Rating, error) {
	if m.err != nil {
		return nil, m.err
	}
	var out []domain.Rating
	for _, r := range m.rows {
		out = append(out, r)
	}
	return out, nil
}

func (m *memRepo) UpdateConfidence(_ context.Context, ratings []domain.Rating) error {
	if m.err != nil {
		return m.err
	}
	for _, r := range ratings {
		m.rows[r.Key] = r
	}
	return nil
}

var t0 = time.Date(2026, 10, 25, 9, 0, 0, 0, time.UTC)

// newHandler returns a handler whose clock starts at t0 and moves when *now changes.
func newHandler() (*Handler, *memRepo, *time.Time) {
	repo := &memRepo{rows: map[domain.Key]domain.Rating{}}
	now := t0
	svc := app.NewService(repo, domain.DefaultThresholds(), func() time.Time { return now })
	return NewHandler(svc), repo, &now
}

func key(pharmacyID, productID string) *commonv1.StockKey {
	return &commonv1.StockKey{PharmacyId: pharmacyID, ProductId: productID}
}

func TestRecordThenGetRoundTrip(t *testing.T) {
	h, _, _ := newHandler()
	ctx := context.Background()

	rec, err := h.RecordConfirmation(ctx, &freshnessv1.RecordConfirmationRequest{
		Key: key("ph-1", "med-1"), ConfirmedAt: timestamppb.New(t0),
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := rec.GetFreshness().GetScore(); got != 1.0 {
		t.Errorf("score after recording = %v, want 1.0", got)
	}

	resp, err := h.GetFreshness(ctx, &freshnessv1.GetFreshnessRequest{
		Keys: []*commonv1.StockKey{key("ph-1", "med-1"), key("ph-1", "never-confirmed")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.GetScores()) != 1 {
		t.Fatalf("got %d scores, want 1 (unknown key left out)", len(resp.GetScores()))
	}
	s := resp.GetScores()[0]
	if s.GetKey().GetPharmacyId() != "ph-1" || s.GetKey().GetProductId() != "med-1" {
		t.Errorf("key = %v, want ph-1 / med-1", s.GetKey())
	}
	if s.GetScore() != 1.0 || !s.GetConfirmedAt().AsTime().Equal(t0) {
		t.Errorf("got score %v, confirmed_at %v; want 1.0, %v", s.GetScore(), s.GetConfirmedAt().AsTime(), t0)
	}
}

func TestRecordConfirmationInvalidInputIsInvalidArgument(t *testing.T) {
	cases := []struct {
		name string
		req  *freshnessv1.RecordConfirmationRequest
	}{
		{"no key", &freshnessv1.RecordConfirmationRequest{ConfirmedAt: timestamppb.New(t0)}},
		{"no product", &freshnessv1.RecordConfirmationRequest{Key: key("ph-1", ""), ConfirmedAt: timestamppb.New(t0)}},
		{"no confirmed_at", &freshnessv1.RecordConfirmationRequest{Key: key("ph-1", "med-1")}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h, repo, _ := newHandler()

			_, err := h.RecordConfirmation(context.Background(), c.req)

			if status.Code(err) != codes.InvalidArgument {
				t.Fatalf("code = %v, want InvalidArgument", status.Code(err))
			}
			if len(repo.rows) != 0 {
				t.Error("an invalid request was saved")
			}
		})
	}
}

func TestGetFreshnessInvalidKeyIsInvalidArgument(t *testing.T) {
	h, _, _ := newHandler()

	_, err := h.GetFreshness(context.Background(), &freshnessv1.GetFreshnessRequest{
		Keys: []*commonv1.StockKey{key("ph-1", "med-1"), nil},
	})

	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("code = %v, want InvalidArgument", status.Code(err))
	}
}

func TestGetFreshnessWithNoKeysReturnsNothing(t *testing.T) {
	h, _, _ := newHandler()

	resp, err := h.GetFreshness(context.Background(), &freshnessv1.GetFreshnessRequest{})

	if err != nil || len(resp.GetScores()) != 0 {
		t.Fatalf("got (%v, %v), want (no scores, nil)", resp, err)
	}
}

func TestApplyDecayLowersScoresAndIsIdempotent(t *testing.T) {
	h, _, _ := newHandler()
	ctx := context.Background()
	_, _ = h.RecordConfirmation(ctx, &freshnessv1.RecordConfirmationRequest{
		Key: key("ph-1", "med-1"), ConfirmedAt: timestamppb.New(t0),
	})
	asOf := &freshnessv1.ApplyDecayRequest{AsOf: timestamppb.New(t0.Add(6 * time.Hour))}

	first, err := h.ApplyDecay(ctx, asOf)
	if err != nil || first.GetUpdatedCount() != 1 {
		t.Fatalf("first ApplyDecay = (%v, %v), want updated_count 1", first, err)
	}
	second, err := h.ApplyDecay(ctx, asOf)
	if err != nil || second.GetUpdatedCount() != 0 {
		t.Fatalf("second ApplyDecay = (%v, %v), want updated_count 0", second, err)
	}

	resp, _ := h.GetFreshness(ctx, &freshnessv1.GetFreshnessRequest{Keys: []*commonv1.StockKey{key("ph-1", "med-1")}})
	if got := resp.GetScores()[0].GetScore(); got != 0.6 {
		t.Errorf("score after decay = %v, want 0.6 (MEDIUM)", got)
	}
}

func TestApplyDecayWithoutAsOfUsesTheClock(t *testing.T) {
	h, _, now := newHandler()
	ctx := context.Background()
	_, _ = h.RecordConfirmation(ctx, &freshnessv1.RecordConfirmationRequest{
		Key: key("ph-1", "med-1"), ConfirmedAt: timestamppb.New(t0),
	})
	*now = t0.Add(25 * time.Hour)

	resp, err := h.ApplyDecay(ctx, &freshnessv1.ApplyDecayRequest{})

	if err != nil || resp.GetUpdatedCount() != 1 {
		t.Fatalf("ApplyDecay = (%v, %v), want updated_count 1", resp, err)
	}
}

func TestRepositoryFailureIsInternalWithASafeMessage(t *testing.T) {
	h, repo, _ := newHandler()
	repo.err = errors.New("password authentication failed for user swiftmed")
	ctx := context.Background()

	_, errRecord := h.RecordConfirmation(ctx, &freshnessv1.RecordConfirmationRequest{
		Key: key("ph-1", "med-1"), ConfirmedAt: timestamppb.New(t0),
	})
	_, errGet := h.GetFreshness(ctx, &freshnessv1.GetFreshnessRequest{Keys: []*commonv1.StockKey{key("ph-1", "med-1")}})
	_, errDecay := h.ApplyDecay(ctx, &freshnessv1.ApplyDecayRequest{})

	for name, err := range map[string]error{"RecordConfirmation": errRecord, "GetFreshness": errGet, "ApplyDecay": errDecay} {
		if status.Code(err) != codes.Internal {
			t.Errorf("%s: code = %v, want Internal", name, status.Code(err))
		}
		if strings.Contains(err.Error(), "password") {
			t.Errorf("%s: database details leaked to the caller: %v", name, err)
		}
	}
}
