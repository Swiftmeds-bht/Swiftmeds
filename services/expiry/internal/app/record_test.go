package app

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/Swiftmeds-bht/Swiftmeds/services/expiry/internal/adapters/memory"
	"github.com/Swiftmeds-bht/Swiftmeds/services/expiry/internal/domain"
)

func TestRecordDeadlineKeepsTheFirstDeadlineOfAHold(t *testing.T) {
	e := newEnv(t, Config{})
	e.holds.add("h-1", domain.HoldActive, t0)
	ctx := context.Background()

	if err := e.svc.RecordDeadline(ctx, "h-1", t0); err != nil {
		t.Fatal(err)
	}
	// The same event is delivered again, now with a different time.
	if err := e.svc.RecordDeadline(ctx, "h-1", t0.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}

	res := e.sweep(t, t0)
	if want := []string{"h-1"}; !slices.Equal(res.Expired, want) {
		t.Errorf("expired = %v, want %v (the first deadline stands)", res.Expired, want)
	}
}

func TestRecordDeadlineRejectsInvalidInput(t *testing.T) {
	e := newEnv(t, Config{})

	if err := e.svc.RecordDeadline(context.Background(), "", t0); !errors.Is(err, domain.ErrMissingHoldID) {
		t.Errorf("no hold id: err = %v, want ErrMissingHoldID", err)
	}
	if err := e.svc.RecordDeadline(context.Background(), "h-1", time.Time{}); !errors.Is(err, domain.ErrMissingExpiry) {
		t.Errorf("no expiry time: err = %v, want ErrMissingExpiry", err)
	}
}

func TestRecordDeadlineReportsARepositoryError(t *testing.T) {
	e := newEnv(t, Config{})
	e.svc.repo = failingRepo{DeadlineRepository: e.repo, saveErr: errBoom}

	if err := e.svc.RecordDeadline(context.Background(), "h-1", t0); !errors.Is(err, errBoom) {
		t.Fatalf("err = %v, want errBoom", err)
	}
}

func TestHoldDurationDefaultsAndCanBeSet(t *testing.T) {
	cases := []struct {
		name string
		cfg  Config
		want time.Duration
	}{
		{"default", Config{}, DefaultHoldDuration},
		{"negative falls back to the default", Config{HoldDuration: -time.Minute}, DefaultHoldDuration},
		{"configured", Config{HoldDuration: 90 * time.Minute}, 90 * time.Minute},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			svc := NewService(memory.NewRepository(), &stubHolds{}, c.cfg, time.Now, nil)

			if got := svc.HoldDuration(); got != c.want {
				t.Errorf("HoldDuration() = %v, want %v", got, c.want)
			}
		})
	}
}

func TestNewServiceDefaultsTheBatchSize(t *testing.T) {
	svc := NewService(memory.NewRepository(), &stubHolds{}, Config{}, time.Now, nil)

	if svc.cfg.BatchSize != DefaultBatchSize {
		t.Errorf("batch size = %d, want %d", svc.cfg.BatchSize, DefaultBatchSize)
	}
}
