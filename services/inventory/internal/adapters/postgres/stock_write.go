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

func (r *Repository) Set(
	ctx context.Context,
	stock domain.Stock,
	updatedByUserID string,
) (int32, domain.Stock, error) {
	if strings.TrimSpace(stock.PharmacyID) == "" ||
		strings.TrimSpace(stock.ProductID) == "" {
		return 0, domain.Stock{}, domain.ErrInvalidStockKey
	}
	if stock.QuantityOnHand < 0 {
		return 0, domain.Stock{}, domain.ErrNegativeStock
	}

	var previousAvailable int32
	var updated domain.Stock
	var confirmedAt *time.Time
	if !stock.ConfirmedAt.IsZero() {
		confirmedAt = &stock.ConfirmedAt
	}

	err := pgx.BeginFunc(ctx, r.db, func(tx pgx.Tx) error {
		var err error
		previousAvailable, updated, err = setOne(
			ctx,
			tx,
			stock.PharmacyID,
			stock.ProductID,
			stock.QuantityOnHand,
			confirmedAt,
			updatedByUserID,
		)
		return err
	})
	if err != nil {
		return 0, domain.Stock{}, fmt.Errorf("set inventory stock: %w", err)
	}
	return previousAvailable, updated, nil
}

func (r *Repository) BulkSet(
	ctx context.Context,
	pharmacyID string,
	updates []ports.StockUpdate,
	updatedByUserID string,
) ([]ports.StockChange, []ports.StockUpdateError, error) {
	if strings.TrimSpace(pharmacyID) == "" {
		return nil, nil, domain.ErrInvalidStockKey
	}
	if len(updates) == 0 {
		return []ports.StockChange{}, []ports.StockUpdateError{}, nil
	}

	changes := make([]ports.StockChange, 0, len(updates))
	rowErrors := make([]ports.StockUpdateError, 0)
	seen := make(map[string]bool, len(updates))

	err := pgx.BeginFunc(ctx, r.db, func(tx pgx.Tx) error {
		for _, update := range updates {
			if seen[update.ProductID] {
				rowErrors = append(rowErrors, ports.StockUpdateError{
					ProductID: update.ProductID,
					Message:   "duplicate product ID in bulk update",
				})
				continue
			}
			seen[update.ProductID] = true

			previousAvailable, stock, err := setOne(
				ctx,
				tx,
				pharmacyID,
				update.ProductID,
				update.QuantityOnHand,
				nil,
				updatedByUserID,
			)
			if err != nil {
				if isStockValidationError(err) {
					rowErrors = append(rowErrors, ports.StockUpdateError{
						ProductID: update.ProductID,
						Message:   err.Error(),
					})
					continue
				}
				return fmt.Errorf("update product %q: %w", update.ProductID, err)
			}

			changes = append(changes, ports.StockChange{
				Stock:             stock,
				PreviousAvailable: previousAvailable,
			})
		}
		return nil
	})
	if err != nil {
		return nil, nil, fmt.Errorf("bulk set inventory stock: %w", err)
	}
	return changes, rowErrors, nil
}

func setOne(
	ctx context.Context,
	tx pgx.Tx,
	pharmacyID, productID string,
	quantity int32,
	confirmedAt *time.Time,
	updatedByUserID string,
) (int32, domain.Stock, error) {
	if strings.TrimSpace(pharmacyID) == "" || strings.TrimSpace(productID) == "" {
		return 0, domain.Stock{}, domain.ErrInvalidStockKey
	}
	if quantity < 0 {
		return 0, domain.Stock{}, domain.ErrNegativeStock
	}

	_, err := tx.Exec(ctx, `
		INSERT INTO inventory_stock (
			pharmacy_id,
			product_id,
			quantity_on_hand,
			quantity_reserved,
			updated_by_user_id
		)
		VALUES ($1, $2, 0, 0, $3)
		ON CONFLICT (pharmacy_id, product_id) DO NOTHING
	`, pharmacyID, productID, updatedByUserID)
	if err != nil {
		return 0, domain.Stock{}, fmt.Errorf("prepare stock row: %w", err)
	}

	existing, err := scanStock(tx.QueryRow(ctx, `
		SELECT `+stockColumns+`
		FROM inventory_stock
		WHERE pharmacy_id = $1 AND product_id = $2
		FOR UPDATE
	`, pharmacyID, productID))
	if err != nil {
		return 0, domain.Stock{}, fmt.Errorf("lock stock row: %w", err)
	}

	previousAvailable := existing.QuantityAvailable()
	updated, err := existing.SetQuantityOnHand(quantity)
	if err != nil {
		return 0, domain.Stock{}, err
	}

	updated, err = scanStock(tx.QueryRow(ctx, `
		UPDATE inventory_stock
		SET quantity_on_hand = $3,
		    confirmed_at = COALESCE($4::timestamptz, CURRENT_TIMESTAMP),
		    updated_by_user_id = $5
		WHERE pharmacy_id = $1 AND product_id = $2
		RETURNING `+stockColumns,
		pharmacyID,
		productID,
		updated.QuantityOnHand,
		confirmedAt,
		updatedByUserID,
	))
	if err != nil {
		return 0, domain.Stock{}, fmt.Errorf("save stock row: %w", err)
	}
	return previousAvailable, updated, nil
}

func isStockValidationError(err error) bool {
	return errors.Is(err, domain.ErrInvalidStockKey) ||
		errors.Is(err, domain.ErrNegativeStock) ||
		errors.Is(err, domain.ErrOnHandBelowReservedQuantity)
}
