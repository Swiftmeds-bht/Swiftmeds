package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Swiftmeds-bht/Swiftmeds/services/pricing/internal/domain"
)

var fixedNow = time.Date(2026, 10, 14, 9, 0, 0, 0, time.UTC)

func clock() time.Time { return fixedNow }

// fakeRepo is a hand-written test double for ports.PriceRepository.
type fakeRepo struct {
	saved   []domain.Price
	stored  []domain.Price
	saveErr error
}

func (f *fakeRepo) SaveAll(_ context.Context, prices []domain.Price) error {
	if f.saveErr != nil {
		return f.saveErr
	}
	f.saved = append(f.saved, prices...)
	return nil
}

func (f *fakeRepo) GetMany(_ context.Context, _ []domain.Key) ([]domain.Price, error) {
	return f.stored, nil
}

func TestSetPricesSavesAllValidUpdates(t *testing.T) {
	repo := &fakeRepo{}
	svc := NewService(repo, clock)

	got, err := svc.SetPrices(context.Background(), "ph-1", []Update{
		{ProductID: "pr-1", AmountChhertum: 4500},
		{ProductID: "pr-2", AmountChhertum: 12000},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || len(repo.saved) != 2 {
		t.Fatalf("returned %d, saved %d; want 2 and 2", len(got), len(repo.saved))
	}
	if !repo.saved[0].UpdatedAt.Equal(fixedNow) {
		t.Errorf("UpdatedAt = %v, want the injected clock time", repo.saved[0].UpdatedAt)
	}
}

func TestSetPricesSavesNothingIfAnyUpdateIsInvalid(t *testing.T) {
	repo := &fakeRepo{}
	svc := NewService(repo, clock)

	_, err := svc.SetPrices(context.Background(), "ph-1", []Update{
		{ProductID: "pr-1", AmountChhertum: 4500},
		{ProductID: "pr-2", AmountChhertum: 0},
	})
	if !errors.Is(err, domain.ErrNonPositivePrice) {
		t.Fatalf("err = %v, want ErrNonPositivePrice", err)
	}
	if len(repo.saved) != 0 {
		t.Errorf("saved %d prices, want 0 (all or nothing)", len(repo.saved))
	}
}

func TestSetPricesWrapsRepositoryErrors(t *testing.T) {
	dbDown := errors.New("db down")
	svc := NewService(&fakeRepo{saveErr: dbDown}, clock)

	_, err := svc.SetPrices(context.Background(), "ph-1", []Update{{ProductID: "pr-1", AmountChhertum: 1}})
	if !errors.Is(err, dbDown) {
		t.Fatalf("err = %v, want wrapped db error", err)
	}
}

func TestBatchGetPricesReturnsStoredPrices(t *testing.T) {
	stored := []domain.Price{{Key: domain.Key{PharmacyID: "ph-1", ProductID: "pr-1"}, AmountChhertum: 4500}}
	svc := NewService(&fakeRepo{stored: stored}, clock)

	got, err := svc.BatchGetPrices(context.Background(), []domain.Key{{PharmacyID: "ph-1", ProductID: "pr-1"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != stored[0] {
		t.Errorf("got %+v, want %+v", got, stored)
	}
}
