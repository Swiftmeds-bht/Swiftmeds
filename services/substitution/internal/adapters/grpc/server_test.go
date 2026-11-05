package grpc

import (
	"context"
	"errors"
	"testing"

	substitutionv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/substitution/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/substitution/internal/app"
	"github.com/Swiftmeds-bht/Swiftmeds/services/substitution/internal/domain"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type catalogStub struct {
	product    domain.Product
	candidates []domain.Product
	getErr     error
	listErr    error
}

func (s *catalogStub) GetProduct(
	_ context.Context,
	_ string,
) (domain.Product, error) {
	return s.product, s.getErr
}

func (s *catalogStub) ListProductsByMolecule(
	_ context.Context,
	_, _ string,
) ([]domain.Product, error) {
	return s.candidates, s.listErr
}

func TestGetSubstitutesMapsResults(t *testing.T) {
	handler := NewHandler(app.NewService(&catalogStub{
		product: domain.Product{
			ProductID:  "source",
			MoleculeID: "molecule-1",
			Strength:   "500mg",
			DosageForm: "DOSAGE_FORM_TABLET",
		},
		candidates: []domain.Product{
			{
				ProductID:   "exact",
				MoleculeID:  "molecule-1",
				BrandName:   "Exact brand",
				GenericName: "Medicine",
				Strength:    "500mg",
				DosageForm:  "DOSAGE_FORM_TABLET",
			},
			{
				ProductID:   "review",
				MoleculeID:  "molecule-1",
				BrandName:   "Review brand",
				GenericName: "Medicine",
				Strength:    "250mg",
				DosageForm:  "DOSAGE_FORM_TABLET",
			},
		},
	}))

	response, err := handler.GetSubstitutes(context.Background(), &substitutionv1.GetSubstitutesRequest{
		ProductId: "source",
		Limit:     10,
	})
	if err != nil {
		t.Fatalf("GetSubstitutes returned error: %v", err)
	}
	if response.GetMoleculeId() != "molecule-1" {
		t.Fatalf("molecule ID = %q, want molecule-1", response.GetMoleculeId())
	}
	if len(response.GetSubstitutes()) != 2 {
		t.Fatalf("got %d substitutes, want 2", len(response.GetSubstitutes()))
	}
	if got := response.GetSubstitutes()[0].GetMatchType(); got != substitutionv1.SubstituteMatchType_SUBSTITUTE_MATCH_TYPE_EXACT {
		t.Errorf("first match type = %v, want EXACT", got)
	}
	if got := response.GetSubstitutes()[1].GetMatchType(); got != substitutionv1.SubstituteMatchType_SUBSTITUTE_MATCH_TYPE_NEEDS_PHARMACIST_CHECK {
		t.Errorf("second match type = %v, want NEEDS_PHARMACIST_CHECK", got)
	}
}

func TestGetSubstitutesReturnsExpectedErrorCodes(t *testing.T) {
	tests := []struct {
		name    string
		handler *Handler
		request *substitutionv1.GetSubstitutesRequest
		want    codes.Code
	}{
		{
			name:    "nil request",
			handler: NewHandler(app.NewService(&catalogStub{})),
			request: nil,
			want:    codes.InvalidArgument,
		},
		{
			name:    "missing product ID",
			handler: NewHandler(app.NewService(&catalogStub{})),
			request: &substitutionv1.GetSubstitutesRequest{},
			want:    codes.InvalidArgument,
		},
		{
			name: "catalog product not found",
			handler: NewHandler(app.NewService(&catalogStub{
				getErr: status.Error(codes.NotFound, "product not found"),
			})),
			request: &substitutionv1.GetSubstitutesRequest{ProductId: "missing"},
			want:    codes.NotFound,
		},
		{
			name:    "missing service",
			handler: NewHandler(nil),
			request: &substitutionv1.GetSubstitutesRequest{ProductId: "source"},
			want:    codes.Internal,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := test.handler.GetSubstitutes(context.Background(), test.request)
			if got := status.Code(err); got != test.want {
				t.Fatalf("status code = %v, want %v (error: %v)", got, test.want, err)
			}
		})
	}
}

func TestGetSubstitutesMapsUnexpectedErrorsToInternal(t *testing.T) {
	handler := NewHandler(app.NewService(&catalogStub{
		getErr: errors.New("unexpected catalog failure"),
	}))

	_, err := handler.GetSubstitutes(context.Background(), &substitutionv1.GetSubstitutesRequest{
		ProductId: "source",
	})
	if got := status.Code(err); got != codes.Internal {
		t.Fatalf("status code = %v, want Internal (error: %v)", got, err)
	}
}
