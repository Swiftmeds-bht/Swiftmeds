package postgres

import (
	"context"
	"fmt"

	"github.com/Swiftmeds-bht/Swiftmeds/services/analytics/internal/domain"
	"github.com/Swiftmeds-bht/Swiftmeds/services/analytics/internal/ports"
	"github.com/jackc/pgx/v5/pgxpool"
)

var _ ports.Repository = (*Repository)(nil)

type Repository struct{ db *pgxpool.Pool }

func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

func (r *Repository) StoreSearchRecorded(ctx context.Context, e domain.SearchRecorded) error {
	_, err := r.db.Exec(ctx, `INSERT INTO analytics_events
		(event_id,event_type,search_id,product_id,product_name,dzongkhag,occurred_at,has_stock)
		VALUES ($1,'SEARCH_RECORDED',$2,$3,$4,$5,$6,$7) ON CONFLICT (event_id) DO NOTHING`,
		e.EventID, e.SearchID, e.ProductID, e.ProductName, e.Dzongkhag, e.OccurredAt, e.HasStock)
	if err != nil {
		return fmt.Errorf("store search recorded: %w", err)
	}
	return nil
}

func (r *Repository) StoreStockUnavailable(ctx context.Context, e domain.StockUnavailable) error {
	_, err := r.db.Exec(ctx, `INSERT INTO analytics_events
		(event_id,event_type,pharmacy_id,product_id,product_name,dzongkhag,occurred_at,has_stock)
		VALUES ($1,'STOCK_UNAVAILABLE',$2,$3,$4,$5,$6,FALSE) ON CONFLICT (event_id) DO NOTHING`,
		e.EventID, e.PharmacyID, e.ProductID, e.ProductName, e.Dzongkhag, e.OccurredAt)
	if err != nil {
		return fmt.Errorf("store stock unavailable: %w", err)
	}
	return nil
}

func (r *Repository) UnmetDemand(ctx context.Context, q domain.DemandQuery) ([]domain.DemandItem, error) {
	const query = `SELECT product_id, max(product_name),
		count(*) FILTER (WHERE event_type = 'SEARCH_RECORDED'),
		count(*) FILTER (WHERE event_type = 'STOCK_UNAVAILABLE'
			OR (event_type = 'SEARCH_RECORDED' AND NOT has_stock))
		FROM analytics_events
		WHERE occurred_at >= $1 AND occurred_at < $2
		  AND ($3 = '' OR dzongkhag = $3)
		GROUP BY product_id
		ORDER BY 4 DESC, 3 DESC, 1
		LIMIT $4`
	rows, err := r.db.Query(ctx, query, q.From, q.To, q.Dzongkhag, q.Limit)
	if err != nil {
		return nil, fmt.Errorf("query unmet demand: %w", err)
	}
	defer rows.Close()
	var out []domain.DemandItem
	for rows.Next() {
		var item domain.DemandItem
		if err := rows.Scan(&item.ProductID, &item.ProductName, &item.SearchCount, &item.UnmetCount); err != nil {
			return nil, fmt.Errorf("scan unmet demand: %w", err)
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read unmet demand: %w", err)
	}
	return out, nil
}
