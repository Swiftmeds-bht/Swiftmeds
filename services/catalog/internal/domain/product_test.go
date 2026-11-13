package domain

import (
	"errors"
	"testing"
)

func validInput() ProductInput {
	return ProductInput{
		BrandName:            "Crocin",
		GenericName:          "Paracetamol",
		MoleculeID:           "mol-paracetamol",
		MoleculeName:         "Paracetamol",
		Strength:             "500mg",
		DosageForm:           DosageFormTablet,
		PackSize:             10,
		Manufacturer:         "GSK",
		RequiresPrescription: true,
		Controlled:           true,
	}
}

func TestNewProductKeepsEveryField(t *testing.T) {
	got, err := NewProduct(validInput())
	if err != nil {
		t.Fatalf("NewProduct: %v", err)
	}
	want := Product{
		BrandName:            "Crocin",
		GenericName:          "Paracetamol",
		MoleculeID:           "mol-paracetamol",
		MoleculeName:         "Paracetamol",
		Strength:             "500mg",
		DosageForm:           DosageFormTablet,
		PackSize:             10,
		Manufacturer:         "GSK",
		RequiresPrescription: true,
		Controlled:           true,
	}
	if got != want {
		t.Fatalf("product = %+v\nwant      %+v", got, want)
	}
	if got.ProductID != "" {
		t.Fatalf("product ID = %q, want empty (the repository assigns it)", got.ProductID)
	}
}

func TestNewProductTrimsTextAndNormalizesStrength(t *testing.T) {
	in := validInput()
	in.BrandName = "  Crocin\t"
	in.GenericName = " Paracetamol "
	in.MoleculeID = " mol-paracetamol "
	in.MoleculeName = "\nParacetamol "
	in.Manufacturer = "  GSK  "
	in.Strength = " 2.5 MG / 5 mL "

	got, err := NewProduct(in)
	if err != nil {
		t.Fatalf("NewProduct: %v", err)
	}
	if got.BrandName != "Crocin" || got.GenericName != "Paracetamol" ||
		got.MoleculeID != "mol-paracetamol" || got.MoleculeName != "Paracetamol" ||
		got.Manufacturer != "GSK" {
		t.Fatalf("text fields were not trimmed: %+v", got)
	}
	if got.Strength != "2.5mg/5ml" {
		t.Fatalf("strength = %q, want %q", got.Strength, "2.5mg/5ml")
	}
}

func TestNewProductRejectsMissingOrInvalidFields(t *testing.T) {
	tests := []struct {
		name    string
		change  func(*ProductInput)
		wantErr error
	}{
		{"blank brand", func(in *ProductInput) { in.BrandName = "   " }, ErrMissingBrandName},
		{"blank generic name", func(in *ProductInput) { in.GenericName = "" }, ErrMissingGenericName},
		{"blank molecule ID", func(in *ProductInput) { in.MoleculeID = "\t" }, ErrMissingMoleculeID},
		{"blank molecule name", func(in *ProductInput) { in.MoleculeName = "" }, ErrMissingMoleculeName},
		{"strength of only spaces", func(in *ProductInput) { in.Strength = " \t " }, ErrMissingStrength},
		{"unspecified dosage form", func(in *ProductInput) { in.DosageForm = DosageFormUnspecified }, ErrInvalidDosageForm},
		{"unknown dosage form", func(in *ProductInput) { in.DosageForm = "DOSAGE_FORM_PATCH" }, ErrInvalidDosageForm},
		{"lower-case dosage form", func(in *ProductInput) { in.DosageForm = "dosage_form_tablet" }, ErrInvalidDosageForm},
		{"pack size zero", func(in *ProductInput) { in.PackSize = 0 }, ErrInvalidPackSize},
		{"negative pack size", func(in *ProductInput) { in.PackSize = -3 }, ErrInvalidPackSize},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := validInput()
			tt.change(&in)
			got, err := NewProduct(in)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
			if got != (Product{}) {
				t.Fatalf("product on error = %+v, want zero value", got)
			}
		})
	}
}

func TestNewProductAcceptsPackSizeOneAndNoManufacturer(t *testing.T) {
	in := validInput()
	in.PackSize = 1
	in.Manufacturer = ""
	got, err := NewProduct(in)
	if err != nil {
		t.Fatalf("NewProduct: %v", err)
	}
	if got.PackSize != 1 || got.Manufacturer != "" {
		t.Fatalf("product = %+v, want pack size 1 and no manufacturer", got)
	}
}

// The first failing check wins, so a caller always gets one clear error.
func TestNewProductReportsTheFirstProblem(t *testing.T) {
	_, err := NewProduct(ProductInput{})
	if !errors.Is(err, ErrMissingBrandName) {
		t.Fatalf("empty input error = %v, want ErrMissingBrandName", err)
	}
}

func TestDosageFormValid(t *testing.T) {
	valid := []DosageForm{
		DosageFormTablet, DosageFormCapsule, DosageFormSyrup, DosageFormSuspension,
		DosageFormInjection, DosageFormCream, DosageFormOintment, DosageFormDrops,
		DosageFormInhaler, DosageFormOther,
	}
	for _, f := range valid {
		if !f.Valid() {
			t.Errorf("%q.Valid() = false, want true", f)
		}
	}
	for _, f := range []DosageForm{DosageFormUnspecified, "TABLET", "DOSAGE_FORM_UNSPECIFIED", " DOSAGE_FORM_TABLET"} {
		if f.Valid() {
			t.Errorf("%q.Valid() = true, want false", f)
		}
	}
}
