package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Swiftmeds-bht/Swiftmeds/services/stock-sync/internal/domain"
)

type catalogFake struct {
	existing map[string]bool
	err      error
	calls    int
	askedIDs []string
}

func (f *catalogFake) GetExistingProductIDs(
	_ context.Context,
	productIDs []string,
) ([]string, error) {
	f.calls++
	f.askedIDs = append([]string(nil), productIDs...)
	if f.err != nil {
		return nil, f.err
	}

	if f.existing == nil {
		return append([]string(nil), productIDs...), nil
	}

	existing := make([]string, 0, len(productIDs))
	for _, id := range productIDs {
		if f.existing[strings.ToLower(id)] {
			existing = append(existing, id)
		}
	}
	return existing, nil
}

type inventoryFake struct {
	result  domain.BulkStockResult
	err     error
	calls   int
	updates []domain.StockUpdate
}

func (f *inventoryFake) BulkSetStock(
	_ context.Context,
	_ string,
	updates []domain.StockUpdate,
	_ string,
) (domain.BulkStockResult, error) {
	f.calls++
	f.updates = append([]domain.StockUpdate(nil), updates...)
	if f.err != nil {
		return domain.BulkStockResult{}, f.err
	}

	result := f.result
	if result.UpdatedCount == 0 && len(result.Errors) == 0 {
		result.UpdatedCount = int32(len(updates))
	}
	return result, nil
}

type pricingFake struct {
	err     error
	calls   int
	updates []domain.PriceUpdate
}

func (f *pricingFake) SetPrices(
	_ context.Context,
	_ string,
	updates []domain.PriceUpdate,
) error {
	f.calls++
	f.updates = append([]domain.PriceUpdate(nil), updates...)
	return f.err
}

type jobsFake struct {
	saved map[string]domain.SyncJob
}

func (f *jobsFake) Save(_ context.Context, job domain.SyncJob) error {
	if f.saved == nil {
		f.saved = make(map[string]domain.SyncJob)
	}
	f.saved[job.JobID] = job
	return nil
}

func (f *jobsFake) GetByID(
	_ context.Context,
	jobID string,
) (domain.SyncJob, error) {
	job, ok := f.saved[jobID]
	if !ok {
		return domain.SyncJob{}, errors.New("job not found")
	}
	return job, nil
}

func (f *jobsFake) ListByPharmacy(
	_ context.Context,
	_ string,
	_ int32,
	_ string,
) ([]domain.SyncJob, string, error) {
	return nil, "", nil
}

func newTestService(
	catalog *catalogFake,
	inventory *inventoryFake,
	pricing *pricingFake,
	jobs *jobsFake,
) *Service {
	fixedTime := time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)
	return NewService(catalog, inventory, pricing, jobs, func() time.Time {
		return fixedTime
	})
}

func TestUploadStockCSVImportsRowsAndOptionalPrices(t *testing.T) {
	catalog := &catalogFake{}
	inventory := &inventoryFake{}
	pricing := &pricingFake{}
	jobs := &jobsFake{}
	service := newTestService(catalog, inventory, pricing, jobs)

	job, err := service.UploadStockCSV(context.Background(), UploadCSVRequest{
		PharmacyID:       "pharmacy-1",
		UploadedByUserID: "user-1",
		Filename:         "stock.csv",
		Content: []byte(
			"product_id,quantity,price_chhertum\n" +
				"med-1,3,2500\n" +
				"med-2,0,\n",
		),
	})
	if err != nil {
		t.Fatalf("UploadStockCSV returned error: %v", err)
	}
	if job.Status != domain.JobStatusCompleted {
		t.Fatalf("status = %v, want completed", job.Status)
	}
	if job.RowsTotal != 2 || job.RowsApplied != 2 || job.RowsRejected != 0 {
		t.Fatalf("row counts = total:%d applied:%d rejected:%d",
			job.RowsTotal, job.RowsApplied, job.RowsRejected)
	}
	if catalog.calls != 1 || inventory.calls != 1 || pricing.calls != 1 {
		t.Fatalf("calls = catalog:%d inventory:%d pricing:%d, want 1 each",
			catalog.calls, inventory.calls, pricing.calls)
	}
	if len(inventory.updates) != 2 {
		t.Fatalf("inventory received %d updates, want 2", len(inventory.updates))
	}
	if len(pricing.updates) != 1 ||
		pricing.updates[0].ProductID != "med-1" ||
		pricing.updates[0].PriceChhertum != 2500 {
		t.Fatalf("price updates = %+v", pricing.updates)
	}
	if _, ok := jobs.saved[job.JobID]; !ok {
		t.Fatal("completed job was not saved")
	}
}

func TestUploadStockCSVReportsEmptyFileAndBadHeader(t *testing.T) {
	tests := []struct {
		name    string
		content []byte
	}{
		{name: "empty file", content: nil},
		{name: "bad header", content: []byte("medicine,amount\nmed-1,2\n")},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			catalog := &catalogFake{}
			inventory := &inventoryFake{}
			pricing := &pricingFake{}
			jobs := &jobsFake{}
			service := newTestService(catalog, inventory, pricing, jobs)

			job, err := service.UploadStockCSV(context.Background(), UploadCSVRequest{
				PharmacyID:       "pharmacy-1",
				UploadedByUserID: "user-1",
				Filename:         "bad.csv",
				Content:          test.content,
			})
			if err != nil {
				t.Fatalf("UploadStockCSV returned error: %v", err)
			}
			if job.Status != domain.JobStatusFailed {
				t.Fatalf("status = %v, want failed", job.Status)
			}
			if len(job.RowErrors) == 0 {
				t.Fatal("failed job has no error report")
			}
			if catalog.calls != 0 || inventory.calls != 0 || pricing.calls != 0 {
				t.Fatalf("downstream calls = catalog:%d inventory:%d pricing:%d, want 0",
					catalog.calls, inventory.calls, pricing.calls)
			}
		})
	}
}

func TestUploadStockCSVReportsUnknownAndNegativeRows(t *testing.T) {
	catalog := &catalogFake{existing: map[string]bool{"known": true}}
	inventory := &inventoryFake{}
	pricing := &pricingFake{}
	jobs := &jobsFake{}
	service := newTestService(catalog, inventory, pricing, jobs)

	job, err := service.UploadStockCSV(context.Background(), UploadCSVRequest{
		PharmacyID:       "pharmacy-1",
		UploadedByUserID: "user-1",
		Filename:         "stock.csv",
		Content: []byte(
			"product_id,quantity\n" +
				"known,2\n" +
				"unknown,4\n" +
				"negative,-1\n",
		),
	})
	if err != nil {
		t.Fatalf("UploadStockCSV returned error: %v", err)
	}
	if job.Status != domain.JobStatusPartial {
		t.Fatalf("status = %v, want partial", job.Status)
	}
	if job.RowsTotal != 3 || job.RowsApplied != 1 || job.RowsRejected != 2 {
		t.Fatalf("row counts = total:%d applied:%d rejected:%d",
			job.RowsTotal, job.RowsApplied, job.RowsRejected)
	}
	if inventory.calls != 1 || len(inventory.updates) != 1 ||
		inventory.updates[0].ProductID != "known" {
		t.Fatalf("inventory calls/updates = %d/%+v", inventory.calls, inventory.updates)
	}
	if len(job.RowErrors) != 2 {
		t.Fatalf("row errors = %+v, want 2", job.RowErrors)
	}
}

func TestUpload500RowsUsesOneInventoryCall(t *testing.T) {
	var csv strings.Builder
	csv.WriteString("product_id,quantity\n")
	for i := 0; i < 500; i++ {
		fmt.Fprintf(&csv, "med-%03d,%d\n", i, i%20)
	}

	catalog := &catalogFake{}
	inventory := &inventoryFake{}
	pricing := &pricingFake{}
	jobs := &jobsFake{}
	service := newTestService(catalog, inventory, pricing, jobs)

	job, err := service.UploadStockCSV(context.Background(), UploadCSVRequest{
		PharmacyID:       "pharmacy-1",
		UploadedByUserID: "user-1",
		Filename:         "500-stock.csv",
		Content:          []byte(csv.String()),
	})
	if err != nil {
		t.Fatalf("UploadStockCSV returned error: %v", err)
	}
	if job.Status != domain.JobStatusCompleted {
		t.Fatalf("status = %v, want completed", job.Status)
	}
	if job.RowsTotal != 500 || job.RowsApplied != 500 || job.RowsRejected != 0 {
		t.Fatalf("row counts = total:%d applied:%d rejected:%d",
			job.RowsTotal, job.RowsApplied, job.RowsRejected)
	}
	if catalog.calls != 1 {
		t.Fatalf("Catalog calls = %d, want 1", catalog.calls)
	}
	if inventory.calls != 1 || len(inventory.updates) != 500 {
		t.Fatalf("Inventory calls/updates = %d/%d, want 1/500",
			inventory.calls, len(inventory.updates))
	}
	if pricing.calls != 0 {
		t.Fatalf("Pricing calls = %d, want 0 for a stock-only CSV", pricing.calls)
	}
}
