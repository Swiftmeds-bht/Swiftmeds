package app

import (
	"context"
	"errors"
	"testing"

	"github.com/Swiftmeds-bht/Swiftmeds/services/catalog/internal/domain"
)

var errDB = errors.New("database unavailable")

// failingRepository records whether it was called and fails every call with err.
type failingRepository struct {
	err   error
	calls int
}

func (r *failingRepository) SearchByName(context.Context, string, int) ([]domain.Product, error) {
	r.calls++
	return nil, r.err
}

func (r *failingRepository) GetByID(context.Context, string) (domain.Product, error) {
	r.calls++
	return domain.Product{}, r.err
}

func (r *failingRepository) GetMany(context.Context, []string) ([]domain.Product, error) {
	r.calls++
	return nil, r.err
}

func (r *failingRepository) ListByMolecule(context.Context, string, string) ([]domain.Product, error) {
	r.calls++
	return nil, r.err
}

func (r *failingRepository) Create(context.Context, domain.Product) (domain.Product, error) {
	r.calls++
	return domain.Product{}, r.err
}

func TestResolveNameLimitBoundaries(t *testing.T) {
	tests := []struct {
		limit, want int
	}{
		{-5, defaultSearchLimit},
		{0, defaultSearchLimit},
		{1, 1},
		{maxSearchLimit - 1, maxSearchLimit - 1},
		{maxSearchLimit, maxSearchLimit},
		{maxSearchLimit + 1, maxSearchLimit},
	}
	for _, tt := range tests {
		repo := &fakeProductRepository{}
		if _, err := NewService(repo).ResolveName(context.Background(), "crocin", tt.limit); err != nil {
			t.Fatalf("limit %d: %v", tt.limit, err)
		}
		if repo.searchLimit != tt.want {
			t.Errorf("limit %d was sent as %d, want %d", tt.limit, repo.searchLimit, tt.want)
		}
	}
}

func TestResolveNameReturnsTheRepositoryResult(t *testing.T) {
	want := []domain.Product{{ProductID: "p-1", BrandName: "Crocin"}, {ProductID: "p-2", BrandName: "Calpol"}}
	repo := &fakeProductRepository{searchResult: want}

	got, err := NewService(repo).ResolveName(context.Background(), "c", 5)
	if err != nil {
		t.Fatalf("ResolveName: %v", err)
	}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("products = %+v, want %+v", got, want)
	}
}

func TestGetProductRejectsBlankIDWithoutCallingTheRepository(t *testing.T) {
	repo := &failingRepository{err: errDB}
	_, err := NewService(repo).GetProduct(context.Background(), "  ")
	if !errors.Is(err, domain.ErrMissingProductID) {
		t.Fatalf("error = %v, want ErrMissingProductID", err)
	}
	if repo.calls != 0 {
		t.Fatalf("repository calls = %d, want 0", repo.calls)
	}
}

func TestListProductsByMoleculeRejectsBlankMolecule(t *testing.T) {
	repo := &failingRepository{err: errDB}
	_, err := NewService(repo).ListProductsByMolecule(context.Background(), " ", "500mg")
	if !errors.Is(err, domain.ErrMissingMoleculeID) {
		t.Fatalf("error = %v, want ErrMissingMoleculeID", err)
	}
	if repo.calls != 0 {
		t.Fatalf("repository calls = %d, want 0", repo.calls)
	}
}

// An empty strength means "every strength" and is passed on as "".
func TestListProductsByMoleculeWithoutStrength(t *testing.T) {
	repo := &fakeProductRepository{strength: "unchanged"}
	if _, err := NewService(repo).ListProductsByMolecule(context.Background(), "mol-1", "   "); err != nil {
		t.Fatalf("ListProductsByMolecule: %v", err)
	}
	if repo.moleculeID != "mol-1" || repo.strength != "" {
		t.Fatalf("repository got (%q, %q), want (mol-1, \"\")", repo.moleculeID, repo.strength)
	}
}

func TestBatchGetProductsKeepsFirstOrder(t *testing.T) {
	repo := &fakeProductRepository{}
	if _, err := NewService(repo).BatchGetProducts(context.Background(), []string{"p-3", "p-1", "p-3", " p-2", "p-1"}); err != nil {
		t.Fatalf("BatchGetProducts: %v", err)
	}
	want := []string{"p-3", "p-1", "p-2"}
	if len(repo.productIDs) != len(want) {
		t.Fatalf("IDs = %v, want %v", repo.productIDs, want)
	}
	for i := range want {
		if repo.productIDs[i] != want[i] {
			t.Fatalf("IDs = %v, want %v", repo.productIDs, want)
		}
	}
}

func TestBatchGetProductsNilInputReturnsAnEmptyList(t *testing.T) {
	got, err := NewService(&failingRepository{err: errDB}).BatchGetProducts(context.Background(), nil)
	if err != nil {
		t.Fatalf("BatchGetProducts(nil): %v", err)
	}
	if got == nil || len(got) != 0 {
		t.Fatalf("products = %#v, want an empty, non-nil list", got)
	}
}

func TestRepositoryErrorsAreWrapped(t *testing.T) {
	ctx := context.Background()
	calls := map[string]func(*Service) error{
		"ResolveName": func(s *Service) error { _, err := s.ResolveName(ctx, "crocin", 10); return err },
		"GetProduct":  func(s *Service) error { _, err := s.GetProduct(ctx, "p-1"); return err },
		"BatchGetProducts": func(s *Service) error {
			_, err := s.BatchGetProducts(ctx, []string{"p-1"})
			return err
		},
		"ListProductsByMolecule": func(s *Service) error {
			_, err := s.ListProductsByMolecule(ctx, "mol-1", "")
			return err
		},
		"CreateProduct": func(s *Service) error {
			_, err := s.CreateProduct(ctx, domain.ProductInput{
				BrandName: "Crocin", GenericName: "Paracetamol", MoleculeID: "mol-1",
				MoleculeName: "Paracetamol", Strength: "500mg", DosageForm: domain.DosageFormTablet, PackSize: 10,
			})
			return err
		},
	}
	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			repo := &failingRepository{err: errDB}
			err := call(NewService(repo))
			if !errors.Is(err, errDB) {
				t.Fatalf("error = %v, want it to wrap the repository error", err)
			}
			if repo.calls != 1 {
				t.Fatalf("repository calls = %d, want 1", repo.calls)
			}
		})
	}
}

func TestGetProductPassesNotFoundThrough(t *testing.T) {
	repo := &failingRepository{err: domain.ErrProductNotFound}
	_, err := NewService(repo).GetProduct(context.Background(), "missing")
	if !errors.Is(err, domain.ErrProductNotFound) {
		t.Fatalf("error = %v, want ErrProductNotFound", err)
	}
}

func TestCreateProductValidationErrorsSkipTheRepository(t *testing.T) {
	repo := &failingRepository{err: errDB}
	_, err := NewService(repo).CreateProduct(context.Background(), domain.ProductInput{
		BrandName: "Crocin", GenericName: "Paracetamol", MoleculeID: "mol-1",
		MoleculeName: "Paracetamol", Strength: "500mg", DosageForm: domain.DosageFormTablet, PackSize: 0,
	})
	if !errors.Is(err, domain.ErrInvalidPackSize) {
		t.Fatalf("error = %v, want ErrInvalidPackSize", err)
	}
	if repo.calls != 0 {
		t.Fatalf("repository calls = %d, want 0", repo.calls)
	}
}
