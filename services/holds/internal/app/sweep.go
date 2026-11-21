package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Swiftmeds-bht/Swiftmeds/services/holds/internal/domain"
)

// DefaultPendingTimeout is how long a hold may stay PENDING before the sweep
// rejects it. The saga normally answers within a second; this only catches
// holds whose events were lost.
func DefaultPendingTimeout() time.Duration { return 10 * time.Minute }

// SweepBatchSize is the most holds one sweep looks at in each of its two steps.
const SweepBatchSize = 100

// releaseRetryWindow is how long the sweep keeps trying to release the stock of
// a timed-out hold. Inventory answers "no such reservation" with the same error
// as an outage, so a hold whose hold.requested never reached Inventory would
// otherwise be retried for ever. If that request arrives late, Inventory
// reserves and sends stock.reserved, and HandleStockReserved releases it
// because the hold is REJECTED.
const releaseRetryWindow = time.Hour

// reasonPendingTimedOut is the status reason of a hold the sweep rejected.
const reasonPendingTimedOut = "stock reservation timed out"

// SweepResult says what one sweep did. The caller logs it.
type SweepResult struct {
	// Rejected: stale PENDING holds that became REJECTED.
	Rejected int
	// Released: timed-out holds whose stock Inventory released.
	Released int
	// ReleaseFailed: releases that failed and will be tried again.
	ReleaseFailed int
	// GaveUp: holds whose release kept failing for releaseRetryWindow.
	GaveUp int
}

// SweepStaleHolds recovers holds that the reservation saga lost (SCRUM-125).
// A hold stays PENDING for ever if hold.requested was never sent (Holds
// stopped between storing the hold and publishing) or got lost, or if
// Inventory reserved the stock but its stock.reserved answer got lost. Then
// the units stay reserved and the hold counts against the patient's limit.
//
// Step 1 rejects every PENDING hold older than the timeout. The update only
// succeeds while the hold is still PENDING, so a reservation that arrives at
// the same moment wins or loses cleanly: a hold that became ACTIVE is left
// alone. Rejecting first (not releasing first) matters: a late stock.reserved
// then finds a REJECTED hold and releases the stock itself, instead of making
// an ACTIVE hold whose stock was already given back.
//
// Step 2 releases the stock of every rejected hold marked StockReleasePending.
// Release is safe to repeat, so a failure is simply tried again next sweep.
//
// Several Holds instances may sweep at once: every step is conditional or
// safe to repeat. The returned error joins every failure; the counts say what
// still happened.
func (s *Service) SweepStaleHolds(ctx context.Context) (SweepResult, error) {
	var res SweepResult
	now := s.now()
	timeout := s.cfg.PendingTimeout
	if timeout <= 0 {
		timeout = DefaultPendingTimeout()
	}

	stale, err := s.repo.ListStalePending(ctx, now.Add(-timeout), SweepBatchSize)
	if err != nil {
		return res, fmt.Errorf("list stale holds: %w", err)
	}
	var errs []error
	for _, h := range stale {
		rejected, err := h.Reject(now, reasonPendingTimedOut)
		if err != nil {
			errs = append(errs, fmt.Errorf("hold %s: %w", h.ID, err))
			continue
		}
		rejected.StockReleasePending = true
		if err := s.repo.Update(ctx, rejected, domain.Pending); err != nil {
			if !errors.Is(err, domain.ErrConflict) { // ErrConflict: it moved on meanwhile; nothing to do
				errs = append(errs, fmt.Errorf("reject hold %s: %w", h.ID, err))
			}
			continue
		}
		res.Rejected++
		// The patient is told through hold.rejected. If that fails the hold is
		// still REJECTED; the stock is released below either way.
		if err := s.events.HoldRejected(ctx, rejected); err != nil {
			errs = append(errs, fmt.Errorf("publish hold.rejected %s: %w", h.ID, err))
		}
	}

	waiting, err := s.repo.ListReleasePending(ctx, SweepBatchSize)
	if err != nil {
		return res, errors.Join(append(errs, fmt.Errorf("list holds to release: %w", err))...)
	}
	for _, h := range waiting {
		releaseErr := s.inventory.Release(ctx, h.ID)
		if releaseErr != nil && now.Sub(h.StatusChangedAt) < releaseRetryWindow {
			res.ReleaseFailed++
			errs = append(errs, fmt.Errorf("release stock of hold %s: %w", h.ID, releaseErr))
			continue
		}
		done := h
		done.StockReleasePending = false
		if err := s.repo.Update(ctx, done, h.Status); err != nil {
			errs = append(errs, fmt.Errorf("clear release mark of hold %s: %w", h.ID, err))
			continue
		}
		if releaseErr != nil {
			res.GaveUp++
			errs = append(errs, fmt.Errorf("gave up releasing stock of hold %s after %s: %w",
				h.ID, releaseRetryWindow, releaseErr))
			continue
		}
		res.Released++
	}
	return res, errors.Join(errs...)
}
