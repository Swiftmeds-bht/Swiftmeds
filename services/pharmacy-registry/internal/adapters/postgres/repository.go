package postgres

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Swiftmeds-bht/Swiftmeds/services/pharmacy-registry/internal/domain"
	"github.com/Swiftmeds-bht/Swiftmeds/services/pharmacy-registry/internal/ports"
)

var _ ports.PharmacyRepository = (*Repository)(nil)

type Repository struct{ db *pgxpool.Pool }

func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

func (r *Repository) Create(ctx context.Context, p domain.Pharmacy) error {
	hours, err := json.Marshal(p.Hours)
	if err != nil {
		return fmt.Errorf("encode hours: %w", err)
	}
	_, err = r.db.Exec(ctx, `INSERT INTO pharmacies
		(pharmacy_id, owner_user_id, name, license_number, phone, address, dzongkhag, latitude, longitude, hours, status, status_reason, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`,
		p.ID, p.OwnerID, p.Name, p.LicenseNumber, p.Phone, p.Address, p.Dzongkhag, p.Location.Latitude,
		p.Location.Longitude, hours, string(p.Status), p.StatusReason, p.CreatedAt)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation && pgErr.ConstraintName == licenseConstraint {
		return domain.ErrLicenseTaken
	}
	if err != nil {
		return fmt.Errorf("insert pharmacy: %w", err)
	}
	return nil
}

const (
	uniqueViolation   = "23505"
	licenseConstraint = "pharmacies_license_number_key"
)

func (r *Repository) FindByOwner(ctx context.Context, ownerID string) (domain.Pharmacy, error) {
	return r.scanOne(r.db.QueryRow(ctx, pharmacySelect+` WHERE owner_user_id = $1 ORDER BY created_at, pharmacy_id LIMIT 1`, ownerID))
}

func (r *Repository) Get(ctx context.Context, id string) (domain.Pharmacy, error) {
	return r.scanOne(r.db.QueryRow(ctx, pharmacySelect+` WHERE pharmacy_id = $1`, id))
}

func (r *Repository) GetMany(ctx context.Context, ids []string) ([]domain.Pharmacy, error) {
	if len(ids) == 0 {
		return []domain.Pharmacy{}, nil
	}
	rows, err := r.db.Query(ctx, pharmacySelect+` WHERE pharmacy_id = ANY($1) ORDER BY pharmacy_id`, ids)
	if err != nil {
		return nil, fmt.Errorf("get pharmacies: %w", err)
	}
	defer rows.Close()
	var out []domain.Pharmacy
	for rows.Next() {
		p, err := scanPharmacy(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("get pharmacies: %w", err)
	}
	return out, nil
}

func (r *Repository) ListPage(ctx context.Context, status domain.Status, pageSize int, pageToken string) ([]domain.Pharmacy, string, error) {
	var cursorAt *time.Time
	cursorID := ""
	if pageToken != "" {
		b, err := base64.RawURLEncoding.DecodeString(pageToken)
		if err != nil {
			return nil, "", domain.ErrInvalidPageToken
		}
		parts := strings.SplitN(string(b), "|", 2)
		if len(parts) != 2 || parts[1] == "" {
			return nil, "", domain.ErrInvalidPageToken
		}
		at, err := time.Parse(time.RFC3339Nano, parts[0])
		if err != nil {
			return nil, "", domain.ErrInvalidPageToken
		}
		cursorAt, cursorID = &at, parts[1]
	}
	conditions := make([]string, 0, 2)
	args := make([]any, 0, 4)
	if status != "" {
		args = append(args, string(status))
		conditions = append(conditions, fmt.Sprintf("status = $%d", len(args)))
	}
	if cursorAt != nil {
		args = append(args, *cursorAt, cursorID)
		conditions = append(conditions, fmt.Sprintf("(created_at, pharmacy_id) > ($%d, $%d)", len(args)-1, len(args)))
	}
	args = append(args, pageSize+1)
	query := pharmacySelect
	if len(conditions) > 0 {
		query += ` WHERE ` + strings.Join(conditions, ` AND `)
	}
	query += fmt.Sprintf(` ORDER BY created_at, pharmacy_id LIMIT $%d`, len(args))
	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, "", fmt.Errorf("list pharmacies page: %w", err)
	}
	defer rows.Close()
	var out []domain.Pharmacy
	for rows.Next() {
		p, err := scanPharmacy(rows)
		if err != nil {
			return nil, "", err
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, "", fmt.Errorf("list pharmacies page: %w", err)
	}
	if len(out) <= pageSize {
		return out, "", nil
	}
	out = out[:pageSize]
	last := out[len(out)-1]
	token := base64.RawURLEncoding.EncodeToString([]byte(last.CreatedAt.UTC().Format(time.RFC3339Nano) + "|" + last.ID))
	return out, token, nil
}

func (r *Repository) List(ctx context.Context, verifiedOnly bool) ([]domain.Pharmacy, error) {
	query := pharmacySelect
	if verifiedOnly {
		query += ` WHERE status = 'VERIFIED'`
	}
	query += ` ORDER BY pharmacy_id`
	rows, err := r.db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list pharmacies: %w", err)
	}
	defer rows.Close()
	var out []domain.Pharmacy
	for rows.Next() {
		p, err := scanPharmacy(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list pharmacies: %w", err)
	}
	return out, nil
}

func (r *Repository) SetStatus(ctx context.Context, id string, status domain.Status, reason string, now time.Time) (domain.Pharmacy, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return domain.Pharmacy{}, fmt.Errorf("begin status update: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var p domain.Pharmacy
	if err := scanRow(tx.QueryRow(ctx, pharmacySelect+` WHERE pharmacy_id = $1 FOR UPDATE`, id), &p); err != nil {
		return domain.Pharmacy{}, err
	}
	next, err := p.ApplyStatus(status, reason, now)
	if err != nil {
		return domain.Pharmacy{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE pharmacies SET status=$1,status_reason=$2,verified_at=$3 WHERE pharmacy_id=$4`,
		string(next.Status), next.StatusReason, next.VerifiedAt, id); err != nil {
		return domain.Pharmacy{}, fmt.Errorf("update pharmacy status: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Pharmacy{}, fmt.Errorf("commit status update: %w", err)
	}
	return next, nil
}

func (r *Repository) Update(ctx context.Context, id, ownerID string, changes domain.Pharmacy) (domain.Pharmacy, error) {
	hours, err := json.Marshal(changes.Hours)
	if err != nil {
		return domain.Pharmacy{}, fmt.Errorf("encode hours: %w", err)
	}
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return domain.Pharmacy{}, fmt.Errorf("begin pharmacy update: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var p domain.Pharmacy
	if err := scanRow(tx.QueryRow(ctx, pharmacySelect+` WHERE pharmacy_id=$1 FOR UPDATE`, id), &p); err != nil {
		return domain.Pharmacy{}, err
	}
	if p.OwnerID != ownerID {
		return domain.Pharmacy{}, domain.ErrOwnerMismatch
	}
	p.Name, p.Phone, p.Address, p.Dzongkhag = changes.Name, changes.Phone, changes.Address, changes.Dzongkhag
	p.Location, p.Hours = changes.Location, append([]domain.OpeningHours(nil), changes.Hours...)
	if p.Status == domain.StatusRejected {
		p.Status, p.StatusReason = domain.StatusPending, ""
	}
	_, err = tx.Exec(ctx, `UPDATE pharmacies SET name=$1,phone=$2,address=$3,dzongkhag=$4,latitude=$5,longitude=$6,hours=$7,status=$8,status_reason=$9 WHERE pharmacy_id=$10`, p.Name, p.Phone, p.Address, p.Dzongkhag, p.Location.Latitude, p.Location.Longitude, hours, string(p.Status), p.StatusReason, id)
	if err != nil {
		return domain.Pharmacy{}, fmt.Errorf("update pharmacy: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Pharmacy{}, fmt.Errorf("commit pharmacy update: %w", err)
	}
	return p, nil
}

const pharmacySelect = `SELECT pharmacy_id, owner_user_id, name, license_number, phone, address, dzongkhag, latitude, longitude, hours, status, status_reason, created_at, verified_at FROM pharmacies`

type rowScanner interface{ Scan(...any) error }

func (r *Repository) scanOne(row pgx.Row) (domain.Pharmacy, error) {
	var p domain.Pharmacy
	if err := scanRow(row, &p); err != nil {
		return domain.Pharmacy{}, err
	}
	return p, nil
}

func scanPharmacy(row rowScanner) (domain.Pharmacy, error) {
	var p domain.Pharmacy
	if err := scanRow(row, &p); err != nil {
		return domain.Pharmacy{}, err
	}
	return p, nil
}

func scanRow(row rowScanner, p *domain.Pharmacy) error {
	var hours []byte
	var status string
	if err := row.Scan(&p.ID, &p.OwnerID, &p.Name, &p.LicenseNumber, &p.Phone, &p.Address, &p.Dzongkhag, &p.Location.Latitude,
		&p.Location.Longitude, &hours, &status, &p.StatusReason, &p.CreatedAt, &p.VerifiedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNotFound
		}
		return fmt.Errorf("scan pharmacy: %w", err)
	}
	if err := json.Unmarshal(hours, &p.Hours); err != nil {
		return fmt.Errorf("decode hours: %w", err)
	}
	p.Status = domain.Status(status)
	return nil
}
