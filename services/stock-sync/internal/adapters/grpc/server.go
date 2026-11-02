package grpc

import (
	"context"
	"errors"
	"fmt"

	commonv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/common/v1"
	stocksyncv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/stock-sync/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/stock-sync/internal/app"
	"github.com/Swiftmeds-bht/Swiftmeds/services/stock-sync/internal/domain"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type Handler struct {
	stocksyncv1.UnimplementedStockSyncServiceServer
	service *app.Service
}

func NewHandler(service *app.Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) UploadStockCsv(
	ctx context.Context,
	request *stocksyncv1.UploadStockCsvRequest,
) (*stocksyncv1.UploadStockCsvResponse, error) {
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}

	job, err := h.service.UploadStockCSV(ctx, app.UploadCSVRequest{
		PharmacyID:       request.GetPharmacyId(),
		UploadedByUserID: request.GetUploadedByUserId(),
		Filename:         request.GetFilename(),
		Content:          request.GetCsvContent(),
	})
	if err != nil {
		return nil, toStatusError(err)
	}
	return &stocksyncv1.UploadStockCsvResponse{Job: toProtoJob(job)}, nil
}

func (h *Handler) GetSyncJob(
	ctx context.Context,
	request *stocksyncv1.GetSyncJobRequest,
) (*stocksyncv1.GetSyncJobResponse, error) {
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}

	job, err := h.service.GetSyncJob(ctx, request.GetJobId())
	if err != nil {
		return nil, toStatusError(err)
	}
	return &stocksyncv1.GetSyncJobResponse{Job: toProtoJob(job)}, nil
}

func (h *Handler) ListSyncJobs(
	ctx context.Context,
	request *stocksyncv1.ListSyncJobsRequest,
) (*stocksyncv1.ListSyncJobsResponse, error) {
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}

	page := request.GetPage()
	pageSize := int32(0)
	pageToken := ""
	if page != nil {
		pageSize = page.GetPageSize()
		pageToken = page.GetPageToken()
	}

	jobs, nextPageToken, err := h.service.ListSyncJobs(ctx, app.ListJobsRequest{
		PharmacyID: request.GetPharmacyId(),
		PageSize:   pageSize,
		PageToken:  pageToken,
	})
	if err != nil {
		return nil, toStatusError(err)
	}

	response := &stocksyncv1.ListSyncJobsResponse{
		Jobs: make([]*stocksyncv1.SyncJob, 0, len(jobs)),
		Page: &commonv1.PageResponse{NextPageToken: nextPageToken},
	}
	for _, job := range jobs {
		response.Jobs = append(response.Jobs, toProtoJob(job))
	}
	return response, nil
}

func toProtoJob(job domain.SyncJob) *stocksyncv1.SyncJob {
	rowErrors := make([]*stocksyncv1.RowError, 0, len(job.RowErrors))
	for _, rowError := range job.RowErrors {
		message := rowError.Message
		if rowError.ProductID != "" {
			message = fmt.Sprintf("%s: %s", rowError.ProductID, message)
		}
		rowErrors = append(rowErrors, &stocksyncv1.RowError{
			RowNumber: rowError.RowNumber,
			Message:   message,
		})
	}

	return &stocksyncv1.SyncJob{
		JobId:            job.JobID,
		PharmacyId:       job.PharmacyID,
		UploadedByUserId: job.UploadedByUserID,
		Filename:         job.Filename,
		Status:           toProtoStatus(job.Status),
		RowsTotal:        job.RowsTotal,
		RowsApplied:      job.RowsApplied,
		RowsRejected:     job.RowsRejected,
		RowErrors:        rowErrors,
		CreatedAt:        timestamppb.New(job.CreatedAt),
		FinishedAt:       timestamppb.New(job.FinishedAt),
	}
}

func toProtoStatus(status domain.JobStatus) stocksyncv1.SyncJobStatus {
	switch status {
	case domain.JobStatusProcessing:
		return stocksyncv1.SyncJobStatus_SYNC_JOB_STATUS_PROCESSING
	case domain.JobStatusCompleted:
		return stocksyncv1.SyncJobStatus_SYNC_JOB_STATUS_COMPLETED
	case domain.JobStatusPartial:
		return stocksyncv1.SyncJobStatus_SYNC_JOB_STATUS_PARTIAL
	case domain.JobStatusFailed:
		return stocksyncv1.SyncJobStatus_SYNC_JOB_STATUS_FAILED
	default:
		return stocksyncv1.SyncJobStatus_SYNC_JOB_STATUS_UNSPECIFIED
	}
}

func toStatusError(err error) error {
	switch {
	case errors.Is(err, domain.ErrMissingPharmacyID),
		errors.Is(err, domain.ErrMissingUploaderID),
		errors.Is(err, domain.ErrMissingJobID),
		errors.Is(err, domain.ErrInvalidPageToken):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, domain.ErrJobNotFound):
		return status.Error(codes.NotFound, err.Error())
	default:
		if upstream, ok := status.FromError(err); ok {
			return upstream.Err()
		}
		return status.Error(codes.Internal, "stock sync request failed")
	}
}

var _ stocksyncv1.StockSyncServiceServer = (*Handler)(nil)
