package clients

import (
	"context"
	"errors"
	"testing"

	catalogv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/catalog/v1"
	inventoryv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/inventory/v1"
	pricingv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/pricing/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/stock-sync/internal/domain"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

// fakeConn answers every unary call by running fill on the reply (or returning
// err) and records the request.
type fakeConn struct {
	method  string
	request proto.Message
	fill    func(reply any)
	err     error
}

func (c *fakeConn) Invoke(_ context.Context, method string, args, reply any, _ ...grpc.CallOption) error {
	c.method = method
	c.request = args.(proto.Message)
	if c.err != nil {
		return c.err
	}
	if c.fill != nil {
		c.fill(reply)
	}
	return nil
}

func (c *fakeConn) NewStream(context.Context, *grpc.StreamDesc, string, ...grpc.CallOption) (grpc.ClientStream, error) {
	return nil, errors.New("streams are not used")
}

func TestCatalogClientReturnsTheIDsCatalogKnows(t *testing.T) {
	conn := &fakeConn{fill: func(reply any) {
		reply.(*catalogv1.BatchGetProductsResponse).Products = []*catalogv1.Product{
			{ProductId: "med-1"}, nil, {ProductId: "med-3"},
		}
	}}

	got, err := NewCatalogClient(conn).GetExistingProductIDs(context.Background(), []string{"med-1", "med-2", "med-3"})
	if err != nil {
		t.Fatalf("GetExistingProductIDs: %v", err)
	}
	if len(got) != 2 || got[0] != "med-1" || got[1] != "med-3" {
		t.Fatalf("IDs = %v, want [med-1 med-3] (nil entries skipped)", got)
	}
	if conn.method != catalogv1.CatalogService_BatchGetProducts_FullMethodName {
		t.Fatalf("method = %s", conn.method)
	}
	req := conn.request.(*catalogv1.BatchGetProductsRequest)
	if len(req.GetProductIds()) != 3 {
		t.Fatalf("request IDs = %v, want all three", req.GetProductIds())
	}
}

func TestInventoryClientSendsUpdatesAndMapsRowErrors(t *testing.T) {
	conn := &fakeConn{fill: func(reply any) {
		r := reply.(*inventoryv1.BulkSetStockResponse)
		r.UpdatedCount = 1
		r.Errors = []*inventoryv1.StockUpdateError{{ProductId: "med-2", Message: "below reserved"}, nil}
	}}

	got, err := NewInventoryClient(conn).BulkSetStock(context.Background(), "ph-1",
		[]domain.StockUpdate{{ProductID: "med-1", QuantityOnHand: 4}, {ProductID: "med-2", QuantityOnHand: 0}}, "u-1")
	if err != nil {
		t.Fatalf("BulkSetStock: %v", err)
	}
	if got.UpdatedCount != 1 || len(got.Errors) != 1 ||
		got.Errors[0] != (domain.StockUpdateError{ProductID: "med-2", Message: "below reserved"}) {
		t.Fatalf("result = %+v", got)
	}
	req := conn.request.(*inventoryv1.BulkSetStockRequest)
	if conn.method != inventoryv1.InventoryService_BulkSetStock_FullMethodName ||
		req.GetPharmacyId() != "ph-1" || req.GetUpdatedByUserId() != "u-1" || len(req.GetUpdates()) != 2 ||
		req.GetUpdates()[0].GetProductId() != "med-1" || req.GetUpdates()[0].GetQuantityOnHand() != 4 ||
		req.GetUpdates()[1].GetQuantityOnHand() != 0 {
		t.Fatalf("request = %v to %s", req, conn.method)
	}
}

func TestPricingClientSendsPrices(t *testing.T) {
	conn := &fakeConn{}

	err := NewPricingClient(conn).SetPrices(context.Background(), "ph-1",
		[]domain.PriceUpdate{{ProductID: "med-1", PriceChhertum: 2500}})
	if err != nil {
		t.Fatalf("SetPrices: %v", err)
	}
	req := conn.request.(*pricingv1.SetPricesRequest)
	if conn.method != pricingv1.PricingService_SetPrices_FullMethodName || req.GetPharmacyId() != "ph-1" ||
		len(req.GetUpdates()) != 1 || req.GetUpdates()[0].GetProductId() != "med-1" ||
		req.GetUpdates()[0].GetPriceChhertum() != 2500 {
		t.Fatalf("request = %v to %s", req, conn.method)
	}
}

// A failed call keeps the gRPC status, so the handler can pass it on.
func TestClientsWrapCallErrors(t *testing.T) {
	callErr := status.Error(codes.Unavailable, "inventory is down")
	ctx := context.Background()

	_, err1 := NewCatalogClient(&fakeConn{err: callErr}).GetExistingProductIDs(ctx, []string{"med-1"})
	_, err2 := NewInventoryClient(&fakeConn{err: callErr}).BulkSetStock(ctx, "ph-1", nil, "u-1")
	err3 := NewPricingClient(&fakeConn{err: callErr}).SetPrices(ctx, "ph-1", nil)

	for i, err := range []error{err1, err2, err3} {
		if !errors.Is(err, callErr) {
			t.Errorf("client %d: error = %v, want it to wrap the call error", i+1, err)
		}
		if s, ok := status.FromError(err); !ok || s.Code() != codes.Unavailable {
			t.Errorf("client %d: status = %v, want Unavailable", i+1, s)
		}
	}
}
