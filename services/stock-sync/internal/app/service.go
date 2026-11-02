// Package app contains the Stock Sync use cases.
package app

import (
	"context"
	"crypto/rand"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Swiftmeds-bht/Swiftmeds/services/stock-sync/internal/domain"
	"github.com/Swiftmeds-bht/Swiftmeds/services/stock-sync/internal/ports"
)

const (
	defaultPageSize int32 = 20
	maxPageSize     int32 = 100
)

type UploadCSVRequest struct {
	PharmacyID       string
	UploadedByUserID string
	Filename         string
	Content          []byte
}

type ListJobsRequest struct {
	PharmacyID string
	PageSize   int32
	PageToken  string
}

type Service struct {
	catalog   ports.Catalog
	inventory ports.Inventory
	pricing   ports.Pricing
	jobs      ports.JobRepository
	now       func() time.Time
}

func NewService(
	catalog ports.Catalog,
	inventory ports.Inventory,
	pricing ports.Pricing,
	jobs ports.JobRepository,
	now func() time.Time,
) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{
		catalog:   catalog,
		inventory: inventory,
		pricing:   pricing,
		jobs:      jobs,
		now:       now,
	}
}

func (s *Service) UploadStockCSV(
	ctx context.Context,
	request UploadCSVRequest,
) (domain.SyncJob, error) {
	pharmacyID := strings.TrimSpace(request.PharmacyID)
	if pharmacyID == "" {
		return domain.SyncJob{}, domain.ErrMissingPharmacyID
	}
	uploadedBy := strings.TrimSpace(request.UploadedByUserID)
	if uploadedBy == "" {
		return domain.SyncJob{}, domain.ErrMissingUploaderID
	}

	jobID, err := newJobID()
	if err != nil {
		return domain.SyncJob{}, err
	}

	job := domain.SyncJob{
		JobID:            jobID,
		PharmacyID:       pharmacyID,
		UploadedByUserID: uploadedBy,
		Filename:         strings.TrimSpace(request.Filename),
		Status:           domain.JobStatusProcessing,
		CreatedAt:        s.now().UTC(),
		RowErrors:        make([]domain.RowError, 0),
	}

	parsed, err := domain.ParseCSV(request.Content)
	if err != nil {
		job.RowErrors = append(job.RowErrors, domain.RowError{
			RowNumber: 1,
			Message:   err.Error(),
		})
		return s.finishAndSave(ctx, job)
	}

	job.RowsTotal = parsed.RowsTotal
	job.RowErrors = append(job.RowErrors, parsed.Errors...)
	if len(parsed.Rows) == 0 {
		return s.finishAndSave(ctx, job)
	}

	productIDs := make([]string, 0, len(parsed.Rows))
	for _, row := range parsed.Rows {
		productIDs = append(productIDs, row.ProductID)
	}

	existingIDs, err := s.catalog.GetExistingProductIDs(ctx, productIDs)
	if err != nil {
		addErrorsForRows(&job, parsed.Rows, "catalog lookup failed: "+err.Error())
		return s.finishAndSave(ctx, job)
	}

	existing := make(map[string]struct{}, len(existingIDs))
	for _, productID := range existingIDs {
		existing[strings.ToLower(strings.TrimSpace(productID))] = struct{}{}
	}

	knownRows := make([]domain.CSVRow, 0, len(parsed.Rows))
	for _, row := range parsed.Rows {
		if _, ok := existing[strings.ToLower(row.ProductID)]; !ok {
			job.RowErrors = append(job.RowErrors, domain.RowError{
				RowNumber: row.RowNumber,
				ProductID: row.ProductID,
				Message:   "unknown product_id",
			})
			continue
		}
		knownRows = append(knownRows, row)
	}
	if len(knownRows) == 0 {
		return s.finishAndSave(ctx, job)
	}

	updates := make([]domain.StockUpdate, 0, len(knownRows))
	for _, row := range knownRows {
		updates = append(updates, domain.StockUpdate{
			ProductID:      row.ProductID,
			QuantityOnHand: row.Quantity,
		})
	}

	bulkResult, err := s.inventory.BulkSetStock(ctx, pharmacyID, updates, uploadedBy)
	if err != nil {
		addErrorsForRows(&job, knownRows, "inventory bulk update failed: "+err.Error())
		return s.finishAndSave(ctx, job)
	}

	inventoryErrors := make(map[string]string, len(bulkResult.Errors))
	for _, rowError := range bulkResult.Errors {
		inventoryErrors[strings.ToLower(strings.TrimSpace(rowError.ProductID))] = rowError.Message
	}

	stockApplied := make([]domain.CSVRow, 0, len(knownRows))
	for _, row := range knownRows {
		if message, failed := inventoryErrors[strings.ToLower(row.ProductID)]; failed {
			job.RowErrors = append(job.RowErrors, domain.RowError{
				RowNumber: row.RowNumber,
				ProductID: row.ProductID,
				Message:   message,
			})
			continue
		}
		stockApplied = append(stockApplied, row)
	}

	priceUpdates := make([]domain.PriceUpdate, 0)
	priceRows := make([]domain.CSVRow, 0)
	for _, row := range stockApplied {
		if row.PriceChhertum == nil {
			continue
		}
		priceUpdates = append(priceUpdates, domain.PriceUpdate{
			ProductID:     row.ProductID,
			PriceChhertum: *row.PriceChhertum,
		})
		priceRows = append(priceRows, row)
	}

	priceFailures := make(map[string]struct{})
	if len(priceUpdates) > 0 {
		if err := s.pricing.SetPrices(ctx, pharmacyID, priceUpdates); err != nil {
			for _, row := range priceRows {
				key := strings.ToLower(row.ProductID)
				priceFailures[key] = struct{}{}
				job.RowErrors = append(job.RowErrors, domain.RowError{
					RowNumber: row.RowNumber,
					ProductID: row.ProductID,
					Message:   "price update failed after stock update: " + err.Error(),
				})
			}
		}
	}

	for _, row := range stockApplied {
		if _, failed := priceFailures[strings.ToLower(row.ProductID)]; !failed {
			job.RowsApplied++
		}
	}

	return s.finishAndSave(ctx, job)
}

func (s *Service) GetSyncJob(
	ctx context.Context,
	jobID string,
) (domain.SyncJob, error) {
	jobID = strings.TrimSpace(jobID)
	if jobID == "" {
		return domain.SyncJob{}, domain.ErrMissingJobID
	}
	return s.jobs.GetByID(ctx, jobID)
}

func (s *Service) ListSyncJobs(
	ctx context.Context,
	request ListJobsRequest,
) ([]domain.SyncJob, string, error) {
	pharmacyID := strings.TrimSpace(request.PharmacyID)
	if pharmacyID == "" {
		return nil, "", domain.ErrMissingPharmacyID
	}

	pageSize := request.PageSize
	if pageSize <= 0 {
		pageSize = defaultPageSize
	}
	if pageSize > maxPageSize {
		pageSize = maxPageSize
	}

	return s.jobs.ListByPharmacy(
		ctx,
		pharmacyID,
		pageSize,
		strings.TrimSpace(request.PageToken),
	)
}

func (s *Service) finishAndSave(
	ctx context.Context,
	job domain.SyncJob,
) (domain.SyncJob, error) {
	sort.SliceStable(job.RowErrors, func(i, j int) bool {
		return job.RowErrors[i].RowNumber < job.RowErrors[j].RowNumber
	})

	job.RowsRejected = int32(len(job.RowErrors))
	if job.RowsRejected > job.RowsTotal {
		job.RowsRejected = job.RowsTotal
	}

	switch {
	case job.RowsTotal == 0 || job.RowsApplied == 0:
		job.Status = domain.JobStatusFailed
	case job.RowsRejected > 0:
		job.Status = domain.JobStatusPartial
	default:
		job.Status = domain.JobStatusCompleted
	}
	job.FinishedAt = s.now().UTC()

	if err := s.jobs.Save(ctx, job); err != nil {
		return domain.SyncJob{}, fmt.Errorf("save sync job: %w", err)
	}
	return job, nil
}

func addErrorsForRows(job *domain.SyncJob, rows []domain.CSVRow, message string) {
	for _, row := range rows {
		job.RowErrors = append(job.RowErrors, domain.RowError{
			RowNumber: row.RowNumber,
			ProductID: row.ProductID,
			Message:   message,
		})
	}
}

func newJobID() (string, error) {
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return "", fmt.Errorf("generate sync job ID: %w", err)
	}
	return fmt.Sprintf(
		"%x-%x-%x-%x-%x",
		id[0:4], id[4:6], id[6:8], id[8:10], id[10:16],
	), nil
}
