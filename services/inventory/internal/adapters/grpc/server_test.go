package grpc

import (
	"context"
	"errors"
	"testing"
	"time"

	commonv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/common/v1"
	inventoryv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/inventory/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/inventory/internal/app"
	"github.com/Swiftmeds-bht/Swiftmeds/services/inventory/internal/domain"
	"github.com/Swiftmeds-bht/Swiftmeds/services/inventory/internal/ports"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var errAdapterTest = errors.New("adapter test failure")

type adapterRepo struct {
	getFn     func(context.Context, ports.StockKey) (domain.Stock, bool, error)
	getManyFn func(context.Context, []string, []string, bool) ([]domain.Stock, error)
	listFn    func(context.Context, string, int32, string) ([]domain.Stock, string, error)
	setFn     func(context.Context, domain.Stock, string) (int32, domain.Stock, error)
	bulkFn    func(context.Context, string, []ports.StockUpdate, string) ([]ports.StockChange, []ports.StockUpdateError, error)
	reserveFn func(context.Context, string, ports.StockKey, int32) (domain.Stock, bool, error)
	commitFn  func(context.Context, string) (domain.Stock, error)
	releaseFn func(context.Context, string) (domain.Stock, error)
}

func (r *adapterRepo) Get(ctx context.Context, key ports.StockKey) (domain.Stock, bool, error) {
	if r.getFn != nil {
		return r.getFn(ctx, key)
	}
	return domain.Stock{}, false, nil
}
func (r *adapterRepo) GetMany(ctx context.Context, pharmacies, products []string, onlyAvailable bool) ([]domain.Stock, error) {
	if r.getManyFn != nil {
		return r.getManyFn(ctx, pharmacies, products, onlyAvailable)
	}
	return nil, nil
}
func (r *adapterRepo) ListByPharmacy(ctx context.Context, pharmacy string, size int32, token string) ([]domain.Stock, string, error) {
	if r.listFn != nil {
		return r.listFn(ctx, pharmacy, size, token)
	}
	return nil, "", nil
}
func (r *adapterRepo) Set(ctx context.Context, stock domain.Stock, user string) (int32, domain.Stock, error) {
	if r.setFn != nil {
		return r.setFn(ctx, stock, user)
	}
	return 0, domain.Stock{}, nil
}
func (r *adapterRepo) BulkSet(ctx context.Context, pharmacy string, updates []ports.StockUpdate, user string) ([]ports.StockChange, []ports.StockUpdateError, error) {
	if r.bulkFn != nil {
		return r.bulkFn(ctx, pharmacy, updates, user)
	}
	return nil, nil, nil
}
func (r *adapterRepo) Reserve(ctx context.Context, hold string, key ports.StockKey, qty int32) (domain.Stock, bool, error) {
	if r.reserveFn != nil {
		return r.reserveFn(ctx, hold, key, qty)
	}
	return domain.Stock{}, false, nil
}
func (r *adapterRepo) CommitReservation(ctx context.Context, hold string) (domain.Stock, error) {
	if r.commitFn != nil {
		return r.commitFn(ctx, hold)
	}
	return domain.Stock{}, nil
}
func (r *adapterRepo) ReleaseReservation(ctx context.Context, hold string) (domain.Stock, error) {
	if r.releaseFn != nil {
		return r.releaseFn(ctx, hold)
	}
	return domain.Stock{}, nil
}

type adapterPublisher struct{}

func (adapterPublisher) PublishStockReserved(context.Context, string, string, string, int32) error {
	return nil
}
func (adapterPublisher) PublishStockUnavailable(context.Context, string, string, string, int32, int32) error {
	return nil
}
func (adapterPublisher) PublishStockIncreased(context.Context, string, string, int32, int32) error {
	return nil
}

func stockForAdapterTest(t *testing.T, pharmacy, product string, onHand, reserved int32) domain.Stock {
	t.Helper()
	confirmed := time.Date(2026, time.October, 29, 12, 0, 0, 0, time.UTC)
	stock, err := domain.NewStock(pharmacy, product, onHand, reserved, confirmed)
	if err != nil {
		t.Fatalf("create stock: %v", err)
	}
	return stock
}

func newAdapterServer(t *testing.T, repo *adapterRepo) *Server {
	t.Helper()
	service, err := app.NewService(repo, adapterPublisher{}, time.Now)
	if err != nil {
		t.Fatalf("create app service: %v", err)
	}
	server, err := NewServer(service)
	if err != nil {
		t.Fatalf("create gRPC server: %v", err)
	}
	return server
}

func TestNewServerRejectsNilService(t *testing.T) {
	if _, err := NewServer(nil); err == nil {
		t.Fatal("NewServer(nil) returned no error")
	}
}

func TestGetStockSuccessAndNotFound(t *testing.T) {
	want := stockForAdapterTest(t, "pharmacy-1", "product-1", 8, 2)
	repo := &adapterRepo{getFn: func(_ context.Context, key ports.StockKey) (domain.Stock, bool, error) {
		if key.PharmacyID != "pharmacy-1" || key.ProductID != "product-1" {
			t.Fatalf("Get key = %+v", key)
		}
		return want, true, nil
	}}
	server := newAdapterServer(t, repo)
	response, err := server.GetStock(context.Background(), &inventoryv1.GetStockRequest{PharmacyId: "pharmacy-1", ProductId: "product-1"})
	if err != nil {
		t.Fatalf("GetStock: %v", err)
	}
	if response.GetStock().GetQuantityAvailable() != 6 || response.GetStock().GetPharmacyId() != "pharmacy-1" {
		t.Fatalf("stock response = %+v", response.GetStock())
	}
	if !response.GetStock().GetConfirmedAt().AsTime().Equal(want.ConfirmedAt) {
		t.Fatalf("confirmed_at = %v, want %v", response.GetStock().GetConfirmedAt().AsTime(), want.ConfirmedAt)
	}
	repo.getFn = func(context.Context, ports.StockKey) (domain.Stock, bool, error) { return domain.Stock{}, false, nil }
	_, err = server.GetStock(context.Background(), &inventoryv1.GetStockRequest{PharmacyId: "pharmacy-1", ProductId: "missing"})
	if status.Code(err) != codes.NotFound {
		t.Fatalf("GetStock status = %v, want NotFound", status.Code(err))
	}
}

func TestBatchGetStockAndListPharmacyStock(t *testing.T) {
	stock := stockForAdapterTest(t, "pharmacy-1", "product-1", 7, 1)
	repo := &adapterRepo{
		getManyFn: func(_ context.Context, pharmacies, products []string, available bool) ([]domain.Stock, error) {
			if len(pharmacies) != 1 || pharmacies[0] != "pharmacy-1" || len(products) != 1 || products[0] != "product-1" || !available {
				t.Fatalf("GetMany args = %v %v %v", pharmacies, products, available)
			}
			return []domain.Stock{stock}, nil
		},
		listFn: func(_ context.Context, pharmacy string, size int32, token string) ([]domain.Stock, string, error) {
			if pharmacy != "pharmacy-1" || size != 5 || token != "cursor" {
				t.Fatalf("List args = %q %d %q", pharmacy, size, token)
			}
			return []domain.Stock{stock}, "next", nil
		},
	}
	server := newAdapterServer(t, repo)
	batch, err := server.BatchGetStock(context.Background(), &inventoryv1.BatchGetStockRequest{PharmacyIds: []string{"pharmacy-1"}, ProductIds: []string{"product-1"}, OnlyAvailable: true})
	if err != nil || len(batch.GetStock()) != 1 || batch.GetStock()[0].GetQuantityAvailable() != 6 {
		t.Fatalf("BatchGetStock = (%+v, %v)", batch, err)
	}
	list, err := server.ListPharmacyStock(context.Background(), &inventoryv1.ListPharmacyStockRequest{PharmacyId: "pharmacy-1", Page: &commonv1.PageRequest{PageSize: 5, PageToken: "cursor"}})
	if err != nil || len(list.GetStock()) != 1 || list.GetPage().GetNextPageToken() != "next" {
		t.Fatalf("ListPharmacyStock = (%+v, %v)", list, err)
	}
}

func TestSetStockAndBulkSetStockResponses(t *testing.T) {
	stock := stockForAdapterTest(t, "pharmacy-1", "product-1", 6, 0)
	repo := &adapterRepo{
		setFn: func(_ context.Context, requested domain.Stock, user string) (int32, domain.Stock, error) {
			if requested.PharmacyID != "pharmacy-1" || requested.ProductID != "product-1" || user != "user-1" {
				t.Fatalf("Set args = %+v, %q", requested, user)
			}
			return 0, stock, nil
		},
		bulkFn: func(_ context.Context, pharmacy string, updates []ports.StockUpdate, user string) ([]ports.StockChange, []ports.StockUpdateError, error) {
			if pharmacy != "pharmacy-1" || user != "user-1" || len(updates) != 2 || updates[0].ProductID != "product-1" || updates[1] != (ports.StockUpdate{}) {
				t.Fatalf("BulkSet args = %q %+v %q", pharmacy, updates, user)
			}
			return []ports.StockChange{{Stock: stock}}, []ports.StockUpdateError{{ProductID: "bad", Message: "invalid quantity"}}, nil
		},
	}
	server := newAdapterServer(t, repo)
	set, err := server.SetStock(context.Background(), &inventoryv1.SetStockRequest{PharmacyId: "pharmacy-1", ProductId: "product-1", QuantityOnHand: 6, UpdatedByUserId: "user-1"})
	if err != nil || set.GetStock().GetQuantityOnHand() != 6 {
		t.Fatalf("SetStock = (%+v, %v)", set, err)
	}
	bulk, err := server.BulkSetStock(context.Background(), &inventoryv1.BulkSetStockRequest{PharmacyId: "pharmacy-1", UpdatedByUserId: "user-1", Updates: []*inventoryv1.StockUpdate{{ProductId: "product-1", QuantityOnHand: 6}, nil}})
	if err != nil || bulk.GetUpdatedCount() != 1 || len(bulk.GetErrors()) != 1 || bulk.GetErrors()[0].GetProductId() != "bad" {
		t.Fatalf("BulkSetStock = (%+v, %v)", bulk, err)
	}
}

func TestReservationRPCResponses(t *testing.T) {
	stock := stockForAdapterTest(t, "pharmacy-1", "product-1", 6, 1)
	repo := &adapterRepo{
		commitFn: func(_ context.Context, hold string) (domain.Stock, error) {
			if hold != "hold-1" {
				t.Errorf("commit hold = %q", hold)
			}
			return stock, nil
		},
		releaseFn: func(_ context.Context, hold string) (domain.Stock, error) {
			if hold != "hold-2" {
				t.Errorf("release hold = %q", hold)
			}
			return stock, nil
		},
	}
	server := newAdapterServer(t, repo)
	committed, err := server.CommitReservation(context.Background(), &inventoryv1.CommitReservationRequest{HoldId: "hold-1"})
	if err != nil || committed.GetStock().GetQuantityReserved() != 1 {
		t.Fatalf("CommitReservation = (%+v, %v)", committed, err)
	}
	released, err := server.ReleaseReservation(context.Background(), &inventoryv1.ReleaseReservationRequest{HoldId: "hold-2"})
	if err != nil || released.GetStock().GetQuantityOnHand() != 6 {
		t.Fatalf("ReleaseReservation = (%+v, %v)", released, err)
	}
}

func TestStatusErrorMappings(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want codes.Code
	}{
		{"not found", app.ErrStockNotFound, codes.NotFound},
		{"invalid key", domain.ErrInvalidStockKey, codes.InvalidArgument},
		{"negative stock", domain.ErrNegativeStock, codes.InvalidArgument},
		{"invalid reservation quantity", domain.ErrInvalidReservationQuantity, codes.InvalidArgument},
		{"insufficient available", domain.ErrInsufficientAvailableStock, codes.ResourceExhausted},
		{"on hand below reserved", domain.ErrOnHandBelowReservedQuantity, codes.FailedPrecondition},
		{"insufficient reserved", domain.ErrInsufficientReservedStock, codes.FailedPrecondition},
		{"internal", errAdapterTest, codes.Internal},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := status.Code(toStatusError(tt.err)); got != tt.want {
				t.Fatalf("status = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRPCMethodsReturnInternalOnRepositoryErrors(t *testing.T) {
	repo := &adapterRepo{
		getManyFn: func(context.Context, []string, []string, bool) ([]domain.Stock, error) { return nil, errAdapterTest },
		listFn: func(context.Context, string, int32, string) ([]domain.Stock, string, error) {
			return nil, "", errAdapterTest
		},
		setFn: func(context.Context, domain.Stock, string) (int32, domain.Stock, error) {
			return 0, domain.Stock{}, errAdapterTest
		},
		bulkFn: func(context.Context, string, []ports.StockUpdate, string) ([]ports.StockChange, []ports.StockUpdateError, error) {
			return nil, nil, errAdapterTest
		},
		commitFn:  func(context.Context, string) (domain.Stock, error) { return domain.Stock{}, errAdapterTest },
		releaseFn: func(context.Context, string) (domain.Stock, error) { return domain.Stock{}, errAdapterTest },
	}
	server := newAdapterServer(t, repo)
	checks := []struct {
		name string
		call func() error
	}{
		{"batch", func() error {
			_, err := server.BatchGetStock(context.Background(), &inventoryv1.BatchGetStockRequest{})
			return err
		}},
		{"list", func() error {
			_, err := server.ListPharmacyStock(context.Background(), &inventoryv1.ListPharmacyStockRequest{})
			return err
		}},
		{"set", func() error {
			_, err := server.SetStock(context.Background(), &inventoryv1.SetStockRequest{PharmacyId: "pharmacy-1", ProductId: "product-1", QuantityOnHand: 2})
			return err
		}},
		{"bulk", func() error {
			_, err := server.BulkSetStock(context.Background(), &inventoryv1.BulkSetStockRequest{})
			return err
		}},
		{"commit", func() error {
			_, err := server.CommitReservation(context.Background(), &inventoryv1.CommitReservationRequest{})
			return err
		}},
		{"release", func() error {
			_, err := server.ReleaseReservation(context.Background(), &inventoryv1.ReleaseReservationRequest{})
			return err
		}},
	}
	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			if got := status.Code(check.call()); got != codes.Internal {
				t.Fatalf("status = %v, want Internal", got)
			}
		})
	}
}
