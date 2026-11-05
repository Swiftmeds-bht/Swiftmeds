package domain

import "time"

type JobStatus int32

const (
	JobStatusProcessing JobStatus = iota + 1
	JobStatusCompleted
	JobStatusPartial
	JobStatusFailed
)

type SyncJob struct {
	JobID            string
	PharmacyID       string
	UploadedByUserID string
	Filename         string
	Status           JobStatus
	RowsTotal        int32
	RowsApplied      int32
	RowsRejected     int32
	RowErrors        []RowError
	CreatedAt        time.Time
	FinishedAt       time.Time
}

type StockUpdate struct {
	ProductID      string
	QuantityOnHand int32
}

type StockUpdateError struct {
	ProductID string
	Message   string
}

type BulkStockResult struct {
	UpdatedCount int32
	Errors       []StockUpdateError
}

type PriceUpdate struct {
	ProductID     string
	PriceChhertum int64
}
