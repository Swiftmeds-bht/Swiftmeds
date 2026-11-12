package app

import (
	"context"
	"errors"
	"math"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Swiftmeds-bht/Swiftmeds/pkg/config"
	"github.com/Swiftmeds-bht/Swiftmeds/services/trust/internal/adapters/memory"
	"github.com/Swiftmeds-bht/Swiftmeds/services/trust/internal/domain"
)

var t0 = time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)

type clock struct{ now time.Time }

func (c *clock) Now() time.Time { return c.now }

func newService(t *testing.T) (*Service, *clock) {
	t.Helper()
	c := &clock{now: t0}
	svc, err := NewService(memory.NewRepository(), domain.DefaultParams, c.Now)
	if err != nil {
		t.Fatal(err)
	}
	return svc, c
}

func feedback(id, pharmacy string, inStock bool) domain.Feedback {
	return domain.Feedback{ID: id, PharmacyID: pharmacy, WasInStock: inStock, OccurredAt: t0}
}

func mustHandle(t *testing.T, svc *Service, f domain.Feedback, want bool) {
	t.Helper()
	applied, err := svc.HandleFeedbackSubmitted(context.Background(), f)
	if err != nil {
		t.Fatal(err)
	}
	if applied != want {
		t.Fatalf("HandleFeedbackSubmitted(%s) = %v, want %v", f.ID, applied, want)
	}
}

func mustGet(t *testing.T, svc *Service, id string) domain.Trust {
	t.Helper()
	got, err := svc.GetTrust(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestNewServiceRejectsInvalidParams(t *testing.T) {
	if _, err := NewService(memory.NewRepository(), domain.Params{}, time.Now); !errors.Is(err, domain.ErrInvalidParams) {
		t.Errorf("NewService with zero params = %v, want ErrInvalidParams", err)
	}
}

func TestNewPharmacyIsNeutral(t *testing.T) {
	svc, _ := newService(t)
	got := mustGet(t, svc, "ph-new")
	if want := (domain.Trust{PharmacyID: "ph-new", Score: domain.NeutralScore}); got != want {
		t.Errorf("GetTrust = %+v, want %+v", got, want)
	}
}

func TestFeedbackMovesTheScore(t *testing.T) {
	svc, _ := newService(t)
	mustHandle(t, svc, feedback("fb-1", "ph-bad", false), true)
	mustHandle(t, svc, feedback("fb-2", "ph-good", true), true)

	bad, good := mustGet(t, svc, "ph-bad"), mustGet(t, svc, "ph-good")
	if math.Abs(bad.Score-0.375) > 1e-12 || math.Abs(good.Score-0.625) > 1e-12 {
		t.Errorf("scores = %v (not in stock) and %v (in stock), want 0.375 and 0.625", bad.Score, good.Score)
	}
	if bad.FeedbackCount != 1 || bad.InStockCount != 0 || !bad.UpdatedAt.Equal(t0) {
		t.Errorf("ph-bad = %+v", bad)
	}
	if good.FeedbackCount != 1 || good.InStockCount != 1 {
		t.Errorf("ph-good = %+v", good)
	}
}

func TestTheSameFeedbackCountsOnce(t *testing.T) {
	svc, _ := newService(t)
	mustHandle(t, svc, feedback("fb-1", "ph-1", false), true)
	before := mustGet(t, svc, "ph-1")
	mustHandle(t, svc, feedback("fb-1", "ph-1", false), false)
	mustHandle(t, svc, feedback(" fb-1 ", "ph-1", false), false) // trimmed to the same ID
	if after := mustGet(t, svc, "ph-1"); after != before {
		t.Errorf("after redelivery %+v, before %+v", after, before)
	}
}

func TestScoreUsesTheClock(t *testing.T) {
	svc, c := newService(t)
	for _, id := range []string{"fb-1", "fb-2", "fb-3"} {
		mustHandle(t, svc, feedback(id, "ph-1", false), true)
	}
	now := mustGet(t, svc, "ph-1").Score
	c.now = t0.Add(domain.DefaultParams.HalfLife)
	later := mustGet(t, svc, "ph-1").Score
	if !(now < later && later < domain.NeutralScore) {
		t.Errorf("score now %v, one half-life later %v: want it to drift up towards 0.5", now, later)
	}
}

func TestInvalidFeedbackIsRejected(t *testing.T) {
	svc, _ := newService(t)
	tests := map[string]struct {
		f    domain.Feedback
		want error
	}{
		"no pharmacy":    {feedback("fb-1", " ", true), domain.ErrMissingPharmacyID},
		"no feedback id": {feedback("", "ph-1", true), domain.ErrMissingFeedbackID},
		"no time":        {domain.Feedback{ID: "fb-1", PharmacyID: "ph-1"}, domain.ErrMissingOccurredAt},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := svc.HandleFeedbackSubmitted(context.Background(), tt.f); !errors.Is(err, tt.want) {
				t.Errorf("err = %v, want %v", err, tt.want)
			}
		})
	}
	if got := mustGet(t, svc, "ph-1"); got.FeedbackCount != 0 {
		t.Errorf("rejected feedback was counted: %+v", got)
	}
}

func TestBatchGetTrust(t *testing.T) {
	svc, _ := newService(t)
	mustHandle(t, svc, feedback("fb-1", "ph-1", true), true)

	got, err := svc.BatchGetTrust(context.Background(), []string{"ph-unknown", " ph-1 ", "ph-1"})
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]string, len(got))
	for i, tr := range got {
		ids[i] = tr.PharmacyID
	}
	if !reflect.DeepEqual(ids, []string{"ph-unknown", "ph-1", "ph-1"}) {
		t.Fatalf("ids = %v, want one per request, in order", ids)
	}
	if got[0].Score != domain.NeutralScore || got[1].Score <= domain.NeutralScore || got[1] != got[2] {
		t.Errorf("scores = %+v", got)
	}

	empty, err := svc.BatchGetTrust(context.Background(), nil)
	if err != nil || empty == nil || len(empty) != 0 {
		t.Errorf("no IDs: %v, %v; want an empty list", empty, err)
	}
}

func TestBatchGetTrustRejectsBadRequests(t *testing.T) {
	svc, _ := newService(t)
	tooMany := strings.Split(strings.Repeat("ph,", domain.MaxBatch+1), ",")[:domain.MaxBatch+1]
	if _, err := svc.BatchGetTrust(context.Background(), tooMany); !errors.Is(err, domain.ErrTooManyPharmacies) {
		t.Errorf("too many: %v", err)
	}
	if _, err := svc.BatchGetTrust(context.Background(), []string{"ph-1", ""}); !errors.Is(err, domain.ErrMissingPharmacyID) {
		t.Errorf("blank ID: %v", err)
	}
	if _, err := svc.GetTrust(context.Background(), "  "); !errors.Is(err, domain.ErrMissingPharmacyID) {
		t.Errorf("GetTrust blank: %v", err)
	}
}

type brokenRepo struct{ err error }

func (b brokenRepo) ApplyFeedback(context.Context, domain.Feedback, func(domain.Record) domain.Record) (bool, error) {
	return false, b.err
}

func (b brokenRepo) Records(context.Context, []string) (map[string]domain.Record, error) {
	return nil, b.err
}

func TestRepositoryErrorsArePassedOn(t *testing.T) {
	boom := errors.New("database is down")
	svc, err := NewService(brokenRepo{boom}, domain.DefaultParams, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.HandleFeedbackSubmitted(context.Background(), feedback("fb-1", "ph-1", true)); !errors.Is(err, boom) {
		t.Errorf("HandleFeedbackSubmitted = %v, want the repository error", err)
	}
	if _, err := svc.GetTrust(context.Background(), "ph-1"); !errors.Is(err, boom) {
		t.Errorf("GetTrust = %v, want the repository error", err)
	}
}

func TestParamsFromConfig(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		want    domain.Params
		wantErr string
	}{
		{"defaults", map[string]string{}, domain.DefaultParams, ""},
		{"overrides", map[string]string{EnvHalfLife: "168h", EnvPriorWeight: "0"}, domain.Params{HalfLife: 168 * time.Hour, PriorWeight: 0}, ""},
		{"prior not a number", map[string]string{EnvPriorWeight: "lots"}, domain.Params{}, EnvPriorWeight},
		{"negative prior", map[string]string{EnvPriorWeight: "-1"}, domain.Params{}, "prior weight"},
		{"negative half-life", map[string]string{EnvHalfLife: "-1h"}, domain.Params{}, "half-life"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParamsFromConfig(config.NewFromMap(tt.env))
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want one mentioning %q", err, tt.wantErr)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Errorf("ParamsFromConfig = %+v, %v; want %+v", got, err, tt.want)
			}
		})
	}

	// An unparsable half-life is reported by the loader, like every other variable.
	cfg := config.NewFromMap(map[string]string{EnvHalfLife: "a month"})
	if _, err := ParamsFromConfig(cfg); err != nil {
		t.Fatalf("ParamsFromConfig = %v, want the default and the error in cfg.Err()", err)
	}
	if err := cfg.Err(); err == nil || !strings.Contains(err.Error(), EnvHalfLife) {
		t.Errorf("cfg.Err() = %v, want it to name %s", err, EnvHalfLife)
	}
}

func TestParamsAccessor(t *testing.T) {
	svc, _ := newService(t)
	if svc.Params() != domain.DefaultParams {
		t.Errorf("Params = %+v", svc.Params())
	}
}
