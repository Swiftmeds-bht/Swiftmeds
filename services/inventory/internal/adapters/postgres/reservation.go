package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Swiftmeds-bht/Swiftmeds/services/inventory/internal/domain"
	"github.com/Swiftmeds-bht/Swiftmeds/services/inventory/internal/ports"
	"github.com/jackc/pgx/v5"
)

const (
	reservationUnavailable = "UNAVAILABLE"
	reservationReserved    = "RESERVED"
	reservationCommitted   = "COMMITTED"
	reservationReleased    = "RELEASED"
)

var (
	errEmptyHoldID            = errors.New("hold ID is empty")
	errReservationNotFound    = errors.New("reservation not found")
	errReservationConflict    = errors.New("reservation is in a conflicting state")
	errReservationKeyMismatch = errors.New("hold ID was already used for a different reservation")
)

type reservationRecord struct {
	pharmacyID string
	productID  string
	quantity   int32
	status     string
}

// Reserve atomically reserves stock and uses holdID as its idempotency key.
func (r *Repository) Reserve(
	ctx context.Context,
	holdID string,
	key ports.StockKey,
	quantity int32,
) (domain.Stock, bool, error) {
	if strings.TrimSpace(holdID) == "" {
		return domain.Stock{}, false, errEmptyHoldID
	}
	if strings.TrimSpace(key.PharmacyID) == "" || strings.TrimSpace(key.ProductID) == "" {
		return domain.Stock{}, false, domain.ErrInvalidStockKey
	}
	if quantity <= 0 {
		return domain.Stock{}, false, domain.ErrInvalidReservationQuantity
	}

	var result domain.Stock
	var reserved bool

	err := pgx.BeginFunc(ctx, r.db, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			INSERT INTO inventory_reservations (
				hold_id, pharmacy_id, product_id, quantity, status
			)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (hold_id) DO NOTHING
		`, holdID, key.PharmacyID, key.ProductID, quantity, reservationUnavailable)
		if err != nil {
			return fmt.Errorf("record reservation request: %w", err)
		}

		record, err := getReservationForUpdate(ctx, tx, holdID)
		if err != nil {
			return err
		}
		if record.pharmacyID != key.PharmacyID ||
			record.productID != key.ProductID ||
			record.quantity != quantity {
			return fmt.Errorf("%w: %q", errReservationKeyMismatch, holdID)
		}

		stock, found, err := getStockForUpdate(ctx, tx, key)
		if err != nil {
			return err
		}
		if !found {
			stock, err = emptyStock(key)
			if err != nil {
				return err
			}
		}

		if tag.RowsAffected() == 0 {
			switch record.status {
			case reservationReserved, reservationCommitted:
				reserved = true
			case reservationUnavailable, reservationReleased:
				reserved = false
			default:
				return fmt.Errorf("%w: unknown status %q", errReservationConflict, record.status)
			}
			result = stock
			return nil
		}

		if !found {
			result = stock
			return nil
		}

		updated, err := stock.Reserve(quantity)
		if errors.Is(err, domain.ErrInsufficientAvailableStock) {
			result = stock
			return nil
		}
		if err != nil {
			return err
		}

		updated, err = scanStock(tx.QueryRow(ctx, `
			UPDATE inventory_stock
			SET quantity_reserved = $3
			WHERE pharmacy_id = $1 AND product_id = $2
			RETURNING `+stockColumns,
			key.PharmacyID,
			key.ProductID,
			updated.QuantityReserved,
		))
		if err != nil {
			return fmt.Errorf("save stock reservation: %w", err)
		}

		if err := updateReservationStatus(ctx, tx, holdID, reservationReserved); err != nil {
			return err
		}

		result = updated
		reserved = true
		return nil
	})
	if err != nil {
		return domain.Stock{}, false, fmt.Errorf("reserve inventory: %w", err)
	}
	return result, reserved, nil
}

// CommitReservation converts a reserved quantity into a completed deduction.
func (r *Repository) CommitReservation(
	ctx context.Context,
	holdID string,
) (domain.Stock, error) {
	if strings.TrimSpace(holdID) == "" {
		return domain.Stock{}, errEmptyHoldID
	}

	var result domain.Stock
	err := pgx.BeginFunc(ctx, r.db, func(tx pgx.Tx) error {
		record, err := getReservationForUpdate(ctx, tx, holdID)
		if err != nil {
			return err
		}

		key := ports.StockKey{PharmacyID: record.pharmacyID, ProductID: record.productID}
		stock, found, err := getStockForUpdate(ctx, tx, key)
		if err != nil {
			return err
		}
		if !found {
			return fmt.Errorf("stock row missing for hold %q", holdID)
		}

		switch record.status {
		case reservationCommitted:
			result = stock
			return nil
		case reservationReserved:
			// Continue below.
		default:
			return fmt.Errorf("%w: cannot commit hold %q with status %s",
				errReservationConflict, holdID, record.status)
		}

		updated, err := stock.CommitReservation(record.quantity)
		if err != nil {
			return fmt.Errorf("commit stock for hold %q: %w", holdID, err)
		}

		updated, err = saveReservationStock(ctx, tx, updated)
		if err != nil {
			return err
		}
		if err := updateReservationStatus(ctx, tx, holdID, reservationCommitted); err != nil {
			return err
		}

		result = updated
		return nil
	})
	if err != nil {
		return domain.Stock{}, fmt.Errorf("commit reservation: %w", err)
	}
	return result, nil
}

// ReleaseReservation returns a reserved quantity to available stock.
func (r *Repository) ReleaseReservation(
	ctx context.Context,
	holdID string,
) (domain.Stock, error) {
	if strings.TrimSpace(holdID) == "" {
		return domain.Stock{}, errEmptyHoldID
	}

	var result domain.Stock
	err := pgx.BeginFunc(ctx, r.db, func(tx pgx.Tx) error {
		record, err := getReservationForUpdate(ctx, tx, holdID)
		if err != nil {
			return err
		}

		key := ports.StockKey{PharmacyID: record.pharmacyID, ProductID: record.productID}
		stock, found, err := getStockForUpdate(ctx, tx, key)
		if err != nil {
			return err
		}
		if !found {
			stock, err = emptyStock(key)
			if err != nil {
				return err
			}
		}

		switch record.status {
		case reservationReleased, reservationUnavailable:
			result = stock
			return nil
		case reservationReserved:
			// Continue below.
		default:
			return fmt.Errorf("%w: cannot release hold %q with status %s",
				errReservationConflict, holdID, record.status)
		}

		updated, err := stock.ReleaseReservation(record.quantity)
		if err != nil {
			return fmt.Errorf("release stock for hold %q: %w", holdID, err)
		}

		updated, err = saveReservationStock(ctx, tx, updated)
		if err != nil {
			return err
		}
		if err := updateReservationStatus(ctx, tx, holdID, reservationReleased); err != nil {
			return err
		}

		result = updated
		return nil
	})
	if err != nil {
		return domain.Stock{}, fmt.Errorf("release reservation: %w", err)
	}
	return result, nil
}

func getReservationForUpdate(
	ctx context.Context,
	tx pgx.Tx,
	holdID string,
) (reservationRecord, error) {
	var record reservationRecord
	err := tx.QueryRow(ctx, `
		SELECT pharmacy_id, product_id, quantity, status
		FROM inventory_reservations
		WHERE hold_id = $1
		FOR UPDATE
	`, holdID).Scan(
		&record.pharmacyID,
		&record.productID,
		&record.quantity,
		&record.status,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return reservationRecord{}, fmt.Errorf("%w: %q", errReservationNotFound, holdID)
	}
	if err != nil {
		return reservationRecord{}, fmt.Errorf("read reservation %q: %w", holdID, err)
	}
	return record, nil
}

func getStockForUpdate(
	ctx context.Context,
	tx pgx.Tx,
	key ports.StockKey,
) (domain.Stock, bool, error) {
	stock, err := scanStock(tx.QueryRow(ctx, `
		SELECT `+stockColumns+`
		FROM inventory_stock
		WHERE pharmacy_id = $1 AND product_id = $2
		FOR UPDATE
	`, key.PharmacyID, key.ProductID))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Stock{}, false, nil
	}
	if err != nil {
		return domain.Stock{}, false, fmt.Errorf("lock inventory stock: %w", err)
	}
	return stock, true, nil
}

func emptyStock(key ports.StockKey) (domain.Stock, error) {
	return domain.NewStock(key.PharmacyID, key.ProductID, 0, 0, time.Time{})
}

func saveReservationStock(
	ctx context.Context,
	tx pgx.Tx,
	stock domain.Stock,
) (domain.Stock, error) {
	updated, err := scanStock(tx.QueryRow(ctx, `
		UPDATE inventory_stock
		SET quantity_on_hand = $3,
		    quantity_reserved = $4
		WHERE pharmacy_id = $1 AND product_id = $2
		RETURNING `+stockColumns,
		stock.PharmacyID,
		stock.ProductID,
		stock.QuantityOnHand,
		stock.QuantityReserved,
	))
	if err != nil {
		return domain.Stock{}, fmt.Errorf("save reservation stock: %w", err)
	}
	return updated, nil
}

func updateReservationStatus(
	ctx context.Context,
	tx pgx.Tx,
	holdID, status string,
) error {
	_, err := tx.Exec(ctx, `
		UPDATE inventory_reservations
		SET status = $2, updated_at = CURRENT_TIMESTAMP
		WHERE hold_id = $1
	`, holdID, status)
	if err != nil {
		return fmt.Errorf("update reservation %q status: %w", holdID, err)
	}
	return nil
}

var _ ports.StockRepository = (*Repository)(nil)
