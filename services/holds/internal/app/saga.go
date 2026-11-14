package app

import (
	"context"
	"fmt"

	"github.com/Swiftmeds-bht/Swiftmeds/services/holds/internal/domain"
)

// Outcome says what a saga event did. The RabbitMQ consumer logs it.
type Outcome string

const (
	// OutcomeConfirmed: the hold became ACTIVE and hold.confirmed was published.
	OutcomeConfirmed Outcome = "confirmed"
	// OutcomeAlreadyConfirmed: the hold was already ACTIVE (a duplicate event, or a
	// retry after publishing failed); hold.confirmed was published again.
	OutcomeAlreadyConfirmed Outcome = "already_confirmed"
	// OutcomeReleased: the hold had already ended, so the late reservation was released.
	OutcomeReleased Outcome = "released"
	// OutcomeRejected: the hold became REJECTED.
	OutcomeRejected Outcome = "rejected"
	// OutcomeAlreadyRejected: the hold was already REJECTED (a duplicate event).
	OutcomeAlreadyRejected Outcome = "already_rejected"
	// OutcomeIgnored: the event no longer matters for the hold's current status.
	OutcomeIgnored Outcome = "ignored"
)

// HandleStockReserved handles the stock.reserved event from Inventory.
//
//	PENDING                      -> ACTIVE, publish hold.confirmed
//	ACTIVE                       -> publish hold.confirmed again (duplicate event, or the first publish failed)
//	CANCELLED, EXPIRED, REJECTED -> release the stock (the patient cancelled while Inventory was reserving)
//	COLLECTED                    -> nothing (cannot happen: collecting needs an ACTIVE hold)
//
// An error means "try again": the consumer retries the event, and every step
// above is safe to repeat.
func (s *Service) HandleStockReserved(ctx context.Context, holdID string) (Outcome, error) {
	h, err := s.GetHold(ctx, holdID)
	if err != nil {
		return "", err
	}

	switch h.Status {
	case domain.Pending:
		active, err := s.ReserveHold(ctx, holdID)
		if err != nil {
			return "", err // for example ErrConflict: it was cancelled at the same moment; the retry sees that
		}
		if err := s.events.HoldConfirmed(ctx, active); err != nil {
			return "", fmt.Errorf("publish hold.confirmed: %w", err)
		}
		return OutcomeConfirmed, nil

	case domain.Active:
		if err := s.events.HoldConfirmed(ctx, h); err != nil {
			return "", fmt.Errorf("publish hold.confirmed: %w", err)
		}
		return OutcomeAlreadyConfirmed, nil

	case domain.Cancelled, domain.Expired, domain.Rejected:
		if err := s.inventory.Release(ctx, holdID); err != nil {
			return "", fmt.Errorf("release late reservation: %w", err)
		}
		return OutcomeReleased, nil

	default:
		return OutcomeIgnored, nil
	}
}

// HandleStockUnavailable handles the stock.unavailable event from Inventory.
//
//	PENDING  -> REJECTED
//	REJECTED -> nothing (duplicate event)
//	any other status -> nothing: the hold was cancelled before Inventory answered, or
//	                    it already has its stock, and this answer no longer applies
//
// Nothing was reserved, so nothing is released.
func (s *Service) HandleStockUnavailable(ctx context.Context, holdID string) (Outcome, error) {
	h, err := s.GetHold(ctx, holdID)
	if err != nil {
		return "", err
	}

	switch h.Status {
	case domain.Pending:
		if _, err := s.RejectHold(ctx, holdID, reasonStockUnavailable); err != nil {
			return "", err
		}
		return OutcomeRejected, nil
	case domain.Rejected:
		if err := s.events.HoldRejected(ctx, h); err != nil {
			return "", fmt.Errorf("publish hold.rejected: %w", err)
		}
		return OutcomeAlreadyRejected, nil
	default:
		return OutcomeIgnored, nil
	}
}

// noInventory is used when no Inventory client is set: no stock is touched.
type noInventory struct{}

func (noInventory) Release(context.Context, string) error { return nil }
func (noInventory) Commit(context.Context, string) error  { return nil }

// noEvents is used when no publisher is set: no event is sent.
type noEvents struct{}

func (noEvents) HoldRequested(context.Context, domain.Hold) error { return nil }
func (noEvents) HoldConfirmed(context.Context, domain.Hold) error { return nil }
func (noEvents) HoldExpired(context.Context, domain.Hold) error   { return nil }
func (noEvents) HoldRejected(context.Context, domain.Hold) error  { return nil }
