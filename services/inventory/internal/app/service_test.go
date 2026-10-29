package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Swiftmeds-bht/Swiftmeds/services/inventory/internal/domain"
	"github.com/Swiftmeds-bht/Swiftmeds/services/inventory/internal/ports"
)

type repositoryFake struct {
	getFn     func(context.Context, ports.StockKey) (domain.Stock, bool, error)
	setFn     func(context.Context, domain.Stock, string) (int32, domain.Stock, error)
	bulkSetFn func(context.Context, string, []ports.StockUpdate, string) ([]ports.StockChange, []ports.StockUpdateError, error)
	reserveFn func(context.Context, string, ports.StockKey, int32) (domain.Stock, bool, error)
}

func (f *repositoryFake) Get(ctx context.Context, key ports.StockKey) (domain.Stock, bool, error) {
	if f.getFn != nil {
		return f.getFn(ctx, key)
	}
	return domain.Stock{}, false, nil
}

func (f *repositoryFake) GetMany(
	context.Context,
	[]string,
	[]string,
	bool,
) ([]domain.Stock, error) {
	return nil, nil
}

func (f *repositoryFake) ListByPharmacy(
	context.Context,
	string,
	int32,
	string,
) ([]domain.Stock, string, error) {
	return nil, "", nil
}

func (f *repositoryFake) Set(
	ctx context.Context,
	stock domain.Stock,
	updatedByUserID string,
) (int32, domain.Stock, error) {
	if f.setFn != nil {
		return f.setFn(ctx, stock, updatedByUserID)
	}
	return 0, domain.Stock{}, nil
}

func (f *repositoryFake) BulkSet(
	ctx context.Context,
	pharmacyID string,
	updates []ports.StockUpdate,
	updatedByUserID string,
) ([]ports.StockChange, []ports.StockUpdateError, error) {
	if f.bulkSetFn != nil {
		return f.bulkSetFn(ctx, pharmacyID, updates, updatedByUserID)
	}
	return nil, nil, nil
}

func (f *repositoryFake) Reserve(
	ctx context.Context,
	holdID string,
	key ports.StockKey,
	quantity int32,
) (domain.Stock, bool, error) {
	if f.reserveFn != nil {
		return f.reserveFn(ctx, holdID, key, quantity)
	}
	return domain.Stock{}, false, nil
}

func (f *repositoryFake) CommitReservation(
	context.Context,
	string,
) (domain.Stock, error) {
	return domain.Stock{}, nil
}

func (f *repositoryFake) ReleaseReservation(
	context.Context,
	string,
) (domain.Stock, error) {
	return domain.Stock{}, nil
}

type reservedEvent struct {
	holdID     string
	pharmacyID string
	productID  string
	quantity   int32
}

type unavailableEvent struct {
	holdID            string
	pharmacyID        string
	productID         string
	requestedQuantity int32
	availableQuantity int32
}

type increasedEvent struct {
	pharmacyID        string
	productID         string
	previousAvailable int32
	newAvailable      int32
}

type publisherFake struct {
	reserved    []reservedEvent
	unavailable []unavailableEvent
	increased   []increasedEvent
	err         error
}

func (f *publisherFake) PublishStockReserved(
	_ context.Context,
	holdID, pharmacyID, productID string,
	quantity int32,
) error {
	f.reserved = append(f.reserved, reservedEvent{
		holdID: holdID, pharmacyID: pharmacyID, productID: productID, quantity: quantity,
	})
	return f.err
}

func (f *publisherFake) PublishStockUnavailable(
	_ context.Context,
	holdID, pharmacyID, productID string,
	requestedQuantity, availableQuantity int32,
) error {
	f.unavailable = append(f.unavailable, unavailableEvent{
		holdID: holdID, pharmacyID: pharmacyID, productID: productID,
		requestedQuantity: requestedQuantity, availableQuantity: availableQuantity,
	})
	return f.err
}

func (f *publisherFake) PublishStockIncreased(
	_ context.Context,
	pharmacyID, productID string,
	previousAvailable, newAvailable int32,
) error {
	f.increased = append(f.increased, increasedEvent{
		pharmacyID: pharmacyID, productID: productID,
		previousAvailable: previousAvailable, newAvailable: newAvailable,
	})
	return f.err
}

func testStock(t *testing.T, pharmacyID, productID string, onHand, reserved int32) domain.Stock {
	t.Helper()
	stock, err := domain.NewStock(pharmacyID, productID, onHand, reserved, time.Time{})
	if err != nil {
		t.Fatalf("create test stock: %v", err)
	}
	return stock
}

func newTestService(t *testing.T, repo ports.StockRepository, publisher ports.EventPublisher) *Service {
	t.Helper()
	service, err := NewService(repo, publisher, func() time.Time {
		return time.Date(2026, time.October, 29, 12, 0, 0, 0, time.UTC)
	})
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	return service
}

func TestGetStockReturnsNotFound(t *testing.T) {
	repo := &repositoryFake{
		getFn: func(context.Context, ports.StockKey) (domain.Stock, bool, error) {
			return domain.Stock{}, false, nil
		},
	}
	service := newTestService(t, repo, &publisherFake{})

	_, err := service.GetStock(context.Background(), "pharmacy-1", "product-1")
	if !errors.Is(err, ErrStockNotFound) {
		t.Fatalf("GetStock error = %v, want ErrStockNotFound", err)
	}
}

func TestSetStockPublishesWhenAvailableQuantityIncreases(t *testing.T) {
	now := time.Date(2026, time.October, 29, 12, 0, 0, 0, time.UTC)
	publisher := &publisherFake{}
	repo := &repositoryFake{
		setFn: func(_ context.Context, requested domain.Stock, updatedBy string) (int32, domain.Stock, error) {
			if requested.ConfirmedAt != now {
				t.Errorf("ConfirmedAt = %v, want %v", requested.ConfirmedAt, now)
			}
			if updatedBy != "user-1" {
				t.Errorf("updatedBy = %q, want user-1", updatedBy)
			}
			return 3, testStock(t, requested.PharmacyID, requested.ProductID, 10, 2), nil
		},
	}
	service, err := NewService(repo, publisher, func() time.Time { return now })
	if err != nil {
		t.Fatalf("create service: %v", err)
	}

	_, err = service.SetStock(context.Background(), "pharmacy-1", "product-1", 10, "user-1")
	if err != nil {
		t.Fatalf("SetStock returned error: %v", err)
	}
	if len(publisher.increased) != 1 {
		t.Fatalf("StockIncreased calls = %d, want 1", len(publisher.increased))
	}
	event := publisher.increased[0]
	if event.previousAvailable != 3 || event.newAvailable != 8 {
		t.Fatalf("available quantities = %d -> %d, want 3 -> 8",
			event.previousAvailable, event.newAvailable)
	}
}

func TestSetStockDoesNotPublishWhenAvailableQuantityDoesNotIncrease(t *testing.T) {
	publisher := &publisherFake{}
	repo := &repositoryFake{
		setFn: func(_ context.Context, requested domain.Stock, _ string) (int32, domain.Stock, error) {
			return 5, testStock(t, requested.PharmacyID, requested.ProductID, 7, 2), nil
		},
	}
	service := newTestService(t, repo, publisher)

	_, err := service.SetStock(context.Background(), "pharmacy-1", "product-1", 7, "user-1")
	if err != nil {
		t.Fatalf("SetStock returned error: %v", err)
	}
	if len(publisher.increased) != 0 {
		t.Fatalf("StockIncreased calls = %d, want 0", len(publisher.increased))
	}
}

func TestReservePublishesReservedEvent(t *testing.T) {
	publisher := &publisherFake{}
	repo := &repositoryFake{
		reserveFn: func(_ context.Context, holdID string, key ports.StockKey, quantity int32) (domain.Stock, bool, error) {
			return testStock(t, key.PharmacyID, key.ProductID, 10, quantity), true, nil
		},
	}
	service := newTestService(t, repo, publisher)

	_, reserved, err := service.Reserve(
		context.Background(),
		"hold-1",
		ports.StockKey{PharmacyID: "pharmacy-1", ProductID: "product-1"},
		2,
	)
	if err != nil {
		t.Fatalf("Reserve returned error: %v", err)
	}
	if !reserved {
		t.Fatal("Reserve returned reserved=false, want true")
	}
	if len(publisher.reserved) != 1 || len(publisher.unavailable) != 0 {
		t.Fatalf("published reserved=%d unavailable=%d, want 1 and 0",
			len(publisher.reserved), len(publisher.unavailable))
	}
}

func TestReservePublishesUnavailableEvent(t *testing.T) {
	publisher := &publisherFake{}
	repo := &repositoryFake{
		reserveFn: func(_ context.Context, _ string, key ports.StockKey, _ int32) (domain.Stock, bool, error) {
			return testStock(t, key.PharmacyID, key.ProductID, 5, 3), false, nil
		},
	}
	service := newTestService(t, repo, publisher)

	_, reserved, err := service.Reserve(
		context.Background(),
		"hold-1",
		ports.StockKey{PharmacyID: "pharmacy-1", ProductID: "product-1"},
		4,
	)
	if err != nil {
		t.Fatalf("Reserve returned error: %v", err)
	}
	if reserved {
		t.Fatal("Reserve returned reserved=true, want false")
	}
	if len(publisher.unavailable) != 1 {
		t.Fatalf("StockUnavailable calls = %d, want 1", len(publisher.unavailable))
	}
	event := publisher.unavailable[0]
	if event.requestedQuantity != 4 || event.availableQuantity != 2 {
		t.Fatalf("quantities = requested %d, available %d; want 4 and 2",
			event.requestedQuantity, event.availableQuantity)
	}
}

func TestBulkSetPublishesOnlyIncreases(t *testing.T) {
	publisher := &publisherFake{}
	repo := &repositoryFake{
		bulkSetFn: func(context.Context, string, []ports.StockUpdate, string) ([]ports.StockChange, []ports.StockUpdateError, error) {
			return []ports.StockChange{
				{
					Stock:             testStock(t, "pharmacy-1", "product-1", 8, 1),
					PreviousAvailable: 3,
				},
				{
					Stock:             testStock(t, "pharmacy-1", "product-2", 4, 0),
					PreviousAvailable: 5,
				},
			}, nil, nil
		},
	}
	service := newTestService(t, repo, publisher)

	_, _, err := service.BulkSetStock(
		context.Background(),
		"pharmacy-1",
		[]ports.StockUpdate{{ProductID: "product-1", QuantityOnHand: 8}},
		"user-1",
	)
	if err != nil {
		t.Fatalf("BulkSetStock returned error: %v", err)
	}
	if len(publisher.increased) != 1 {
		t.Fatalf("StockIncreased calls = %d, want 1", len(publisher.increased))
	}
	if publisher.increased[0].productID != "product-1" {
		t.Fatalf("published product = %q, want product-1", publisher.increased[0].productID)
	}
}

var errCoverageRepo = errors.New("coverage repository failure")

type coverageRepository struct {
	*repositoryFake
	getManyFn func(context.Context, []string, []string, bool) ([]domain.Stock, error)
	listFn    func(context.Context, string, int32, string) ([]domain.Stock, string, error)
	commitFn  func(context.Context, string) (domain.Stock, error)
	releaseFn func(context.Context, string) (domain.Stock, error)
}

func newCoverageRepository() *coverageRepository {
	return &coverageRepository{repositoryFake: &repositoryFake{}}
}

func (r *coverageRepository) GetMany(ctx context.Context, pharmacyIDs, productIDs []string, onlyAvailable bool) ([]domain.Stock, error) {
	if r.getManyFn != nil {
		return r.getManyFn(ctx, pharmacyIDs, productIDs, onlyAvailable)
	}
	return nil, nil
}

func (r *coverageRepository) ListByPharmacy(ctx context.Context, pharmacyID string, pageSize int32, pageToken string) ([]domain.Stock, string, error) {
	if r.listFn != nil {
		return r.listFn(ctx, pharmacyID, pageSize, pageToken)
	}
	return nil, "", nil
}

func (r *coverageRepository) CommitReservation(ctx context.Context, holdID string) (domain.Stock, error) {
	if r.commitFn != nil {
		return r.commitFn(ctx, holdID)
	}
	return domain.Stock{}, nil
}

func (r *coverageRepository) ReleaseReservation(ctx context.Context, holdID string) (domain.Stock, error) {
	if r.releaseFn != nil {
		return r.releaseFn(ctx, holdID)
	}
	return domain.Stock{}, nil
}

func TestNewServiceValidatesDependenciesAndDefaultsClock(t *testing.T) {
	publisher := &publisherFake{}
	if _, err := NewService(nil, publisher, nil); err == nil {
		t.Fatal("NewService with nil repository returned no error")
	}
	if _, err := NewService(newCoverageRepository(), nil, nil); err == nil {
		t.Fatal("NewService with nil publisher returned no error")
	}
	service, err := NewService(newCoverageRepository(), publisher, nil)
	if err != nil {
		t.Fatalf("NewService returned error: %v", err)
	}
	if service.now().IsZero() {
		t.Fatal("default clock returned zero time")
	}
}

func TestGetStockSuccessAndRepositoryError(t *testing.T) {
	want := testStock(t, "pharmacy-1", "product-1", 8, 2)
	repo := newCoverageRepository()
	repo.getFn = func(_ context.Context, key ports.StockKey) (domain.Stock, bool, error) {
		if key != (ports.StockKey{PharmacyID: "pharmacy-1", ProductID: "product-1"}) {
			t.Fatalf("Get key = %+v", key)
		}
		return want, true, nil
	}
	service := newTestService(t, repo, &publisherFake{})
	got, err := service.GetStock(context.Background(), "pharmacy-1", "product-1")
	if err != nil || got != want {
		t.Fatalf("GetStock = (%+v, %v), want (%+v, nil)", got, err, want)
	}
	repo.getFn = func(context.Context, ports.StockKey) (domain.Stock, bool, error) {
		return domain.Stock{}, false, errCoverageRepo
	}
	if _, err := service.GetStock(context.Background(), "pharmacy-1", "product-1"); !errors.Is(err, errCoverageRepo) {
		t.Fatalf("GetStock error = %v, want wrapped repository error", err)
	}
}

func TestBatchGetStockSuccessAndRepositoryError(t *testing.T) {
	want := []domain.Stock{testStock(t, "pharmacy-1", "product-1", 5, 0)}
	repo := newCoverageRepository()
	repo.getManyFn = func(_ context.Context, pharmacies, products []string, onlyAvailable bool) ([]domain.Stock, error) {
		if len(pharmacies) != 1 || pharmacies[0] != "pharmacy-1" || len(products) != 1 || products[0] != "product-1" || !onlyAvailable {
			t.Fatalf("GetMany arguments = %v, %v, %v", pharmacies, products, onlyAvailable)
		}
		return want, nil
	}
	service := newTestService(t, repo, &publisherFake{})
	got, err := service.BatchGetStock(context.Background(), []string{"pharmacy-1"}, []string{"product-1"}, true)
	if err != nil || len(got) != 1 || got[0] != want[0] {
		t.Fatalf("BatchGetStock = (%v, %v), want one stock and nil", got, err)
	}
	repo.getManyFn = func(context.Context, []string, []string, bool) ([]domain.Stock, error) {
		return nil, errCoverageRepo
	}
	if _, err := service.BatchGetStock(context.Background(), nil, nil, false); !errors.Is(err, errCoverageRepo) {
		t.Fatalf("BatchGetStock error = %v, want wrapped repository error", err)
	}
}

func TestListPharmacyStockSuccessAndRepositoryError(t *testing.T) {
	want := []domain.Stock{testStock(t, "pharmacy-1", "product-1", 3, 1)}
	repo := newCoverageRepository()
	repo.listFn = func(_ context.Context, pharmacy string, size int32, token string) ([]domain.Stock, string, error) {
		if pharmacy != "pharmacy-1" || size != 10 || token != "next" {
			t.Fatalf("ListByPharmacy arguments = %q, %d, %q", pharmacy, size, token)
		}
		return want, "next-page", nil
	}
	service := newTestService(t, repo, &publisherFake{})
	got, next, err := service.ListPharmacyStock(context.Background(), "pharmacy-1", 10, "next")
	if err != nil || next != "next-page" || len(got) != 1 || got[0] != want[0] {
		t.Fatalf("ListPharmacyStock = (%v, %q, %v)", got, next, err)
	}
	repo.listFn = func(context.Context, string, int32, string) ([]domain.Stock, string, error) {
		return nil, "", errCoverageRepo
	}
	if _, _, err := service.ListPharmacyStock(context.Background(), "pharmacy-1", 10, ""); !errors.Is(err, errCoverageRepo) {
		t.Fatalf("ListPharmacyStock error = %v, want wrapped repository error", err)
	}
}

func TestSetStockValidationRepositoryAndPublisherErrors(t *testing.T) {
	repo := newCoverageRepository()
	service := newTestService(t, repo, &publisherFake{})
	if _, err := service.SetStock(context.Background(), "pharmacy-1", "product-1", -1, "user-1"); err == nil {
		t.Fatal("SetStock accepted a negative quantity")
	}
	repo.setFn = func(context.Context, domain.Stock, string) (int32, domain.Stock, error) {
		return 0, domain.Stock{}, errCoverageRepo
	}
	if _, err := service.SetStock(context.Background(), "pharmacy-1", "product-1", 5, "user-1"); !errors.Is(err, errCoverageRepo) {
		t.Fatalf("SetStock repository error = %v", err)
	}
	publisher := &publisherFake{err: errCoverageRepo}
	repo.setFn = func(_ context.Context, requested domain.Stock, _ string) (int32, domain.Stock, error) {
		return 0, testStock(t, requested.PharmacyID, requested.ProductID, 5, 0), nil
	}
	service = newTestService(t, repo, publisher)
	updated, err := service.SetStock(context.Background(), "pharmacy-1", "product-1", 5, "user-1")
	if !errors.Is(err, errCoverageRepo) || updated.QuantityOnHand != 5 {
		t.Fatalf("SetStock publisher failure = (%+v, %v)", updated, err)
	}
}

func TestBulkSetStockRepositoryAndPublisherErrors(t *testing.T) {
	repo := newCoverageRepository()
	service := newTestService(t, repo, &publisherFake{})
	repo.bulkSetFn = func(context.Context, string, []ports.StockUpdate, string) ([]ports.StockChange, []ports.StockUpdateError, error) {
		return nil, nil, errCoverageRepo
	}
	if _, _, err := service.BulkSetStock(context.Background(), "pharmacy-1", nil, "user-1"); !errors.Is(err, errCoverageRepo) {
		t.Fatalf("BulkSetStock repository error = %v", err)
	}
	change := ports.StockChange{Stock: testStock(t, "pharmacy-1", "product-1", 8, 0), PreviousAvailable: 2}
	rowErrors := []ports.StockUpdateError{{}}
	repo.bulkSetFn = func(context.Context, string, []ports.StockUpdate, string) ([]ports.StockChange, []ports.StockUpdateError, error) {
		return []ports.StockChange{change}, rowErrors, nil
	}
	publisher := &publisherFake{err: errCoverageRepo}
	service = newTestService(t, repo, publisher)
	changes, gotErrors, err := service.BulkSetStock(context.Background(), "pharmacy-1", nil, "user-1")
	if !errors.Is(err, errCoverageRepo) || len(changes) != 1 || len(gotErrors) != 1 {
		t.Fatalf("BulkSetStock publisher failure = (%v, %v, %v)", changes, gotErrors, err)
	}
}

func TestReserveRepositoryAndPublisherErrors(t *testing.T) {
	key := ports.StockKey{PharmacyID: "pharmacy-1", ProductID: "product-1"}
	repo := newCoverageRepository()
	repo.reserveFn = func(context.Context, string, ports.StockKey, int32) (domain.Stock, bool, error) {
		return domain.Stock{}, false, errCoverageRepo
	}
	service := newTestService(t, repo, &publisherFake{})
	if _, _, err := service.Reserve(context.Background(), "hold-1", key, 1); !errors.Is(err, errCoverageRepo) {
		t.Fatalf("Reserve repository error = %v", err)
	}
	repo.reserveFn = func(context.Context, string, ports.StockKey, int32) (domain.Stock, bool, error) {
		return testStock(t, key.PharmacyID, key.ProductID, 4, 1), true, nil
	}
	service = newTestService(t, repo, &publisherFake{err: errCoverageRepo})
	stock, reserved, err := service.Reserve(context.Background(), "hold-1", key, 1)
	if !errors.Is(err, errCoverageRepo) || !reserved || stock.QuantityReserved != 1 {
		t.Fatalf("Reserve publisher failure = (%+v, %v, %v)", stock, reserved, err)
	}
}

func TestCommitAndReleaseReservationSuccessAndErrors(t *testing.T) {
	want := testStock(t, "pharmacy-1", "product-1", 5, 1)
	repo := newCoverageRepository()
	service := newTestService(t, repo, &publisherFake{})
	repo.commitFn = func(_ context.Context, holdID string) (domain.Stock, error) {
		if holdID != "hold-1" {
			t.Errorf("commit hold ID = %q", holdID)
		}
		return want, nil
	}
	if got, err := service.CommitReservation(context.Background(), "hold-1"); err != nil || got != want {
		t.Fatalf("CommitReservation = (%+v, %v)", got, err)
	}
	repo.commitFn = func(context.Context, string) (domain.Stock, error) { return domain.Stock{}, errCoverageRepo }
	if _, err := service.CommitReservation(context.Background(), "hold-1"); !errors.Is(err, errCoverageRepo) {
		t.Fatalf("CommitReservation error = %v", err)
	}
	repo.releaseFn = func(_ context.Context, holdID string) (domain.Stock, error) {
		if holdID != "hold-1" {
			t.Errorf("release hold ID = %q", holdID)
		}
		return want, nil
	}
	if got, err := service.ReleaseReservation(context.Background(), "hold-1"); err != nil || got != want {
		t.Fatalf("ReleaseReservation = (%+v, %v)", got, err)
	}
	repo.releaseFn = func(context.Context, string) (domain.Stock, error) { return domain.Stock{}, errCoverageRepo }
	if _, err := service.ReleaseReservation(context.Background(), "hold-1"); !errors.Is(err, errCoverageRepo) {
		t.Fatalf("ReleaseReservation error = %v", err)
	}
}
