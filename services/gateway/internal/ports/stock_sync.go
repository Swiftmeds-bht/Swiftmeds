package ports

import (
	"context"

	stocksyncv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/stock-sync/v1"
)

// StockSyncClient is the Stock Sync service, which imports a pharmacy's stock CSV.
type StockSyncClient interface {
	UploadStockCsv(context.Context, *stocksyncv1.UploadStockCsvRequest) (*stocksyncv1.UploadStockCsvResponse, error)
	GetSyncJob(context.Context, *stocksyncv1.GetSyncJobRequest) (*stocksyncv1.GetSyncJobResponse, error)
	ListSyncJobs(context.Context, *stocksyncv1.ListSyncJobsRequest) (*stocksyncv1.ListSyncJobsResponse, error)
}
