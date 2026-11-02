package domain

import "testing"

func TestBuildSubstitute(t *testing.T) {
	source := Product{
		ProductID:  "source",
		MoleculeID: "molecule-1",
		Strength:   "500 mg",
		DosageForm: "DOSAGE_FORM_TABLET",
	}

	tests := []struct {
		name      string
		source    Product
		candidate Product
		wantOK    bool
		wantMatch MatchType
	}{
		{
			name:   "same molecule strength and form is exact",
			source: source,
			candidate: Product{
				ProductID:  "candidate",
				MoleculeID: "molecule-1",
				BrandName:  "Other Brand",
				Strength:   "500mg",
				DosageForm: "DOSAGE_FORM_TABLET",
			},
			wantOK:    true,
			wantMatch: MatchTypeExact,
		},
		{
			name:   "different strength needs pharmacist check",
			source: source,
			candidate: Product{
				ProductID:  "candidate",
				MoleculeID: "molecule-1",
				Strength:   "250mg",
				DosageForm: "DOSAGE_FORM_TABLET",
			},
			wantOK:    true,
			wantMatch: MatchTypeNeedsPharmacistCheck,
		},
		{
			name:   "different form needs pharmacist check",
			source: source,
			candidate: Product{
				ProductID:  "candidate",
				MoleculeID: "molecule-1",
				Strength:   "500mg",
				DosageForm: "DOSAGE_FORM_CAPSULE",
			},
			wantOK:    true,
			wantMatch: MatchTypeNeedsPharmacistCheck,
		},
		{
			name:   "different molecule is excluded",
			source: source,
			candidate: Product{
				ProductID:  "candidate",
				MoleculeID: "molecule-2",
				Strength:   "500mg",
				DosageForm: "DOSAGE_FORM_TABLET",
			},
			wantOK: false,
		},
		{
			name:   "controlled candidate is excluded",
			source: source,
			candidate: Product{
				ProductID:  "candidate",
				MoleculeID: "molecule-1",
				Strength:   "500mg",
				DosageForm: "DOSAGE_FORM_TABLET",
				Controlled: true,
			},
			wantOK: false,
		},
		{
			name: "controlled source has no suggestions",
			source: Product{
				ProductID:  "source",
				MoleculeID: "molecule-1",
				Strength:   "500mg",
				DosageForm: "DOSAGE_FORM_TABLET",
				Controlled: true,
			},
			candidate: Product{
				ProductID:  "candidate",
				MoleculeID: "molecule-1",
				Strength:   "500mg",
				DosageForm: "DOSAGE_FORM_TABLET",
			},
			wantOK: false,
		},
		{
			name:   "source product is excluded",
			source: source,
			candidate: Product{
				ProductID:  "source",
				MoleculeID: "molecule-1",
				Strength:   "500mg",
				DosageForm: "DOSAGE_FORM_TABLET",
			},
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := BuildSubstitute(tt.source, tt.candidate)
			if ok != tt.wantOK {
				t.Fatalf("BuildSubstitute ok = %v, want %v", ok, tt.wantOK)
			}
			if ok && got.MatchType != tt.wantMatch {
				t.Errorf("match type = %q, want %q", got.MatchType, tt.wantMatch)
			}
		})
	}
}
