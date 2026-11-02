package postgres

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/Swiftmeds-bht/Swiftmeds/services/catalog/internal/domain"
	"github.com/Swiftmeds-bht/Swiftmeds/services/catalog/internal/ports"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const productSelect = `
	SELECT p.product_id,
	       p.brand_name,
	       p.generic_name,
	       p.molecule_id,
	       m.molecule_name,
	       p.strength,
	       p.dosage_form,
	       p.pack_size,
	       p.manufacturer,
	       p.requires_prescription,
	       p.controlled
	FROM catalog_products p
	JOIN catalog_molecules m ON m.molecule_id = p.molecule_id
`

type scanner interface {
	Scan(dest ...any) error
}

// Repository stores Catalog products in PostgreSQL.
type Repository struct {
	db *pgxpool.Pool
}

var _ ports.ProductRepository = (*Repository)(nil)

// NewRepository creates a PostgreSQL product repository.
func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

// SearchByName finds non-controlled products by brand, generic, or molecule
// name. Prefix matches are ranked ahead of fuzzy matches.
func (r *Repository) SearchByName(
	ctx context.Context,
	query string,
	limit int,
) ([]domain.Product, error) {
	if err := r.checkReady(); err != nil {
		return nil, err
	}

	query = strings.TrimSpace(query)
	if query == "" {
		return []domain.Product{}, nil
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	rows, err := r.db.Query(ctx, productSelect+`
		WHERE NOT p.controlled
		  AND (
		       lower(p.brand_name) LIKE lower($1) || '%'
		    OR lower(p.generic_name) LIKE lower($1) || '%'
		    OR lower(m.molecule_name) LIKE lower($1) || '%'
		    OR lower(p.brand_name) % lower($1)
		    OR lower(p.generic_name) % lower($1)
		    OR lower(m.molecule_name) % lower($1)
		  )
		ORDER BY
		  CASE
		    WHEN lower(p.brand_name) = lower($1)
		      OR lower(p.generic_name) = lower($1)
		      OR lower(m.molecule_name) = lower($1) THEN 0
		    WHEN lower(p.brand_name) LIKE lower($1) || '%'
		      OR lower(p.generic_name) LIKE lower($1) || '%'
		      OR lower(m.molecule_name) LIKE lower($1) || '%' THEN 1
		    ELSE 2
		  END,
		  GREATEST(
		    similarity(lower(p.brand_name), lower($1)),
		    similarity(lower(p.generic_name), lower($1)),
		    similarity(lower(m.molecule_name), lower($1))
		  ) DESC,
		  lower(p.brand_name),
		  p.product_id
		LIMIT $2
	`, query, limit)
	if err != nil {
		return nil, fmt.Errorf("search catalog products: %w", err)
	}
	defer rows.Close()

	products := make([]domain.Product, 0)
	for rows.Next() {
		product, err := scanProduct(rows)
		if err != nil {
			return nil, fmt.Errorf("scan catalog product: %w", err)
		}
		products = append(products, product)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read catalog products: %w", err)
	}
	return products, nil
}

// GetByID returns one product by ID.
func (r *Repository) GetByID(
	ctx context.Context,
	productID string,
) (domain.Product, error) {
	if err := r.checkReady(); err != nil {
		return domain.Product{}, err
	}
	productID = strings.TrimSpace(productID)
	if productID == "" {
		return domain.Product{}, domain.ErrMissingProductID
	}

	product, err := scanProduct(r.db.QueryRow(
		ctx,
		productSelect+` WHERE p.product_id = $1`,
		productID,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Product{}, fmt.Errorf("%w: %s", domain.ErrProductNotFound, productID)
	}
	if err != nil {
		return domain.Product{}, fmt.Errorf("get catalog product %q: %w", productID, err)
	}
	return product, nil
}

// GetMany returns products matching the supplied IDs, in input order.
func (r *Repository) GetMany(
	ctx context.Context,
	productIDs []string,
) ([]domain.Product, error) {
	if err := r.checkReady(); err != nil {
		return nil, err
	}
	products := make([]domain.Product, 0, len(productIDs))
	if len(productIDs) == 0 {
		return products, nil
	}

	rows, err := r.db.Query(ctx, productSelect+`
		WHERE p.product_id = ANY($1::text[])
		ORDER BY array_position($1::text[], p.product_id)
	`, productIDs)
	if err != nil {
		return nil, fmt.Errorf("get catalog products: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		product, err := scanProduct(rows)
		if err != nil {
			return nil, fmt.Errorf("scan catalog product: %w", err)
		}
		products = append(products, product)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read catalog products: %w", err)
	}
	return products, nil
}

// ListByMolecule returns products for a molecule, optionally limited to one
// normalized strength.
func (r *Repository) ListByMolecule(
	ctx context.Context,
	moleculeID string,
	strength string,
) ([]domain.Product, error) {
	if err := r.checkReady(); err != nil {
		return nil, err
	}
	moleculeID = strings.TrimSpace(moleculeID)
	if moleculeID == "" {
		return nil, domain.ErrMissingMoleculeID
	}

	rows, err := r.db.Query(ctx, productSelect+`
		WHERE p.molecule_id = $1
		  AND NOT p.controlled
		  AND ($2 = '' OR p.strength = $2)
		ORDER BY p.strength, lower(p.brand_name), p.product_id
	`, moleculeID, strength)
	if err != nil {
		return nil, fmt.Errorf("list products for molecule %q: %w", moleculeID, err)
	}
	defer rows.Close()

	products := make([]domain.Product, 0)
	for rows.Next() {
		product, err := scanProduct(rows)
		if err != nil {
			return nil, fmt.Errorf("scan catalog product: %w", err)
		}
		products = append(products, product)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read products for molecule %q: %w", moleculeID, err)
	}
	return products, nil
}

// Create saves a molecule and product in one transaction.
func (r *Repository) Create(
	ctx context.Context,
	product domain.Product,
) (domain.Product, error) {
	if err := r.checkReady(); err != nil {
		return domain.Product{}, err
	}

	productID, err := newProductID()
	if err != nil {
		return domain.Product{}, err
	}
	product.ProductID = productID

	err = pgx.BeginFunc(ctx, r.db, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO catalog_molecules (molecule_id, molecule_name)
			VALUES ($1, $2)
			ON CONFLICT (molecule_id)
			DO UPDATE SET molecule_name = EXCLUDED.molecule_name
		`, product.MoleculeID, product.MoleculeName)
		if err != nil {
			return fmt.Errorf("save molecule: %w", err)
		}

		err = tx.QueryRow(ctx, `
			INSERT INTO catalog_products (
				product_id,
				brand_name,
				generic_name,
				molecule_id,
				strength,
				dosage_form,
				pack_size,
				manufacturer,
				requires_prescription,
				controlled
			)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
			RETURNING product_id
		`,
			product.ProductID,
			product.BrandName,
			product.GenericName,
			product.MoleculeID,
			product.Strength,
			string(product.DosageForm),
			product.PackSize,
			product.Manufacturer,
			product.RequiresPrescription,
			product.Controlled,
		).Scan(&product.ProductID)
		if err != nil {
			return fmt.Errorf("insert catalog product: %w", err)
		}
		return nil
	})
	if err != nil {
		return domain.Product{}, fmt.Errorf("create catalog product: %w", err)
	}
	return product, nil
}

func (r *Repository) checkReady() error {
	if r == nil || r.db == nil {
		return errors.New("catalog PostgreSQL repository is not initialized")
	}
	return nil
}

func scanProduct(row scanner) (domain.Product, error) {
	var product domain.Product
	var dosageForm string

	err := row.Scan(
		&product.ProductID,
		&product.BrandName,
		&product.GenericName,
		&product.MoleculeID,
		&product.MoleculeName,
		&product.Strength,
		&dosageForm,
		&product.PackSize,
		&product.Manufacturer,
		&product.RequiresPrescription,
		&product.Controlled,
	)
	if err != nil {
		return domain.Product{}, err
	}
	product.DosageForm = domain.DosageForm(dosageForm)
	return product, nil
}

func newProductID() (string, error) {
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return "", fmt.Errorf("generate catalog product ID: %w", err)
	}

	id[6] = (id[6] & 0x0f) | 0x40
	id[8] = (id[8] & 0x3f) | 0x80

	encoded := make([]byte, 32)
	hex.Encode(encoded, id[:])
	return fmt.Sprintf(
		"%s-%s-%s-%s-%s",
		encoded[0:8],
		encoded[8:12],
		encoded[12:16],
		encoded[16:20],
		encoded[20:32],
	), nil
}
