package grpc

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	commonv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/common/v1"
	stocksyncv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/stock-sync/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/stock-sync/internal/app"
	"github.com/Swiftmeds-bht/Swiftmeds/services/stock-sync/internal/domain"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var now = time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)

type allCatalog struct{}

func (allCatalog) GetExistingProductIDs(_ context.Context, ids []string) ([]string, error) {
	return ids, nil
}

type okInventory struct{ errs []domain.StockUpdateError }

func (i okInventory) BulkSetStock(_ context.Context, _ string, u []domain.StockUpdate, _ string) (domain.BulkStockResult, error) {
	return domain.BulkStockResult{UpdatedCount: int32(len(u)), Errors: i.errs}, nil
}

type okPricing struct{}

func (okPricing) SetPrices(context.Context, string, []domain.PriceUpdate) error { return nil }

// jobStore is an in-memory JobRepository; getErr and listErr make those calls fail.
type jobStore struct {
	jobs                    map[string]domain.SyncJob
	getErr, listErr         error
	listPharmacy, listToken string
	listSize                int32
}

func (s *jobStore) Save(_ context.Context, job domain.SyncJob) error {
	if s.jobs == nil {
		s.jobs = map[string]domain.SyncJob{}
	}
	s.jobs[job.JobID] = job
	return nil
}

func (s *jobStore) GetByID(_ context.Context, id string) (domain.SyncJob, error) {
	if s.getErr != nil {
		return domain.SyncJob{}, s.getErr
	}
	job, ok := s.jobs[id]
	if !ok {
		return domain.SyncJob{}, domain.ErrJobNotFound
	}
	return job, nil
}

func (s *jobStore) ListByPharmacy(_ context.Context, pharmacyID string, size int32, token string) ([]domain.SyncJob, string, error) {
	s.listPharmacy, s.listSize, s.listToken = pharmacyID, size, token
	if s.listErr != nil {
		return nil, "", s.listErr
	}
	var out []domain.SyncJob
	for _, j := range s.jobs {
		if j.PharmacyID == pharmacyID {
			out = append(out, j)
		}
	}
	return out, "page-2", nil
}

func newHandler(store *jobStore, inv okInventory) *Handler {
	return NewHandler(app.NewService(allCatalog{}, inv, okPricing{}, store, func() time.Time { return now }))
}

func wantCode(t *testing.T, err error, want codes.Code) {
	t.Helper()
	if got := status.Code(err); got != want {
		t.Fatalf("code = %s (%v), want %s", got, err, want)
	}
}

func TestUploadStockCsvReturnsTheJob(t *testing.T) {
	store := &jobStore{}
	inv := okInventory{errs: []domain.StockUpdateError{{ProductID: "med-2", Message: "below reserved"}}}
	resp, err := newHandler(store, inv).UploadStockCsv(context.Background(), &stocksyncv1.UploadStockCsvRequest{
		PharmacyId: "ph-1", UploadedByUserId: "u-1", Filename: "stock.csv",
		CsvContent: []byte("product_id,quantity\nmed-1,2\nmed-2,1\n,4\n"),
	})
	if err != nil {
		t.Fatalf("UploadStockCsv: %v", err)
	}
	job := resp.GetJob()
	if job.GetJobId() == "" || job.GetPharmacyId() != "ph-1" || job.GetUploadedByUserId() != "u-1" || job.GetFilename() != "stock.csv" {
		t.Fatalf("job = %v", job)
	}
	if job.GetStatus() != stocksyncv1.SyncJobStatus_SYNC_JOB_STATUS_PARTIAL ||
		job.GetRowsTotal() != 3 || job.GetRowsApplied() != 1 || job.GetRowsRejected() != 2 {
		t.Fatalf("status %s total %d applied %d rejected %d, want PARTIAL 3/1/2",
			job.GetStatus(), job.GetRowsTotal(), job.GetRowsApplied(), job.GetRowsRejected())
	}
	// Errors with a product say which product; errors without one do not.
	errs := job.GetRowErrors()
	if len(errs) != 2 ||
		errs[0].GetRowNumber() != 3 || errs[0].GetMessage() != "med-2: below reserved" ||
		errs[1].GetRowNumber() != 4 || errs[1].GetMessage() != "product_id is required" {
		t.Fatalf("row errors = %v", errs)
	}
	if !job.GetCreatedAt().AsTime().Equal(now) || !job.GetFinishedAt().AsTime().Equal(now) {
		t.Fatalf("times = %v, %v; want %v", job.GetCreatedAt(), job.GetFinishedAt(), now)
	}
	if _, ok := store.jobs[job.GetJobId()]; !ok {
		t.Fatal("job was not stored")
	}
}

func TestUploadStockCsvValidation(t *testing.T) {
	h := newHandler(&jobStore{}, okInventory{})
	_, err := h.UploadStockCsv(context.Background(), &stocksyncv1.UploadStockCsvRequest{UploadedByUserId: "u-1"})
	wantCode(t, err, codes.InvalidArgument)
	_, err = h.UploadStockCsv(context.Background(), &stocksyncv1.UploadStockCsvRequest{PharmacyId: "ph-1"})
	wantCode(t, err, codes.InvalidArgument)
}

func TestEveryRPCRejectsANilRequest(t *testing.T) {
	h := newHandler(&jobStore{}, okInventory{})
	_, err1 := h.UploadStockCsv(context.Background(), nil)
	_, err2 := h.GetSyncJob(context.Background(), nil)
	_, err3 := h.ListSyncJobs(context.Background(), nil)
	for i, err := range []error{err1, err2, err3} {
		if status.Code(err) != codes.InvalidArgument {
			t.Errorf("rpc %d: code = %s, want InvalidArgument", i+1, status.Code(err))
		}
	}
}

func TestGetSyncJobStatusCodes(t *testing.T) {
	store := &jobStore{jobs: map[string]domain.SyncJob{
		"job-1": {JobID: "job-1", Status: domain.JobStatusCompleted, CreatedAt: now, FinishedAt: now},
	}}
	h := newHandler(store, okInventory{})
	ctx := context.Background()

	resp, err := h.GetSyncJob(ctx, &stocksyncv1.GetSyncJobRequest{JobId: "job-1"})
	if err != nil || resp.GetJob().GetStatus() != stocksyncv1.SyncJobStatus_SYNC_JOB_STATUS_COMPLETED {
		t.Fatalf("GetSyncJob = %v, %v", resp, err)
	}

	_, err = h.GetSyncJob(ctx, &stocksyncv1.GetSyncJobRequest{})
	wantCode(t, err, codes.InvalidArgument)

	_, err = h.GetSyncJob(ctx, &stocksyncv1.GetSyncJobRequest{JobId: "missing"})
	wantCode(t, err, codes.NotFound)

	store.getErr = status.Error(codes.Unavailable, "database is restarting")
	_, err = h.GetSyncJob(ctx, &stocksyncv1.GetSyncJobRequest{JobId: "job-1"})
	wantCode(t, err, codes.Unavailable)

	store.getErr = errors.New("dial tcp 10.0.0.5:5432: password authentication failed")
	_, err = h.GetSyncJob(ctx, &stocksyncv1.GetSyncJobRequest{JobId: "job-1"})
	wantCode(t, err, codes.Internal)
	if strings.Contains(err.Error(), "password") {
		t.Fatalf("error %q leaks the internal cause", err)
	}
}

func TestListSyncJobsPassesPageAndReturnsNextToken(t *testing.T) {
	store := &jobStore{jobs: map[string]domain.SyncJob{
		"job-1": {JobID: "job-1", PharmacyID: "ph-1", Status: domain.JobStatusFailed},
		"job-2": {JobID: "job-2", PharmacyID: "ph-2", Status: domain.JobStatusCompleted},
	}}
	h := newHandler(store, okInventory{})

	resp, err := h.ListSyncJobs(context.Background(), &stocksyncv1.ListSyncJobsRequest{
		PharmacyId: "ph-1", Page: &commonv1.PageRequest{PageSize: 7, PageToken: "tok"},
	})
	if err != nil {
		t.Fatalf("ListSyncJobs: %v", err)
	}
	if len(resp.GetJobs()) != 1 || resp.GetJobs()[0].GetJobId() != "job-1" ||
		resp.GetJobs()[0].GetStatus() != stocksyncv1.SyncJobStatus_SYNC_JOB_STATUS_FAILED {
		t.Fatalf("jobs = %v, want job-1 FAILED", resp.GetJobs())
	}
	if resp.GetPage().GetNextPageToken() != "page-2" {
		t.Fatalf("next token = %q, want page-2", resp.GetPage().GetNextPageToken())
	}
	if store.listSize != 7 || store.listToken != "tok" {
		t.Fatalf("repository got size %d token %q, want 7 and tok", store.listSize, store.listToken)
	}
}

func TestListSyncJobsWithoutAPageUsesTheDefault(t *testing.T) {
	store := &jobStore{}
	resp, err := newHandler(store, okInventory{}).ListSyncJobs(context.Background(),
		&stocksyncv1.ListSyncJobsRequest{PharmacyId: "ph-1"})
	if err != nil {
		t.Fatalf("ListSyncJobs: %v", err)
	}
	if store.listSize != 20 || store.listToken != "" {
		t.Fatalf("repository got size %d token %q, want the default 20 and no token", store.listSize, store.listToken)
	}
	if resp.GetJobs() == nil || len(resp.GetJobs()) != 0 {
		t.Fatalf("jobs = %#v, want an empty list", resp.GetJobs())
	}
}

func TestListSyncJobsErrors(t *testing.T) {
	_, err := newHandler(&jobStore{}, okInventory{}).ListSyncJobs(context.Background(), &stocksyncv1.ListSyncJobsRequest{})
	wantCode(t, err, codes.InvalidArgument)

	_, err = newHandler(&jobStore{listErr: domain.ErrInvalidPageToken}, okInventory{}).ListSyncJobs(context.Background(),
		&stocksyncv1.ListSyncJobsRequest{PharmacyId: "ph-1", Page: &commonv1.PageRequest{PageToken: "garbage"}})
	wantCode(t, err, codes.InvalidArgument)
}

func TestToProtoStatus(t *testing.T) {
	cases := map[domain.JobStatus]stocksyncv1.SyncJobStatus{
		domain.JobStatusProcessing: stocksyncv1.SyncJobStatus_SYNC_JOB_STATUS_PROCESSING,
		domain.JobStatusCompleted:  stocksyncv1.SyncJobStatus_SYNC_JOB_STATUS_COMPLETED,
		domain.JobStatusPartial:    stocksyncv1.SyncJobStatus_SYNC_JOB_STATUS_PARTIAL,
		domain.JobStatusFailed:     stocksyncv1.SyncJobStatus_SYNC_JOB_STATUS_FAILED,
		domain.JobStatus(0):        stocksyncv1.SyncJobStatus_SYNC_JOB_STATUS_UNSPECIFIED,
		domain.JobStatus(99):       stocksyncv1.SyncJobStatus_SYNC_JOB_STATUS_UNSPECIFIED,
	}
	for in, want := range cases {
		if got := toProtoStatus(in); got != want {
			t.Errorf("toProtoStatus(%d) = %s, want %s", in, got, want)
		}
	}
}
