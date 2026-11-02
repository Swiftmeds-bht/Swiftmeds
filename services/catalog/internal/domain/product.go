package domain

import (
	"errors"
	"strings"
	"unicode"
)

var (
	ErrMissingProductID    = errors.New("product ID is required")
	ErrMissingQuery        = errors.New("query is required")
	ErrProductNotFound     = errors.New("product not found")
	ErrMissingBrandName    = errors.New("brand name is required")
	ErrMissingGenericName  = errors.New("generic name is required")
	ErrMissingMoleculeID   = errors.New("molecule ID is required")
	ErrMissingMoleculeName = errors.New("molecule name is required")
	ErrMissingStrength     = errors.New("strength is required")
	ErrInvalidDosageForm   = errors.New("dosage form is invalid")
	ErrInvalidPackSize     = errors.New("pack size must be greater than zero")
)

type DosageForm string

const (
	DosageFormUnspecified DosageForm = ""
	DosageFormTablet      DosageForm = "DOSAGE_FORM_TABLET"
	DosageFormCapsule     DosageForm = "DOSAGE_FORM_CAPSULE"
	DosageFormSyrup       DosageForm = "DOSAGE_FORM_SYRUP"
	DosageFormSuspension  DosageForm = "DOSAGE_FORM_SUSPENSION"
	DosageFormInjection   DosageForm = "DOSAGE_FORM_INJECTION"
	DosageFormCream       DosageForm = "DOSAGE_FORM_CREAM"
	DosageFormOintment    DosageForm = "DOSAGE_FORM_OINTMENT"
	DosageFormDrops       DosageForm = "DOSAGE_FORM_DROPS"
	DosageFormInhaler     DosageForm = "DOSAGE_FORM_INHALER"
	DosageFormOther       DosageForm = "DOSAGE_FORM_OTHER"
)

func (f DosageForm) Valid() bool {
	switch f {
	case DosageFormTablet, DosageFormCapsule, DosageFormSyrup,
		DosageFormSuspension, DosageFormInjection, DosageFormCream,
		DosageFormOintment, DosageFormDrops, DosageFormInhaler,
		DosageFormOther:
		return true
	default:
		return false
	}
}

type Product struct {
	ProductID            string
	BrandName            string
	GenericName          string
	MoleculeID           string
	MoleculeName         string
	Strength             string
	DosageForm           DosageForm
	PackSize             int32
	Manufacturer         string
	RequiresPrescription bool
	Controlled           bool
}

type ProductInput struct {
	BrandName            string
	GenericName          string
	MoleculeID           string
	MoleculeName         string
	Strength             string
	DosageForm           DosageForm
	PackSize             int32
	Manufacturer         string
	RequiresPrescription bool
	Controlled           bool
}

func NewProduct(input ProductInput) (Product, error) {
	product := Product{
		BrandName:            strings.TrimSpace(input.BrandName),
		GenericName:          strings.TrimSpace(input.GenericName),
		MoleculeID:           strings.TrimSpace(input.MoleculeID),
		MoleculeName:         strings.TrimSpace(input.MoleculeName),
		Strength:             normalizeStrength(input.Strength),
		DosageForm:           input.DosageForm,
		PackSize:             input.PackSize,
		Manufacturer:         strings.TrimSpace(input.Manufacturer),
		RequiresPrescription: input.RequiresPrescription,
		Controlled:           input.Controlled,
	}

	switch {
	case product.BrandName == "":
		return Product{}, ErrMissingBrandName
	case product.GenericName == "":
		return Product{}, ErrMissingGenericName
	case product.MoleculeID == "":
		return Product{}, ErrMissingMoleculeID
	case product.MoleculeName == "":
		return Product{}, ErrMissingMoleculeName
	case product.Strength == "":
		return Product{}, ErrMissingStrength
	case !product.DosageForm.Valid():
		return Product{}, ErrInvalidDosageForm
	case product.PackSize <= 0:
		return Product{}, ErrInvalidPackSize
	default:
		return product, nil
	}
}

func normalizeStrength(value string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return unicode.ToLower(r)
	}, strings.TrimSpace(value))
}
