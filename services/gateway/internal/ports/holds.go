package ports

import (
	"context"

	holdsv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/holds/v1"
)

type HoldsClient interface {
	CreateHold(context.Context, *holdsv1.CreateHoldRequest) (*holdsv1.CreateHoldResponse, error)
	GetHold(context.Context, *holdsv1.GetHoldRequest) (*holdsv1.GetHoldResponse, error)
	ListHolds(context.Context, *holdsv1.ListHoldsRequest) (*holdsv1.ListHoldsResponse, error)
	CancelHold(context.Context, *holdsv1.CancelHoldRequest) (*holdsv1.CancelHoldResponse, error)
	CollectHold(context.Context, *holdsv1.CollectHoldRequest) (*holdsv1.CollectHoldResponse, error)
}
