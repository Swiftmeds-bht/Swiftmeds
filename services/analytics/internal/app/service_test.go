package app

import (
	"context"
	"testing"
	"time"

	"github.com/Swiftmeds-bht/Swiftmeds/services/analytics/internal/domain"
)

type repoStub struct {
	search domain.SearchRecorded
	stock  domain.StockUnavailable
	query  domain.DemandQuery
}

func (r *repoStub) StoreSearchRecorded(_ context.Context, e domain.SearchRecorded) error {
	r.search = e
	return nil
}
func (r *repoStub) StoreStockUnavailable(_ context.Context, e domain.StockUnavailable) error {
	r.stock = e
	return nil
}
func (r *repoStub) UnmetDemand(_ context.Context, q domain.DemandQuery) ([]domain.DemandItem, error) {
	r.query = q
	return nil, nil
}

type registryStub struct{}

func (registryStub) GetPharmacy(context.Context, string) (string, error) { return " Paro ", nil }

type catalogStub struct{}

func (catalogStub) GetProduct(context.Context, string) (string, error) { return "Paracetamol", nil }
func TestGetUnmetDemandDefaultsLimitAndTrimsArea(t *testing.T) {
	r := &repoStub{}
	const occurredAtUnix = int64(123)
	s := NewService(r, registryStub{}, catalogStub{}, func() time.Time {
		return time.Unix(occurredAtUnix, 0)
	})
	err := s.RecordStockUnavailable(context.Background(), domain.StockUnavailable{
		EventID: "event-1", PharmacyID: "pharmacy-1", ProductID: "product-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if r.stock.EventID != "event-1" || r.stock.PharmacyID != "pharmacy-1" ||
		r.stock.ProductID != "product-1" || r.stock.Dzongkhag != "Paro" ||
		r.stock.ProductName != "Paracetamol" ||
		!r.stock.OccurredAt.Equal(time.Unix(occurredAtUnix, 0)) {
		t.Fatalf("resolved dimensions = %#v", r.stock)
	}
	_, err = s.GetUnmetDemand(context.Background(), domain.DemandQuery{
		From: time.Unix(1, 0), To: time.Unix(2, 0), Dzongkhag: " Paro ",
	})
	if err != nil {
		t.Fatal(err)
	}
	if r.query.Limit != DefaultLimit || r.query.Dzongkhag != "Paro" {
		t.Fatalf("demand query = %#v", r.query)
	}
}

func TestRecordStockUnavailableResolvesDimensionsBeforePersisting(t *testing.T) {
	r := &repoStub{}
	s := NewService(r, registryStub{}, catalogStub{}, time.Now)
	err := s.RecordStockUnavailable(context.Background(), domain.StockUnavailable{
		EventID: "event-1", PharmacyID: "pharmacy-1", ProductID: "product-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if r.stock.EventID != "event-1" || r.stock.PharmacyID != "pharmacy-1" ||
		r.stock.ProductID != "product-1" || r.stock.Dzongkhag != "Paro" ||
		r.stock.ProductName != "Paracetamol" {
		t.Fatalf("resolved dimensions = %#v", r.stock)
	}
}
