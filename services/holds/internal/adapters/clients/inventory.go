// Package clients holds Holds' outbound gRPC clients. Each one only translates
// between proto messages and the ports the app layer needs.
package clients

import (
	"context"
	"fmt"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	inventoryv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/inventory/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/holds/internal/domain"
	"github.com/Swiftmeds-bht/Swiftmeds/services/holds/internal/ports"
)

var _ ports.Inventory = Inventory{}

// Inventory calls the Inventory service to release or commit a hold's reserved stock.
type Inventory struct {
	c inventoryv1.InventoryServiceClient
}

func NewInventory(cc grpc.ClientConnInterface) Inventory {
	return Inventory{c: inventoryv1.NewInventoryServiceClient(cc)}
}

// CheckStocked asks Inventory for the pharmacy's stock row of the product.
// NOT_FOUND means one of the IDs is unknown or the pharmacy does not sell the
// medicine; any other failure means Inventory could not answer.
func (i Inventory) CheckStocked(ctx context.Context, pharmacyID, productID string) error {
	_, err := i.c.GetStock(ctx, &inventoryv1.GetStockRequest{PharmacyId: pharmacyID, ProductId: productID})
	if status.Code(err) == codes.NotFound {
		return domain.ErrNotStocked
	}
	return unavailable("get stock", err)
}

// Release gives the hold's reserved units back to available stock.
func (i Inventory) Release(ctx context.Context, holdID string) error {
	_, err := i.c.ReleaseReservation(ctx, &inventoryv1.ReleaseReservationRequest{HoldId: holdID})
	return unavailable("release", err)
}

// Commit makes the hold's reserved units leave stock for good.
func (i Inventory) Commit(ctx context.Context, holdID string) error {
	_, err := i.c.CommitReservation(ctx, &inventoryv1.CommitReservationRequest{HoldId: holdID})
	return unavailable("commit", err)
}

// unavailable turns any failure into domain.ErrInventoryUnavailable. Inventory
// reports "no such reservation" and "wrong state" as plain INTERNAL errors, so
// they cannot be told from an outage. Treating all of them as "try again" is
// safe: the hold is left unchanged and both calls can be repeated.
func unavailable(operation string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%w: inventory %s: %w", domain.ErrInventoryUnavailable, operation, err)
}
