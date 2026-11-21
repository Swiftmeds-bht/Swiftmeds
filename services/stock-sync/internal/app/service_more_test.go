package app

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/Swiftmeds-bht/Swiftmeds/services/stock-sync/internal/domain"
)

var errDown = errors.New("connection refused")

// recordingJobs is a JobRepository that records the List arguments and can fail.
type recordingJobs struct {
	jobsFake
	saveErr error
	saves   int

	listPharmacy string
	listSize     int32
	listToken    string
	getID        string
}

func (r *recordingJobs) Save(ctx context.Context, job domain.SyncJob) error {
	r.saves++
	if r.saveErr != nil {
		return r.saveErr
	}
	return r.jobsFake.Save(ctx, job)
}

func (r *recordingJobs) GetByID(ctx context.Context, jobID string) (domain.SyncJob, error) {
	r.getID = jobID
	return r.jobsFake.GetByID(ctx, jobID)
}

func (r *recordingJobs) ListByPharmacy(_ context.Context, pharmacyID string, size int32, token string) ([]domain.SyncJob, string, error) {
	r.listPharmacy, r.listSize, r.listToken = pharmacyID, size, token
	return []domain.SyncJob{{JobID: "j-1"}}, "next", nil
}

var fixedNow = time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)

func upload(t *testing.T, svc *Service, csv string) domain.SyncJob {
	t.Helper()
	job, err := svc.UploadStockCSV(context.Background(), UploadCSVRequest{
		PharmacyID: " pharmacy-1 ", UploadedByUserID: " user-1 ", Filename: "  stock.csv ", Content: []byte(csv),
	})
	if err != nil {
		t.Fatalf("UploadStockCSV: %v", err)
	}
	return job
}

func TestUploadRequiresPharmacyAndUploader(t *testing.T) {
	tests := []struct {
		name    string
		req     UploadCSVRequest
		wantErr error
	}{
		{"blank pharmacy", UploadCSVRequest{PharmacyID: " ", UploadedByUserID: "u-1"}, domain.ErrMissingPharmacyID},
		{"blank uploader", UploadCSVRequest{PharmacyID: "ph-1", UploadedByUserID: "\t"}, domain.ErrMissingUploaderID},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jobs := &recordingJobs{}
			catalog := &catalogFake{}
			svc := NewService(catalog, &inventoryFake{}, &pricingFake{}, jobs, nil)
			_, err := svc.UploadStockCSV(context.Background(), tt.req)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
			if jobs.saves != 0 || catalog.calls != 0 {
				t.Fatalf("saves = %d, catalog calls = %d; want no job and no lookup", jobs.saves, catalog.calls)
			}
		})
	}
}

func TestUploadRecordsJobDetails(t *testing.T) {
	jobs := &recordingJobs{}
	svc := NewService(&catalogFake{}, &inventoryFake{}, &pricingFake{}, jobs,
		func() time.Time { return fixedNow.In(time.FixedZone("BTT", 6*3600)) })

	job := upload(t, svc, "product_id,quantity\nmed-1,1\n")

	if !regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`).MatchString(job.JobID) {
		t.Errorf("job ID %q is not UUID-shaped", job.JobID)
	}
	if job.PharmacyID != "pharmacy-1" || job.UploadedByUserID != "user-1" || job.Filename != "stock.csv" {
		t.Errorf("job = %+v, want trimmed pharmacy, uploader and filename", job)
	}
	if !job.CreatedAt.Equal(fixedNow) || job.CreatedAt.Location() != time.UTC ||
		!job.FinishedAt.Equal(fixedNow) || job.FinishedAt.Location() != time.UTC {
		t.Errorf("created %v, finished %v; want both %v in UTC", job.CreatedAt, job.FinishedAt, fixedNow)
	}
	if saved, ok := jobs.saved[job.JobID]; !ok || saved.Status != domain.JobStatusCompleted {
		t.Errorf("saved job = %+v (found %v), want the completed job", saved, ok)
	}
}

func TestUploadJobIDsAreUnique(t *testing.T) {
	svc := newTestService(&catalogFake{}, &inventoryFake{}, &pricingFake{}, &jobsFake{})
	first := upload(t, svc, "product_id,quantity\nmed-1,1\n")
	second := upload(t, svc, "product_id,quantity\nmed-1,1\n")
	if first.JobID == second.JobID {
		t.Fatalf("two uploads got the same job ID %q", first.JobID)
	}
}

// One file with every kind of row: clean, without price, unparsable, rejected
// by Inventory and unknown to Catalog. The errors come back sorted by row.
func TestUploadMixedFileIsPartial(t *testing.T) {
	catalog := &catalogFake{existing: map[string]bool{"med-1": true, "med-2": true, "med-3": true}}
	inventory := &inventoryFake{result: domain.BulkStockResult{
		UpdatedCount: 2,
		Errors:       []domain.StockUpdateError{{ProductID: " MED-3 ", Message: "on-hand quantity cannot be below reserved quantity"}},
	}}
	pricing := &pricingFake{}
	svc := newTestService(catalog, inventory, pricing, &jobsFake{})

	job := upload(t, svc, "product_id,quantity,price_chhertum\n"+
		"ghost,1,\n"+ // row 2: unknown product
		"med-1,3,2500\n"+ // row 3: applied with a price
		"bad,x,\n"+ // row 4: not a number
		"med-3,2,100\n"+ // row 5: Inventory refuses it
		"med-2,1,\n") // row 6: applied, no price

	if job.Status != domain.JobStatusPartial {
		t.Fatalf("status = %v, want partial", job.Status)
	}
	if job.RowsTotal != 5 || job.RowsApplied != 2 || job.RowsRejected != 3 {
		t.Fatalf("rows total %d applied %d rejected %d, want 5/2/3", job.RowsTotal, job.RowsApplied, job.RowsRejected)
	}
	want := []domain.RowError{
		{RowNumber: 2, ProductID: "ghost", Message: "unknown product_id"},
		{RowNumber: 4, ProductID: "bad", Message: "quantity must be a whole number"},
		{RowNumber: 5, ProductID: "med-3", Message: "on-hand quantity cannot be below reserved quantity"},
	}
	if len(job.RowErrors) != len(want) {
		t.Fatalf("row errors = %+v, want %+v", job.RowErrors, want)
	}
	for i := range want {
		if job.RowErrors[i] != want[i] {
			t.Errorf("row error %d = %+v, want %+v", i, job.RowErrors[i], want[i])
		}
	}
	// Only known products reach Inventory, and only stock that was applied gets a price.
	if len(inventory.updates) != 3 {
		t.Errorf("inventory updates = %+v, want med-1, med-3, med-2", inventory.updates)
	}
	if len(pricing.updates) != 1 || pricing.updates[0] != (domain.PriceUpdate{ProductID: "med-1", PriceChhertum: 2500}) {
		t.Errorf("price updates = %+v, want only med-1 at 2500", pricing.updates)
	}
}

func TestUploadCatalogLookupMatchesIgnoringCase(t *testing.T) {
	catalog := &catalogFake{existing: map[string]bool{"med-1": true}}
	svc := newTestService(catalog, &inventoryFake{}, &pricingFake{}, &jobsFake{})

	job := upload(t, svc, "product_id,quantity\nMED-1,4\n")

	if job.Status != domain.JobStatusCompleted || job.RowsApplied != 1 {
		t.Fatalf("job = %+v, want MED-1 applied", job)
	}
	if len(catalog.askedIDs) != 1 || catalog.askedIDs[0] != "MED-1" {
		t.Fatalf("catalog was asked for %v, want [MED-1]", catalog.askedIDs)
	}
}

func TestUploadFailsEveryRowWhenADependencyIsDown(t *testing.T) {
	csv := "product_id,quantity,price_chhertum\nmed-1,3,2500\nmed-2,1,\n"
	tests := []struct {
		name          string
		catalog       *catalogFake
		inventory     *inventoryFake
		pricing       *pricingFake
		wantMessage   string
		wantInventory int
		wantPricing   int
		wantStatus    domain.JobStatus
		wantApplied   int32
		wantRejected  int32
	}{
		{
			name: "catalog", catalog: &catalogFake{err: errDown}, inventory: &inventoryFake{}, pricing: &pricingFake{},
			wantMessage: "catalog lookup failed: connection refused", wantStatus: domain.JobStatusFailed, wantRejected: 2,
		},
		{
			name: "inventory", catalog: &catalogFake{}, inventory: &inventoryFake{err: errDown}, pricing: &pricingFake{},
			wantMessage: "inventory bulk update failed: connection refused", wantInventory: 1,
			wantStatus: domain.JobStatusFailed, wantRejected: 2,
		},
		{
			// Stock was set, but the price was not: the priced row is reported, the other row counts as applied.
			name: "pricing", catalog: &catalogFake{}, inventory: &inventoryFake{}, pricing: &pricingFake{err: errDown},
			wantMessage: "price update failed after stock update: connection refused", wantInventory: 1, wantPricing: 1,
			wantStatus: domain.JobStatusPartial, wantApplied: 1, wantRejected: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jobs := &recordingJobs{}
			svc := NewService(tt.catalog, tt.inventory, tt.pricing, jobs, func() time.Time { return fixedNow })

			job := upload(t, svc, csv)

			if job.Status != tt.wantStatus || job.RowsApplied != tt.wantApplied || job.RowsRejected != tt.wantRejected {
				t.Fatalf("job status %v applied %d rejected %d, want %v/%d/%d",
					job.Status, job.RowsApplied, job.RowsRejected, tt.wantStatus, tt.wantApplied, tt.wantRejected)
			}
			for _, e := range job.RowErrors {
				if e.Message != tt.wantMessage {
					t.Errorf("row error %+v, want message %q", e, tt.wantMessage)
				}
			}
			if tt.inventory.calls != tt.wantInventory || tt.pricing.calls != tt.wantPricing {
				t.Errorf("calls inventory %d pricing %d, want %d/%d",
					tt.inventory.calls, tt.pricing.calls, tt.wantInventory, tt.wantPricing)
			}
			if jobs.saves != 1 {
				t.Errorf("job saved %d times, want once (the failure is recorded)", jobs.saves)
			}
		})
	}
}

func TestUploadWithOnlyUnknownProductsFailsWithoutCallingInventory(t *testing.T) {
	inventory := &inventoryFake{}
	svc := newTestService(&catalogFake{existing: map[string]bool{}}, inventory, &pricingFake{}, &jobsFake{})

	job := upload(t, svc, "product_id,quantity\nghost-1,1\nghost-2,2\n")

	if job.Status != domain.JobStatusFailed || job.RowsRejected != 2 || job.RowsApplied != 0 {
		t.Fatalf("job = %+v, want failed with 2 rejected", job)
	}
	if inventory.calls != 0 {
		t.Fatalf("inventory calls = %d, want 0", inventory.calls)
	}
}

func TestUploadWithOnlyInvalidRowsSkipsCatalog(t *testing.T) {
	catalog := &catalogFake{}
	svc := newTestService(catalog, &inventoryFake{}, &pricingFake{}, &jobsFake{})

	job := upload(t, svc, "product_id,quantity\nmed-1,-2\n,3\n")

	if job.Status != domain.JobStatusFailed || job.RowsTotal != 2 || job.RowsRejected != 2 {
		t.Fatalf("job = %+v, want failed with 2 of 2 rejected", job)
	}
	if catalog.calls != 0 {
		t.Fatalf("catalog calls = %d, want 0", catalog.calls)
	}
}

// A file-level error is reported on row 1. The file has no data rows, so the
// rejected count stays at 0: it never exceeds the row count.
func TestUploadFileLevelErrorIsReportedOnRowOne(t *testing.T) {
	svc := newTestService(&catalogFake{}, &inventoryFake{}, &pricingFake{}, &jobsFake{})

	job := upload(t, svc, "product_id,quantity\n")

	if job.Status != domain.JobStatusFailed || job.RowsTotal != 0 || job.RowsRejected != 0 {
		t.Fatalf("job = %+v, want failed with no rows", job)
	}
	if len(job.RowErrors) != 1 || job.RowErrors[0].RowNumber != 1 ||
		job.RowErrors[0].Message != domain.ErrNoCSVRows.Error() {
		t.Fatalf("row errors = %+v, want the file error on row 1", job.RowErrors)
	}
}

func TestUploadReturnsASaveError(t *testing.T) {
	jobs := &recordingJobs{saveErr: errDown}
	svc := NewService(&catalogFake{}, &inventoryFake{}, &pricingFake{}, jobs, nil)

	_, err := svc.UploadStockCSV(context.Background(), UploadCSVRequest{
		PharmacyID: "ph-1", UploadedByUserID: "u-1", Content: []byte("product_id,quantity\nmed-1,1\n"),
	})
	if !errors.Is(err, errDown) || !strings.Contains(err.Error(), "save sync job") {
		t.Fatalf("error = %v, want the wrapped save error", err)
	}
}

func TestGetSyncJob(t *testing.T) {
	jobs := &recordingJobs{}
	jobs.saved = map[string]domain.SyncJob{"job-1": {JobID: "job-1", PharmacyID: "ph-1"}}
	svc := NewService(&catalogFake{}, &inventoryFake{}, &pricingFake{}, jobs, nil)

	if _, err := svc.GetSyncJob(context.Background(), "  "); !errors.Is(err, domain.ErrMissingJobID) {
		t.Fatalf("blank ID: error = %v, want ErrMissingJobID", err)
	}
	if jobs.getID != "" {
		t.Fatalf("repository was asked for %q, want no call", jobs.getID)
	}
	got, err := svc.GetSyncJob(context.Background(), " job-1 ")
	if err != nil || got.JobID != "job-1" || jobs.getID != "job-1" {
		t.Fatalf("GetSyncJob = %+v, %v (asked %q); want job-1 with a trimmed ID", got, err, jobs.getID)
	}
}

func TestListSyncJobsPageSizeAndToken(t *testing.T) {
	tests := []struct {
		size, want int32
	}{
		{-1, defaultPageSize},
		{0, defaultPageSize},
		{1, 1},
		{maxPageSize, maxPageSize},
		{maxPageSize + 1, maxPageSize},
	}
	for _, tt := range tests {
		jobs := &recordingJobs{}
		svc := NewService(&catalogFake{}, &inventoryFake{}, &pricingFake{}, jobs, nil)
		got, next, err := svc.ListSyncJobs(context.Background(), ListJobsRequest{
			PharmacyID: " ph-1 ", PageSize: tt.size, PageToken: " tok ",
		})
		if err != nil || len(got) != 1 || next != "next" {
			t.Fatalf("size %d: ListSyncJobs = %v, %q, %v", tt.size, got, next, err)
		}
		if jobs.listPharmacy != "ph-1" || jobs.listSize != tt.want || jobs.listToken != "tok" {
			t.Errorf("size %d: repository got (%q, %d, %q), want (ph-1, %d, tok)",
				tt.size, jobs.listPharmacy, jobs.listSize, jobs.listToken, tt.want)
		}
	}
}

func TestListSyncJobsRequiresPharmacy(t *testing.T) {
	jobs := &recordingJobs{}
	svc := NewService(&catalogFake{}, &inventoryFake{}, &pricingFake{}, jobs, nil)
	if _, _, err := svc.ListSyncJobs(context.Background(), ListJobsRequest{PharmacyID: " "}); !errors.Is(err, domain.ErrMissingPharmacyID) {
		t.Fatalf("error = %v, want ErrMissingPharmacyID", err)
	}
	if jobs.listPharmacy != "" {
		t.Fatal("repository was called without a pharmacy")
	}
}
