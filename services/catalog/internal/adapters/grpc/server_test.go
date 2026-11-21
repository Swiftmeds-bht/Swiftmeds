package grpc

import (
	"context"
	"errors"
	"strings"
	"testing"

	catalogv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/catalog/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/catalog/internal/app"
	"github.com/Swiftmeds-bht/Swiftmeds/services/catalog/internal/domain"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// memoryRepository is a small ProductRepository for handler tests. Set err to
// make every call fail.
type memoryRepository struct {
	products map[string]domain.Product
	created  []domain.Product
	err      error
}

func (r *memoryRepository) SearchByName(_ context.Context, query string, limit int) ([]domain.Product, error) {
	if r.err != nil {
		return nil, r.err
	}
	var out []domain.Product
	for _, p := range r.products {
		if strings.Contains(strings.ToLower(p.BrandName), strings.ToLower(query)) && len(out) < limit {
			out = append(out, p)
		}
	}
	return out, nil
}

func (r *memoryRepository) GetByID(_ context.Context, id string) (domain.Product, error) {
	if r.err != nil {
		return domain.Product{}, r.err
	}
	p, ok := r.products[id]
	if !ok {
		return domain.Product{}, domain.ErrProductNotFound
	}
	return p, nil
}

func (r *memoryRepository) GetMany(_ context.Context, ids []string) ([]domain.Product, error) {
	if r.err != nil {
		return nil, r.err
	}
	var out []domain.Product
	for _, id := range ids {
		if p, ok := r.products[id]; ok {
			out = append(out, p)
		}
	}
	return out, nil
}

func (r *memoryRepository) ListByMolecule(_ context.Context, moleculeID, strength string) ([]domain.Product, error) {
	if r.err != nil {
		return nil, r.err
	}
	var out []domain.Product
	for _, p := range r.products {
		if p.MoleculeID == moleculeID && (strength == "" || p.Strength == strength) {
			out = append(out, p)
		}
	}
	return out, nil
}

func (r *memoryRepository) Create(_ context.Context, p domain.Product) (domain.Product, error) {
	if r.err != nil {
		return domain.Product{}, r.err
	}
	p.ProductID = "new-id"
	r.created = append(r.created, p)
	return p, nil
}

var crocin = domain.Product{
	ProductID: "p-1", BrandName: "Crocin", GenericName: "Paracetamol",
	MoleculeID: "mol-1", MoleculeName: "Paracetamol", Strength: "500mg",
	DosageForm: domain.DosageFormTablet, PackSize: 10, Manufacturer: "GSK",
	RequiresPrescription: true, Controlled: false,
}

func newServer(repo *memoryRepository) *Server {
	if repo.products == nil {
		repo.products = map[string]domain.Product{crocin.ProductID: crocin}
	}
	return NewServer(app.NewService(repo))
}

func wantCode(t *testing.T, err error, want codes.Code) {
	t.Helper()
	if got := status.Code(err); got != want {
		t.Fatalf("code = %s (%v), want %s", got, err, want)
	}
}

func TestGetProductMapsEveryField(t *testing.T) {
	resp, err := newServer(&memoryRepository{}).GetProduct(context.Background(),
		&catalogv1.GetProductRequest{ProductId: " p-1 "})
	if err != nil {
		t.Fatalf("GetProduct: %v", err)
	}
	p := resp.GetProduct()
	if p.GetProductId() != "p-1" || p.GetBrandName() != "Crocin" || p.GetGenericName() != "Paracetamol" ||
		p.GetMoleculeId() != "mol-1" || p.GetMoleculeName() != "Paracetamol" || p.GetStrength() != "500mg" ||
		p.GetDosageForm() != catalogv1.DosageForm_DOSAGE_FORM_TABLET || p.GetPackSize() != 10 ||
		p.GetManufacturer() != "GSK" || !p.GetRequiresPrescription() || p.GetControlled() {
		t.Fatalf("product = %v", p)
	}
}

func TestErrorsMapToStatusCodes(t *testing.T) {
	ctx := context.Background()
	t.Run("unknown product is NOT_FOUND", func(t *testing.T) {
		_, err := newServer(&memoryRepository{}).GetProduct(ctx, &catalogv1.GetProductRequest{ProductId: "nope"})
		wantCode(t, err, codes.NotFound)
	})
	t.Run("blank product ID is INVALID_ARGUMENT", func(t *testing.T) {
		_, err := newServer(&memoryRepository{}).GetProduct(ctx, &catalogv1.GetProductRequest{})
		wantCode(t, err, codes.InvalidArgument)
	})
	t.Run("blank query is INVALID_ARGUMENT", func(t *testing.T) {
		_, err := newServer(&memoryRepository{}).ResolveName(ctx, &catalogv1.ResolveNameRequest{Query: " "})
		wantCode(t, err, codes.InvalidArgument)
	})
	t.Run("blank molecule is INVALID_ARGUMENT", func(t *testing.T) {
		_, err := newServer(&memoryRepository{}).ListProductsByMolecule(ctx, &catalogv1.ListProductsByMoleculeRequest{})
		wantCode(t, err, codes.InvalidArgument)
	})
	t.Run("database failure is INTERNAL and hides the cause", func(t *testing.T) {
		_, err := newServer(&memoryRepository{err: errors.New("password=secret")}).GetProduct(ctx,
			&catalogv1.GetProductRequest{ProductId: "p-1"})
		wantCode(t, err, codes.Internal)
		if strings.Contains(err.Error(), "secret") {
			t.Fatalf("error %q leaks the internal cause", err)
		}
	})
}

func TestEveryRPCRejectsANilRequest(t *testing.T) {
	s := newServer(&memoryRepository{})
	ctx := context.Background()
	_, err1 := s.ResolveName(ctx, nil)
	_, err2 := s.GetProduct(ctx, nil)
	_, err3 := s.BatchGetProducts(ctx, nil)
	_, err4 := s.ListProductsByMolecule(ctx, nil)
	_, err5 := s.CreateProduct(ctx, nil)
	for i, err := range []error{err1, err2, err3, err4, err5} {
		if status.Code(err) != codes.InvalidArgument {
			t.Errorf("rpc %d: code = %s, want InvalidArgument", i+1, status.Code(err))
		}
	}
}

func TestEveryRPCFailsWithoutAService(t *testing.T) {
	ctx := context.Background()
	for name, s := range map[string]*Server{"nil server": nil, "no service": NewServer(nil)} {
		t.Run(name, func(t *testing.T) {
			_, err1 := s.ResolveName(ctx, &catalogv1.ResolveNameRequest{Query: "x"})
			_, err2 := s.GetProduct(ctx, &catalogv1.GetProductRequest{ProductId: "x"})
			_, err3 := s.BatchGetProducts(ctx, &catalogv1.BatchGetProductsRequest{})
			_, err4 := s.ListProductsByMolecule(ctx, &catalogv1.ListProductsByMoleculeRequest{MoleculeId: "x"})
			_, err5 := s.CreateProduct(ctx, &catalogv1.CreateProductRequest{})
			for i, err := range []error{err1, err2, err3, err4, err5} {
				if status.Code(err) != codes.Internal {
					t.Errorf("rpc %d: code = %s, want Internal", i+1, status.Code(err))
				}
			}
		})
	}
}

func TestResolveNameBatchGetAndListByMolecule(t *testing.T) {
	s := newServer(&memoryRepository{})
	ctx := context.Background()

	resolved, err := s.ResolveName(ctx, &catalogv1.ResolveNameRequest{Query: "croc", Limit: 5})
	if err != nil || len(resolved.GetProducts()) != 1 || resolved.GetProducts()[0].GetProductId() != "p-1" {
		t.Fatalf("ResolveName = %v, %v", resolved, err)
	}

	batch, err := s.BatchGetProducts(ctx, &catalogv1.BatchGetProductsRequest{ProductIds: []string{"p-1", "missing", "p-1"}})
	if err != nil || len(batch.GetProducts()) != 1 {
		t.Fatalf("BatchGetProducts = %v, %v; want only p-1 once", batch, err)
	}

	empty, err := s.BatchGetProducts(ctx, &catalogv1.BatchGetProductsRequest{})
	if err != nil || empty.GetProducts() == nil || len(empty.GetProducts()) != 0 {
		t.Fatalf("BatchGetProducts(no IDs) = %v, %v; want an empty list", empty, err)
	}

	listed, err := s.ListProductsByMolecule(ctx, &catalogv1.ListProductsByMoleculeRequest{MoleculeId: "mol-1", Strength: "500 MG"})
	if err != nil || len(listed.GetProducts()) != 1 {
		t.Fatalf("ListProductsByMolecule = %v, %v; want p-1 (strength is normalized)", listed, err)
	}
}

func TestCreateProductSendsTheRequestToTheRepository(t *testing.T) {
	repo := &memoryRepository{}
	resp, err := newServer(repo).CreateProduct(context.Background(), &catalogv1.CreateProductRequest{
		BrandName: "Calpol", GenericName: "Paracetamol", MoleculeId: "mol-1", MoleculeName: "Paracetamol",
		Strength: "120 mg/5 ml", DosageForm: catalogv1.DosageForm_DOSAGE_FORM_SYRUP, PackSize: 1,
		Manufacturer: "GSK", RequiresPrescription: false, Controlled: true,
	})
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}
	if resp.GetProduct().GetProductId() != "new-id" || resp.GetProduct().GetDosageForm() != catalogv1.DosageForm_DOSAGE_FORM_SYRUP {
		t.Fatalf("response = %v", resp)
	}
	if len(repo.created) != 1 {
		t.Fatalf("created = %d products, want 1", len(repo.created))
	}
	got := repo.created[0]
	if got.Strength != "120mg/5ml" || got.DosageForm != domain.DosageFormSyrup || !got.Controlled || got.RequiresPrescription {
		t.Fatalf("stored product = %+v", got)
	}
}

func TestCreateProductRejectsAnUnspecifiedDosageForm(t *testing.T) {
	repo := &memoryRepository{}
	_, err := newServer(repo).CreateProduct(context.Background(), &catalogv1.CreateProductRequest{
		BrandName: "Calpol", GenericName: "Paracetamol", MoleculeId: "mol-1", MoleculeName: "Paracetamol",
		Strength: "120mg", PackSize: 1,
	})
	wantCode(t, err, codes.InvalidArgument)
	if len(repo.created) != 0 {
		t.Fatal("an invalid product reached the repository")
	}
}

// Every domain dosage form must survive the trip to proto and back.
func TestDosageFormRoundTrip(t *testing.T) {
	forms := map[domain.DosageForm]catalogv1.DosageForm{
		domain.DosageFormTablet:     catalogv1.DosageForm_DOSAGE_FORM_TABLET,
		domain.DosageFormCapsule:    catalogv1.DosageForm_DOSAGE_FORM_CAPSULE,
		domain.DosageFormSyrup:      catalogv1.DosageForm_DOSAGE_FORM_SYRUP,
		domain.DosageFormSuspension: catalogv1.DosageForm_DOSAGE_FORM_SUSPENSION,
		domain.DosageFormInjection:  catalogv1.DosageForm_DOSAGE_FORM_INJECTION,
		domain.DosageFormCream:      catalogv1.DosageForm_DOSAGE_FORM_CREAM,
		domain.DosageFormOintment:   catalogv1.DosageForm_DOSAGE_FORM_OINTMENT,
		domain.DosageFormDrops:      catalogv1.DosageForm_DOSAGE_FORM_DROPS,
		domain.DosageFormInhaler:    catalogv1.DosageForm_DOSAGE_FORM_INHALER,
		domain.DosageFormOther:      catalogv1.DosageForm_DOSAGE_FORM_OTHER,
	}
	for d, p := range forms {
		if got := dosageFormToProto(d); got != p {
			t.Errorf("dosageFormToProto(%q) = %s, want %s", d, got, p)
		}
		if got := dosageFormFromProto(p); got != d {
			t.Errorf("dosageFormFromProto(%s) = %q, want %q", p, got, d)
		}
	}
	if got := dosageFormToProto("DOSAGE_FORM_PATCH"); got != catalogv1.DosageForm_DOSAGE_FORM_UNSPECIFIED {
		t.Errorf("unknown domain form = %s, want UNSPECIFIED", got)
	}
	if got := dosageFormFromProto(catalogv1.DosageForm(999)); got != domain.DosageFormUnspecified {
		t.Errorf("unknown proto form = %q, want unspecified", got)
	}
}
