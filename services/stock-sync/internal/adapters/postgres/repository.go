package postgres

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Swiftmeds-bht/Swiftmeds/services/stock-sync/internal/domain"
	"github.com/Swiftmeds-bht/Swiftmeds/services/stock-sync/internal/ports"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const jobColumns = `
	job_id,
	pharmacy_id,
	uploaded_by_user_id,
	filename,
	status,
	rows_total,
	rows_applied,
	rows_rejected,
	row_errors,
	created_at,
	finished_at
`

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Save(ctx context.Context, job domain.SyncJob) error {
	rowErrors, err := json.Marshal(job.RowErrors)
	if err != nil {
		return fmt.Errorf("marshal sync job row errors: %w", err)
	}

	_, err = r.db.Exec(ctx, `
		INSERT INTO stock_sync_jobs (
			job_id, pharmacy_id, uploaded_by_user_id, filename, status,
			rows_total, rows_applied, rows_rejected, row_errors,
			created_at, finished_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9::jsonb, $10, $11)
		ON CONFLICT (job_id) DO UPDATE SET
			pharmacy_id = EXCLUDED.pharmacy_id,
			uploaded_by_user_id = EXCLUDED.uploaded_by_user_id,
			filename = EXCLUDED.filename,
			status = EXCLUDED.status,
			rows_total = EXCLUDED.rows_total,
			rows_applied = EXCLUDED.rows_applied,
			rows_rejected = EXCLUDED.rows_rejected,
			row_errors = EXCLUDED.row_errors,
			created_at = EXCLUDED.created_at,
			finished_at = EXCLUDED.finished_at
	`,
		job.JobID,
		job.PharmacyID,
		job.UploadedByUserID,
		job.Filename,
		int16(job.Status),
		job.RowsTotal,
		job.RowsApplied,
		job.RowsRejected,
		rowErrors,
		job.CreatedAt,
		job.FinishedAt,
	)
	if err != nil {
		return fmt.Errorf("save sync job %q: %w", job.JobID, err)
	}
	return nil
}

func (r *Repository) GetByID(
	ctx context.Context,
	jobID string,
) (domain.SyncJob, error) {
	job, err := scanJob(r.db.QueryRow(ctx,
		`SELECT `+jobColumns+` FROM stock_sync_jobs WHERE job_id = $1`,
		jobID,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.SyncJob{}, fmt.Errorf("%w: %q", domain.ErrJobNotFound, jobID)
	}
	if err != nil {
		return domain.SyncJob{}, fmt.Errorf("read sync job %q: %w", jobID, err)
	}
	return job, nil
}

func (r *Repository) ListByPharmacy(
	ctx context.Context,
	pharmacyID string,
	pageSize int32,
	pageToken string,
) ([]domain.SyncJob, string, error) {
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}

	query := `SELECT ` + jobColumns + `
		FROM stock_sync_jobs
		WHERE pharmacy_id = $1
		ORDER BY created_at DESC, job_id DESC
		LIMIT $2`
	args := []any{pharmacyID, pageSize + 1}

	if pageToken != "" {
		cursor, err := decodeCursor(pageToken)
		if err != nil {
			return nil, "", err
		}
		query = `SELECT ` + jobColumns + `
			FROM stock_sync_jobs
			WHERE pharmacy_id = $1
			  AND (created_at, job_id) < ($2, $3)
			ORDER BY created_at DESC, job_id DESC
			LIMIT $4`
		args = []any{pharmacyID, cursor.CreatedAt, cursor.JobID, pageSize + 1}
	}

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, "", fmt.Errorf("list sync jobs for pharmacy %q: %w", pharmacyID, err)
	}
	defer rows.Close()

	jobs := make([]domain.SyncJob, 0, pageSize+1)
	for rows.Next() {
		job, err := scanJob(rows)
		if err != nil {
			return nil, "", fmt.Errorf("scan sync job: %w", err)
		}
		jobs = append(jobs, job)
	}
	if err := rows.Err(); err != nil {
		return nil, "", fmt.Errorf("read sync jobs: %w", err)
	}

	var nextPageToken string
	if len(jobs) > int(pageSize) {
		last := jobs[int(pageSize)-1]
		nextPageToken = encodeCursor(last)
		jobs = jobs[:int(pageSize)]
	}
	return jobs, nextPageToken, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanJob(row rowScanner) (domain.SyncJob, error) {
	var job domain.SyncJob
	var status int16
	var rowErrorsJSON []byte

	err := row.Scan(
		&job.JobID,
		&job.PharmacyID,
		&job.UploadedByUserID,
		&job.Filename,
		&status,
		&job.RowsTotal,
		&job.RowsApplied,
		&job.RowsRejected,
		&rowErrorsJSON,
		&job.CreatedAt,
		&job.FinishedAt,
	)
	if err != nil {
		return domain.SyncJob{}, err
	}

	job.Status = domain.JobStatus(status)
	if err := json.Unmarshal(rowErrorsJSON, &job.RowErrors); err != nil {
		return domain.SyncJob{}, fmt.Errorf("decode sync job row errors: %w", err)
	}
	if job.RowErrors == nil {
		job.RowErrors = make([]domain.RowError, 0)
	}
	return job, nil
}

type cursor struct {
	CreatedAt time.Time
	JobID     string
}

func encodeCursor(job domain.SyncJob) string {
	raw := job.CreatedAt.UTC().Format(time.RFC3339Nano) + "\n" + job.JobID
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

func decodeCursor(token string) (cursor, error) {
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return cursor{}, fmt.Errorf("%w: decode token", domain.ErrInvalidPageToken)
	}

	parts := strings.SplitN(string(raw), "\n", 2)
	if len(parts) != 2 || strings.TrimSpace(parts[1]) == "" {
		return cursor{}, domain.ErrInvalidPageToken
	}

	createdAt, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return cursor{}, fmt.Errorf("%w: invalid timestamp", domain.ErrInvalidPageToken)
	}
	return cursor{CreatedAt: createdAt, JobID: parts[1]}, nil
}

var _ ports.JobRepository = (*Repository)(nil)
