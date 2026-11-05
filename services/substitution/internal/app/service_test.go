package app

import (
	"context"
	"errors"
	"testing"

	"github.com/Swiftmeds-bht/Swiftmeds/services/substitution/internal/domain"
)

type fakeCatalog struct {
	product       domain.Product
	candidates    []domain.Product
	getErr        error
	listErr       error
	gotProductID  string
	gotMoleculeID string
	gotStrength   string
	listCalls     int
}

func (f *fakeCatalog) GetProduct(_ context.Context, productID string) (domain.Product, error) {
	f.gotProductID = productID
	return f.product, f.getErr
}

func (f *fakeCatalog) ListProductsByMolecule(
	_ context.Context,
	moleculeID, strength string,
) ([]domain.Product, error) {
	f.listCalls++
	f.gotMoleculeID = moleculeID
	f.gotStrength = strength
	return f.candidates, f.listErr
}

func TestGetSubstitutesRejectsEmptyProductID(t *testing.T) {
	catalog := &fakeCatalog{}
	service := NewService(catalog)

	_, err := service.GetSubstitutes(context.Background(), "  ", 10)
	if !errors.Is(err, domain.ErrMissingProductID) {
		t.Fatalf("error = %v, want ErrMissingProductID", err)
	}
	if catalog.gotProductID != "" {
		t.Fatalf("catalog was called with product ID %q", catalog.gotProductID)
	}
}

func TestGetSubstitutesClassifiesFiltersAndSortsCandidates(t *testing.T) {
	catalog := &fakeCatalog{
		product: domain.Product{
			ProductID:  "source",
			MoleculeID: "molecule-1",
			Strength:   "500 mg",
			DosageForm: "DOSAGE_FORM_TABLET",
			BrandName:  "Source",
		},
		candidates: []domain.Product{
			{
				ProductID:  "exact-z",
				MoleculeID: "molecule-1",
				BrandName:  "Zeta",
				Strength:   "500mg",
				DosageForm: "DOSAGE_FORM_TABLET",
			},
			{
				ProductID:  "check-a",
				MoleculeID: "molecule-1",
				BrandName:  "Check",
				Strength:   "250mg",
				DosageForm: "DOSAGE_FORM_TABLET",
			},
			{
				ProductID:  "exact-a",
				MoleculeID: "molecule-1",
				BrandName:  "Alpha",
				Strength:   "500mg",
				DosageForm: "DOSAGE_FORM_TABLET",
			},
			{
				ProductID:  "EXACT-A",
				MoleculeID: "molecule-1",
				BrandName:  "Duplicate",
				Strength:   "500mg",
				DosageForm: "DOSAGE_FORM_TABLET",
			},
			{
				ProductID:  "other-molecule",
				MoleculeID: "molecule-2",
				BrandName:  "Other",
				Strength:   "500mg",
				DosageForm: "DOSAGE_FORM_TABLET",
			},
			{
				ProductID:  "controlled",
				MoleculeID: "molecule-1",
				BrandName:  "Controlled",
				Strength:   "500mg",
				DosageForm: "DOSAGE_FORM_TABLET",
				Controlled: true,
			},
		},
	}
	service := NewService(catalog)

	result, err := service.GetSubstitutes(context.Background(), " source ", 10)
	if err != nil {
		t.Fatalf("GetSubstitutes returned error: %v", err)
	}
	if result.MoleculeID != "molecule-1" {
		t.Fatalf("molecule ID = %q, want molecule-1", result.MoleculeID)
	}
	if catalog.gotProductID != "source" {
		t.Fatalf("source product ID = %q, want trimmed ID", catalog.gotProductID)
	}
	if catalog.gotMoleculeID != "molecule-1" || catalog.gotStrength != "" {
		t.Fatalf("catalog list args = (%q, %q), want (molecule-1, empty strength)",
			catalog.gotMoleculeID, catalog.gotStrength)
	}

	got := result.Substitutes
	if len(got) != 3 {
		t.Fatalf("got %d substitutes, want 3: %+v", len(got), got)
	}

	wantIDs := []string{"exact-a", "exact-z", "check-a"}
	wantTypes := []domain.MatchType{
		domain.MatchTypeExact,
		domain.MatchTypeExact,
		domain.MatchTypeNeedsPharmacistCheck,
	}
	for i := range wantIDs {
		if got[i].ProductID != wantIDs[i] {
			t.Errorf("substitute %d ID = %q, want %q", i, got[i].ProductID, wantIDs[i])
		}
		if got[i].MatchType != wantTypes[i] {
			t.Errorf("substitute %d match type = %q, want %q", i, got[i].MatchType, wantTypes[i])
		}
	}
}

func TestGetSubstitutesSkipsLookupForControlledSource(t *testing.T) {
	catalog := &fakeCatalog{
		product: domain.Product{
			ProductID:  "source",
			MoleculeID: "molecule-1",
			Controlled: true,
		},
	}
	service := NewService(catalog)

	result, err := service.GetSubstitutes(context.Background(), "source", 10)
	if err != nil {
		t.Fatalf("GetSubstitutes returned error: %v", err)
	}
	if len(result.Substitutes) != 0 {
		t.Fatalf("got substitutes for controlled source: %+v", result.Substitutes)
	}
	if catalog.listCalls != 0 {
		t.Fatalf("catalog list called %d times, want 0", catalog.listCalls)
	}
}

func TestGetSubstitutesPropagatesCatalogErrors(t *testing.T) {
	upstreamErr := errors.New("catalog unavailable")
	catalog := &fakeCatalog{getErr: upstreamErr}
	service := NewService(catalog)

	_, err := service.GetSubstitutes(context.Background(), "source", 10)
	if !errors.Is(err, upstreamErr) {
		t.Fatalf("error = %v, want wrapped catalog error", err)
	}
}
