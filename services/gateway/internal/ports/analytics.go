package ports

import (
	"context"

	analyticsv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/analytics/v1"
)

type AnalyticsClient interface {
	GetUnmetDemand(context.Context, *analyticsv1.GetUnmetDemandRequest) (*analyticsv1.GetUnmetDemandResponse, error)
}
