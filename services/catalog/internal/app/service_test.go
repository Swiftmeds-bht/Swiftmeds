package app

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/Swiftmeds-bht/Swiftmeds/services/catalog/internal/domain"
)

type fakeProductRepository struct {
	searchQuery  string
	searchLimit  int
	productID    string
	productIDs   []string
	moleculeID   string
	strength     string
	created      domain.Product
	createCalls  int
	searchResult []domain.Product
	batchResult  []domain.Product
	createErr    error
}

func (f *fakeProductRepository) SearchByName(
	_ context.Context,
	query string,
	limit int,
) ([]domain.Product, error) {
	f.searchQuery = query
	f.searchLimit = limit
	return f.searchResult, nil
}

func (f *fakeProductRepository) GetByID(
	_ context.Context,
	productID string,
) (domain.Product, error) {
	f.productID = productID
	return domain.Product{ProductID: productID}, nil
}

func (f *fakeProductRepository) GetMany(
	_ context.Context,
	productIDs []string,
) ([]domain.Product, error) {
	f.productIDs = productIDs
	return f.batchResult, nil
}

func (f *fakeProductRepository) ListByMolecule(
	_ context.Context,
	moleculeID string,
	strength string,
) ([]domain.Product, error) {
	f.moleculeID = moleculeID
	f.strength = strength
	return nil, nil
}

func (f *fakeProductRepository) Create(
	_ context.Context,
	product domain.Product,
) (domain.Product, error) {
	f.createCalls++
	f.created = product
	if f.createErr != nil {
		return domain.Product{}, f.createErr
	}
	product.ProductID = "generated-product-id"
	return product, nil
}

func TestResolveNameTrimsQueryAndUsesDefaultLimit(t *testing.T) {
	repo := &fakeProductRepository{}
	service := NewService(repo)

	_, err := service.ResolveName(context.Background(), "  Crocin  ", 0)
	if err != nil {
		t.Fatalf("ResolveName() error = %v", err)
	}
	if repo.searchQuery != "Crocin" {
		t.Errorf("query = %q, want %q", repo.searchQuery, "Crocin")
	}
	if repo.searchLimit != 20 {
		t.Errorf("limit = %d, want 20", repo.searchLimit)
	}
}

func TestResolveNameRejectsBlankQuery(t *testing.T) {
	repo := &fakeProductRepository{}
	service := NewService(repo)

	_, err := service.ResolveName(context.Background(), "  ", 10)
	if !errors.Is(err, domain.ErrMissingQuery) {
		t.Fatalf("ResolveName() error = %v, want ErrMissingQuery", err)
	}
	if repo.searchQuery != "" {
		t.Errorf("repository called with query %q", repo.searchQuery)
	}
}

func TestResolveNameCapsLimit(t *testing.T) {
	repo := &fakeProductRepository{}
	service := NewService(repo)

	_, err := service.ResolveName(context.Background(), "Crocin", 500)
	if err != nil {
		t.Fatalf("ResolveName() error = %v", err)
	}
	if repo.searchLimit != 100 {
		t.Errorf("limit = %d, want 100", repo.searchLimit)
	}
}

func TestGetProductTrimsID(t *testing.T) {
	repo := &fakeProductRepository{}
	service := NewService(repo)

	_, err := service.GetProduct(context.Background(), " product-1 ")
	if err != nil {
		t.Fatalf("GetProduct() error = %v", err)
	}
	if repo.productID != "product-1" {
		t.Errorf("product ID = %q, want %q", repo.productID, "product-1")
	}
}

func TestBatchGetProductsTrimsDeduplicatesAndSkipsEmptyIDs(t *testing.T) {
	repo := &fakeProductRepository{}
	service := NewService(repo)

	_, err := service.BatchGetProducts(
		context.Background(),
		[]string{" product-1 ", "", "product-1", "product-2"},
	)
	if err != nil {
		t.Fatalf("BatchGetProducts() error = %v", err)
	}

	want := []string{"product-1", "product-2"}
	if !reflect.DeepEqual(repo.productIDs, want) {
		t.Errorf("product IDs = %#v, want %#v", repo.productIDs, want)
	}
}

func TestBatchGetProductsEmptyInputSkipsRepository(t *testing.T) {
	repo := &fakeProductRepository{}
	service := NewService(repo)

	products, err := service.BatchGetProducts(context.Background(), []string{" ", ""})
	if err != nil {
		t.Fatalf("BatchGetProducts() error = %v", err)
	}
	if len(products) != 0 {
		t.Errorf("got %d products, want none", len(products))
	}
	if repo.productIDs != nil {
		t.Errorf("repository called with IDs %#v, want no call", repo.productIDs)
	}
}

func TestListProductsByMoleculeNormalizesStrength(t *testing.T) {
	repo := &fakeProductRepository{}
	service := NewService(repo)

	_, err := service.ListProductsByMolecule(
		context.Background(),
		" molecule-1 ",
		" 500 MG ",
	)
	if err != nil {
		t.Fatalf("ListProductsByMolecule() error = %v", err)
	}
	if repo.moleculeID != "molecule-1" {
		t.Errorf("molecule ID = %q, want %q", repo.moleculeID, "molecule-1")
	}
	if repo.strength != "500mg" {
		t.Errorf("strength = %q, want %q", repo.strength, "500mg")
	}
}

func TestCreateProductValidatesAndNormalizesInput(t *testing.T) {
	repo := &fakeProductRepository{}
	service := NewService(repo)

	product, err := service.CreateProduct(context.Background(), domain.ProductInput{
		BrandName:            "  Crocin  ",
		GenericName:          " Paracetamol ",
		MoleculeID:           " molecule-1 ",
		MoleculeName:         " Paracetamol ",
		Strength:             " 500 MG ",
		DosageForm:           domain.DosageFormTablet,
		PackSize:             10,
		Manufacturer:         " Demo ",
		RequiresPrescription: false,
		Controlled:           false,
	})
	if err != nil {
		t.Fatalf("CreateProduct() error = %v", err)
	}
	if product.ProductID != "generated-product-id" {
		t.Errorf("product ID = %q, want generated ID", product.ProductID)
	}
	if repo.created.BrandName != "Crocin" ||
		repo.created.GenericName != "Paracetamol" ||
		repo.created.MoleculeID != "molecule-1" ||
		repo.created.MoleculeName != "Paracetamol" ||
		repo.created.Strength != "500mg" {
		t.Errorf("repository received unnormalized product: %+v", repo.created)
	}
	if repo.createCalls != 1 {
		t.Errorf("Create called %d times, want 1", repo.createCalls)
	}
}

func TestCreateProductRejectsInvalidDosageForm(t *testing.T) {
	repo := &fakeProductRepository{}
	service := NewService(repo)

	_, err := service.CreateProduct(context.Background(), domain.ProductInput{
		BrandName:    "Crocin",
		GenericName:  "Paracetamol",
		MoleculeID:   "molecule-1",
		MoleculeName: "Paracetamol",
		Strength:     "500mg",
		DosageForm:   domain.DosageFormUnspecified,
		PackSize:     10,
	})
	if !errors.Is(err, domain.ErrInvalidDosageForm) {
		t.Fatalf("CreateProduct() error = %v, want ErrInvalidDosageForm", err)
	}
	if repo.createCalls != 0 {
		t.Errorf("repository Create called %d times, want 0", repo.createCalls)
	}
}
