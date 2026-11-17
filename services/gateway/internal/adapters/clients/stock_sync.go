package clients

import (
	"context"

	stocksyncv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/stock-sync/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/gateway/internal/ports"
)

var _ ports.StockSyncClient = (*StockSyncClient)(nil)

type StockSyncClient struct {
	client stocksyncv1.StockSyncServiceClient
}

func NewStockSyncClient(client stocksyncv1.StockSyncServiceClient) *StockSyncClient {
	return &StockSyncClient{client: client}
}

func (c *StockSyncClient) UploadStockCsv(ctx context.Context, req *stocksyncv1.UploadStockCsvRequest) (*stocksyncv1.UploadStockCsvResponse, error) {
	return c.client.UploadStockCsv(ctx, req)
}

func (c *StockSyncClient) GetSyncJob(ctx context.Context, req *stocksyncv1.GetSyncJobRequest) (*stocksyncv1.GetSyncJobResponse, error) {
	return c.client.GetSyncJob(ctx, req)
}

func (c *StockSyncClient) ListSyncJobs(ctx context.Context, req *stocksyncv1.ListSyncJobsRequest) (*stocksyncv1.ListSyncJobsResponse, error) {
	return c.client.ListSyncJobs(ctx, req)
}
