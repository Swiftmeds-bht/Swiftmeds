package app

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	authv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/auth/v1"
	stocksyncv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/stock-sync/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/gateway/internal/ports"
)

type fakeStockSyncClient struct {
	upload *stocksyncv1.UploadStockCsvRequest
	get    *stocksyncv1.GetSyncJobRequest
	list   *stocksyncv1.ListSyncJobsRequest
	job    *stocksyncv1.SyncJob
}

func (f *fakeStockSyncClient) UploadStockCsv(_ context.Context, req *stocksyncv1.UploadStockCsvRequest) (*stocksyncv1.UploadStockCsvResponse, error) {
	f.upload = req
	return &stocksyncv1.UploadStockCsvResponse{Job: f.job}, nil
}

func (f *fakeStockSyncClient) GetSyncJob(_ context.Context, req *stocksyncv1.GetSyncJobRequest) (*stocksyncv1.GetSyncJobResponse, error) {
	f.get = req
	return &stocksyncv1.GetSyncJobResponse{Job: f.job}, nil
}

func (f *fakeStockSyncClient) ListSyncJobs(_ context.Context, req *stocksyncv1.ListSyncJobsRequest) (*stocksyncv1.ListSyncJobsResponse, error) {
	f.list = req
	return &stocksyncv1.ListSyncJobsResponse{Jobs: []*stocksyncv1.SyncJob{f.job}}, nil
}

func partialJob(pharmacyID string) *stocksyncv1.SyncJob {
	return &stocksyncv1.SyncJob{
		JobId: "job-1", PharmacyId: pharmacyID, Status: stocksyncv1.SyncJobStatus_SYNC_JOB_STATUS_PARTIAL,
		RowsTotal: 2, RowsApplied: 1, RowsRejected: 1,
		RowErrors: []*stocksyncv1.RowError{{RowNumber: 3, Message: "p-x: unknown product"}},
	}
}

// multipartBody builds a form with one part named field holding content.
func multipartBody(t *testing.T, field, filename string, content []byte) (*bytes.Buffer, string) {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile(field, filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return &body, writer.FormDataContentType()
}

func uploadRequest(t *testing.T, field, filename string, content []byte, identity ports.Identity) *http.Request {
	t.Helper()
	body, contentType := multipartBody(t, field, filename, content)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/pharmacies/ph-1/stock-sync/jobs", body)
	req.Header.Set("Content-Type", contentType)
	req = withIdentity(req, identity)
	req.SetPathValue("pharmacyId", "ph-1")
	return req
}

func TestStockSyncUploadReturnsRowReport(t *testing.T) {
	fake := &fakeStockSyncClient{job: partialJob("ph-1")}
	csv := []byte("product_id,quantity\np-1,10\np-x,4\n")
	req := uploadRequest(t, "file", `C:\Users\staff\stock.csv`, csv, staffOfPharmacy1)

	rec := httptest.NewRecorder()
	stockSyncHandler(fake, nil)(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("got %d, want 201: %s", rec.Code, rec.Body.String())
	}
	if fake.upload.GetPharmacyId() != "ph-1" || fake.upload.GetUploadedByUserId() != "staff-1" {
		t.Fatalf("unexpected downstream request: %v", fake.upload)
	}
	if fake.upload.GetFilename() != "stock.csv" {
		t.Fatalf("filename = %q, want the base name only", fake.upload.GetFilename())
	}
	if !bytes.Equal(fake.upload.GetCsvContent(), csv) {
		t.Fatalf("csv content was changed: %q", fake.upload.GetCsvContent())
	}
	for _, want := range []string{`"rowsRejected":1`, `"rowNumber":3`, "unknown product", "SYNC_JOB_STATUS_PARTIAL"} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Fatalf("response is missing %s: %s", want, rec.Body.String())
		}
	}
}

func TestStockSyncUploadRejectsBadRequests(t *testing.T) {
	tooBig := bytes.Repeat([]byte("a"), MaxStockCSVBytes+1)
	tests := map[string]struct {
		field   string
		content []byte
		want    int
	}{
		"missing file part": {field: "other", content: []byte("product_id,quantity\n"), want: http.StatusBadRequest},
		"not utf-8":         {field: "file", content: []byte{0xff, 0xfe, 'a'}, want: http.StatusBadRequest},
		"larger than 1 MB":  {field: "file", content: tooBig, want: http.StatusRequestEntityTooLarge},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			fake := &fakeStockSyncClient{}
			req := uploadRequest(t, tt.field, "stock.csv", tt.content, staffOfPharmacy1)

			rec := httptest.NewRecorder()
			stockSyncHandler(fake, nil)(rec, req)

			if rec.Code != tt.want {
				t.Fatalf("got %d, want %d: %s", rec.Code, tt.want, rec.Body.String())
			}
			if fake.upload != nil {
				t.Fatal("downstream call should not be made")
			}
		})
	}
}

func TestStockSyncUploadRejectsNonMultipart(t *testing.T) {
	fake := &fakeStockSyncClient{}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/pharmacies/ph-1/stock-sync/jobs", strings.NewReader(`{"file":"x"}`))
	req.Header.Set("Content-Type", "application/json")
	req = withIdentity(req, staffOfPharmacy1)
	req.SetPathValue("pharmacyId", "ph-1")

	rec := httptest.NewRecorder()
	stockSyncHandler(fake, nil)(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("got %d, want 400", rec.Code)
	}
}

func TestStockSyncUploadIsStaffOfOwnPharmacyOnly(t *testing.T) {
	csv := []byte("product_id,quantity\np-1,1\n")
	for name, identity := range map[string]ports.Identity{
		"admin":          {UserID: "admin-1", PharmacyID: "ph-1", Role: authv1.Role_ROLE_ADMIN},
		"other pharmacy": {UserID: "staff-2", PharmacyID: "ph-2", Role: authv1.Role_ROLE_PHARMACY_STAFF},
		"patient":        {UserID: "user-1", Role: authv1.Role_ROLE_PATIENT},
	} {
		t.Run(name, func(t *testing.T) {
			fake := &fakeStockSyncClient{}
			req := uploadRequest(t, "file", "stock.csv", csv, identity)

			rec := httptest.NewRecorder()
			stockSyncHandler(fake, nil)(rec, req)

			if rec.Code != http.StatusForbidden {
				t.Fatalf("got %d, want 403", rec.Code)
			}
			if fake.upload != nil {
				t.Fatal("downstream call should not be made")
			}
		})
	}
}

func jobRequest(jobID string) *http.Request {
	req := withIdentity(httptest.NewRequest(http.MethodGet, "/api/v1/pharmacies/ph-1/stock-sync/jobs/"+jobID, nil), staffOfPharmacy1)
	req.SetPathValue("pharmacyId", "ph-1")
	req.SetPathValue("jobId", jobID)
	return req
}

func TestStockSyncGetJobReturnsOwnJob(t *testing.T) {
	fake := &fakeStockSyncClient{job: partialJob("ph-1")}

	rec := httptest.NewRecorder()
	stockSyncHandler(fake, nil)(rec, jobRequest("job-1"))

	if rec.Code != http.StatusOK {
		t.Fatalf("got %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if fake.get.GetJobId() != "job-1" || !strings.Contains(rec.Body.String(), "unknown product") {
		t.Fatalf("unexpected job response: %s", rec.Body.String())
	}
}

func TestStockSyncGetJobOfAnotherPharmacyIsNotFound(t *testing.T) {
	fake := &fakeStockSyncClient{job: partialJob("ph-2")}

	rec := httptest.NewRecorder()
	stockSyncHandler(fake, testLogger())(rec, jobRequest("job-1"))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("got %d, want 404", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "unknown product") {
		t.Fatalf("another pharmacy's report leaked: %s", rec.Body.String())
	}
}

func TestStockSyncListJobsUsesPathPharmacy(t *testing.T) {
	fake := &fakeStockSyncClient{job: partialJob("ph-1")}
	req := withIdentity(httptest.NewRequest(http.MethodGet, "/api/v1/pharmacies/ph-1/stock-sync/jobs?pageSize=10", nil), staffOfPharmacy1)
	req.SetPathValue("pharmacyId", "ph-1")

	rec := httptest.NewRecorder()
	stockSyncHandler(fake, nil)(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("got %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if fake.list.GetPharmacyId() != "ph-1" || fake.list.GetPage().GetPageSize() != 10 {
		t.Fatalf("unexpected downstream request: %v", fake.list)
	}
}

func TestRouterServesDashboardStockSyncRoutes(t *testing.T) {
	fake := &fakeStockSyncClient{job: partialJob("ph-1")}
	router := pharmacyRouter(t, WithInventory(&fakeInventoryClient{}), WithStockSync(fake))

	body, contentType := multipartBody(t, "file", "stock.csv", []byte("product_id,quantity\np-1,1\n"))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/pharmacies/ph-1/stock-sync/jobs", body)
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Authorization", "Bearer token")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("upload: got %d, want 201: %s", rec.Code, rec.Body.String())
	}

	if rec := serve(router, http.MethodGet, "/api/v1/pharmacies/ph-1/stock-sync/jobs/job-1", ""); rec.Code != http.StatusOK {
		t.Fatalf("get job: got %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if rec := serve(router, http.MethodGet, "/api/v1/pharmacies/ph-1/stock-sync/jobs", ""); rec.Code != http.StatusOK {
		t.Fatalf("list jobs: got %d, want 200: %s", rec.Code, rec.Body.String())
	}
}
