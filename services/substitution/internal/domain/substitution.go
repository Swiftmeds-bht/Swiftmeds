package domain

import "strings"

// Product contains the Catalog fields needed to decide whether to suggest a substitute.
type Product struct {
	ProductID            string
	MoleculeID           string
	BrandName            string
	GenericName          string
	Strength             string
	DosageForm           string
	RequiresPrescription bool
	Controlled           bool
}

// MatchType describes the review needed for a suggested product.
type MatchType string

const (
	MatchTypeExact                MatchType = "EXACT"
	MatchTypeNeedsPharmacistCheck MatchType = "NEEDS_PHARMACIST_CHECK"
)

// Substitute is a same-molecule product suggested for the source product.
type Substitute struct {
	ProductID   string
	BrandName   string
	GenericName string
	Strength    string
	MatchType   MatchType
}

// BuildSubstitute excludes the source product, controlled products, and products
// from another molecule. Strength or dosage-form differences require pharmacist review.
func BuildSubstitute(source, candidate Product) (Substitute, bool) {
	sourceID := strings.TrimSpace(source.ProductID)
	candidateID := strings.TrimSpace(candidate.ProductID)

	if source.Controlled || candidate.Controlled ||
		sourceID == "" || candidateID == "" ||
		strings.EqualFold(sourceID, candidateID) ||
		strings.TrimSpace(source.MoleculeID) == "" ||
		!strings.EqualFold(strings.TrimSpace(source.MoleculeID), strings.TrimSpace(candidate.MoleculeID)) {
		return Substitute{}, false
	}

	matchType := MatchTypeNeedsPharmacistCheck
	if normalize(source.Strength) == normalize(candidate.Strength) &&
		normalize(source.DosageForm) == normalize(candidate.DosageForm) {
		matchType = MatchTypeExact
	}

	return Substitute{
		ProductID:   candidateID,
		BrandName:   strings.TrimSpace(candidate.BrandName),
		GenericName: strings.TrimSpace(candidate.GenericName),
		Strength:    strings.TrimSpace(candidate.Strength),
		MatchType:   matchType,
	}, true
}

func normalize(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(value), ""))
}
