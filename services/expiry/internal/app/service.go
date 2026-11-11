// Package app holds Expiry's use cases. It depends only on domain and ports.
package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/Swiftmeds-bht/Swiftmeds/services/expiry/internal/domain"
	"github.com/Swiftmeds-bht/Swiftmeds/services/expiry/internal/ports"
)

// DefaultBatchSize is how many due deadlines a sweep reads from the database at a time.
const DefaultBatchSize = 50

// DefaultHoldDuration is how long a hold lasts, as answered by GetHoldDuration.
const DefaultHoldDuration = 2 * time.Hour

// Config holds the settings the use cases need. Zero values use the defaults.
type Config struct {
	// BatchSize is how many due deadlines a sweep reads at a time.
	BatchSize int

	// HoldDuration is what GetHoldDuration answers.
	HoldDuration time.Duration
}

// Service implements the Expiry use cases.
type Service struct {
	repo  ports.DeadlineRepository
	holds ports.HoldExpirer
	cfg   Config
	now   func() time.Time
	log   *slog.Logger
}

// NewService wires the use cases. Pass time.Now in production and a fixed clock
// in tests. A nil logger uses slog's default.
func NewService(repo ports.DeadlineRepository, holds ports.HoldExpirer, cfg Config, now func() time.Time, log *slog.Logger) *Service {
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = DefaultBatchSize
	}
	if cfg.HoldDuration <= 0 {
		cfg.HoldDuration = DefaultHoldDuration
	}
	if log == nil {
		log = slog.Default()
	}
	return &Service{repo: repo, holds: holds, cfg: cfg, now: now, log: log}
}

// RecordDeadline stores the deadline of a confirmed hold (the hold.confirmed
// event). Recording the same hold twice is harmless: the first deadline stays.
func (s *Service) RecordDeadline(ctx context.Context, holdID string, expiresAt time.Time) error {
	d, err := domain.NewDeadline(holdID, expiresAt)
	if err != nil {
		return err
	}
	if err := s.repo.Save(ctx, d); err != nil {
		return fmt.Errorf("save deadline: %w", err)
	}
	return nil
}

// HoldDuration is how long a new hold should last.
func (s *Service) HoldDuration() time.Duration {
	return s.cfg.HoldDuration
}

// SweepResult says what one sweep did.
type SweepResult struct {
	// Expired lists the holds this sweep expired, oldest deadline first. A hold
	// is listed by exactly one sweep, however often the sweep runs.
	Expired []string

	// Deferred counts due holds left for the next sweep because Holds does not
	// consider them expired yet (the two clocks differ by a moment).
	Deferred int

	// Failed counts due holds that could not be settled this time.
	Failed int
}

// RunSweep expires every hold whose deadline is at or before asOf. A zero asOf
// means now. It is safe to run again and again, and to run twice at the same
// moment: a deadline is settled once, so a hold is expired and reported once.
//
// The sweep works through the due deadlines in pages and stops early, keeping
// what it has done, when ctx ends or when Holds cannot be reached. The deadlines
// it did not reach stay pending for the next sweep. The returned error is
// non-nil only when the sweep was cut short; the result always shows the work
// that was done before that.
func (s *Service) RunSweep(ctx context.Context, asOf time.Time) (SweepResult, error) {
	if asOf.IsZero() {
		asOf = s.now()
	}
	var res SweepResult
	var after *domain.Deadline
	for {
		if err := ctx.Err(); err != nil {
			return res, fmt.Errorf("sweep stopped: %w", err)
		}
		due, err := s.repo.ListDue(ctx, asOf, after, s.cfg.BatchSize)
		if err != nil {
			return res, fmt.Errorf("list due deadlines: %w", err)
		}
		for _, d := range due {
			if err := ctx.Err(); err != nil {
				return res, fmt.Errorf("sweep stopped: %w", err)
			}
			if err := s.settle(ctx, d, &res); err != nil {
				return res, err
			}
		}
		if len(due) < s.cfg.BatchSize {
			return res, nil
		}
		last := due[len(due)-1]
		after = &last
	}
}

// settle expires one due hold and settles its deadline. It returns an error
// only when the whole sweep should stop; a problem with this one hold is counted
// in res and the hold stays pending.
func (s *Service) settle(ctx context.Context, d domain.Deadline, res *SweepResult) error {
	var outcome domain.Outcome
	status, err := s.holds.ExpireHold(ctx, d.HoldID)
	switch {
	case errors.Is(err, domain.ErrHoldsUnavailable):
		res.Failed++
		return fmt.Errorf("expire hold %s: %w", d.HoldID, err)
	case errors.Is(err, domain.ErrNotYetExpired):
		res.Deferred++
		return nil
	case errors.Is(err, domain.ErrHoldNotFound):
		outcome = domain.OutcomeNotFound
		s.log.Warn("deadline for a hold that Holds does not know", "hold_id", d.HoldID)
	case err != nil:
		res.Failed++
		s.log.Warn("expiring a hold failed", "hold_id", d.HoldID, "error", err.Error())
		return nil
	case status == domain.HoldExpired:
		outcome = domain.OutcomeExpired
	case status.Final():
		outcome = domain.OutcomeAlreadyFinal
	default:
		res.Deferred++
		s.log.Warn("hold is not final after ExpireHold", "hold_id", d.HoldID, "status", string(status))
		return nil
	}

	won, err := s.repo.Complete(ctx, d.HoldID, outcome, s.now())
	if err != nil {
		res.Failed++
		return fmt.Errorf("settle deadline of hold %s: %w", d.HoldID, err)
	}
	// Two sweeps at the same moment both reach this point. Only the one that
	// settled the deadline reports the hold.
	if won && outcome == domain.OutcomeExpired {
		res.Expired = append(res.Expired, d.HoldID)
	}
	return nil
}
