package grpc

import (
	"context"
	"errors"

	catalogv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/catalog/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/catalog/internal/app"
	"github.com/Swiftmeds-bht/Swiftmeds/services/catalog/internal/domain"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Server struct {
	catalogv1.UnimplementedCatalogServiceServer
	service *app.Service
}

var _ catalogv1.CatalogServiceServer = (*Server)(nil)

func NewServer(service *app.Service) *Server {
	return &Server{service: service}
}

func (s *Server) ResolveName(
	ctx context.Context,
	request *catalogv1.ResolveNameRequest,
) (*catalogv1.ResolveNameResponse, error) {
	if err := s.checkService(); err != nil {
		return nil, err
	}
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}

	products, err := s.service.ResolveName(ctx, request.GetQuery(), int(request.GetLimit()))
	if err != nil {
		return nil, grpcError(err)
	}
	return &catalogv1.ResolveNameResponse{Products: productsToProto(products)}, nil
}

func (s *Server) GetProduct(
	ctx context.Context,
	request *catalogv1.GetProductRequest,
) (*catalogv1.GetProductResponse, error) {
	if err := s.checkService(); err != nil {
		return nil, err
	}
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}

	product, err := s.service.GetProduct(ctx, request.GetProductId())
	if err != nil {
		return nil, grpcError(err)
	}
	return &catalogv1.GetProductResponse{Product: productToProto(product)}, nil
}

func (s *Server) BatchGetProducts(
	ctx context.Context,
	request *catalogv1.BatchGetProductsRequest,
) (*catalogv1.BatchGetProductsResponse, error) {
	if err := s.checkService(); err != nil {
		return nil, err
	}
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}

	products, err := s.service.BatchGetProducts(ctx, request.GetProductIds())
	if err != nil {
		return nil, grpcError(err)
	}
	return &catalogv1.BatchGetProductsResponse{Products: productsToProto(products)}, nil
}

func (s *Server) ListProductsByMolecule(
	ctx context.Context,
	request *catalogv1.ListProductsByMoleculeRequest,
) (*catalogv1.ListProductsByMoleculeResponse, error) {
	if err := s.checkService(); err != nil {
		return nil, err
	}
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}

	products, err := s.service.ListProductsByMolecule(
		ctx,
		request.GetMoleculeId(),
		request.GetStrength(),
	)
	if err != nil {
		return nil, grpcError(err)
	}
	return &catalogv1.ListProductsByMoleculeResponse{Products: productsToProto(products)}, nil
}

func (s *Server) CreateProduct(
	ctx context.Context,
	request *catalogv1.CreateProductRequest,
) (*catalogv1.CreateProductResponse, error) {
	if err := s.checkService(); err != nil {
		return nil, err
	}
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}

	product, err := s.service.CreateProduct(ctx, domain.ProductInput{
		BrandName:            request.GetBrandName(),
		GenericName:          request.GetGenericName(),
		MoleculeID:           request.GetMoleculeId(),
		MoleculeName:         request.GetMoleculeName(),
		Strength:             request.GetStrength(),
		DosageForm:           dosageFormFromProto(request.GetDosageForm()),
		PackSize:             request.GetPackSize(),
		Manufacturer:         request.GetManufacturer(),
		RequiresPrescription: request.GetRequiresPrescription(),
		Controlled:           request.GetControlled(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	return &catalogv1.CreateProductResponse{Product: productToProto(product)}, nil
}

func (s *Server) checkService() error {
	if s == nil || s.service == nil {
		return status.Error(codes.Internal, "catalog service is unavailable")
	}
	return nil
}

func grpcError(err error) error {
	switch {
	case errors.Is(err, domain.ErrProductNotFound):
		return status.Error(codes.NotFound, err.Error())
	case isValidationError(err):
		return status.Error(codes.InvalidArgument, err.Error())
	default:
		return status.Error(codes.Internal, "catalog operation failed")
	}
}

func isValidationError(err error) bool {
	return errors.Is(err, domain.ErrMissingProductID) ||
		errors.Is(err, domain.ErrMissingQuery) ||
		errors.Is(err, domain.ErrMissingBrandName) ||
		errors.Is(err, domain.ErrMissingGenericName) ||
		errors.Is(err, domain.ErrMissingMoleculeID) ||
		errors.Is(err, domain.ErrMissingMoleculeName) ||
		errors.Is(err, domain.ErrMissingStrength) ||
		errors.Is(err, domain.ErrInvalidDosageForm) ||
		errors.Is(err, domain.ErrInvalidPackSize)
}

func productsToProto(products []domain.Product) []*catalogv1.Product {
	result := make([]*catalogv1.Product, 0, len(products))
	for _, product := range products {
		result = append(result, productToProto(product))
	}
	return result
}

func productToProto(product domain.Product) *catalogv1.Product {
	return &catalogv1.Product{
		ProductId:            product.ProductID,
		BrandName:            product.BrandName,
		GenericName:          product.GenericName,
		MoleculeId:           product.MoleculeID,
		MoleculeName:         product.MoleculeName,
		Strength:             product.Strength,
		DosageForm:           dosageFormToProto(product.DosageForm),
		PackSize:             product.PackSize,
		Manufacturer:         product.Manufacturer,
		RequiresPrescription: product.RequiresPrescription,
		Controlled:           product.Controlled,
	}
}

func dosageFormToProto(form domain.DosageForm) catalogv1.DosageForm {
	switch form {
	case domain.DosageFormTablet:
		return catalogv1.DosageForm_DOSAGE_FORM_TABLET
	case domain.DosageFormCapsule:
		return catalogv1.DosageForm_DOSAGE_FORM_CAPSULE
	case domain.DosageFormSyrup:
		return catalogv1.DosageForm_DOSAGE_FORM_SYRUP
	case domain.DosageFormSuspension:
		return catalogv1.DosageForm_DOSAGE_FORM_SUSPENSION
	case domain.DosageFormInjection:
		return catalogv1.DosageForm_DOSAGE_FORM_INJECTION
	case domain.DosageFormCream:
		return catalogv1.DosageForm_DOSAGE_FORM_CREAM
	case domain.DosageFormOintment:
		return catalogv1.DosageForm_DOSAGE_FORM_OINTMENT
	case domain.DosageFormDrops:
		return catalogv1.DosageForm_DOSAGE_FORM_DROPS
	case domain.DosageFormInhaler:
		return catalogv1.DosageForm_DOSAGE_FORM_INHALER
	case domain.DosageFormOther:
		return catalogv1.DosageForm_DOSAGE_FORM_OTHER
	default:
		return catalogv1.DosageForm_DOSAGE_FORM_UNSPECIFIED
	}
}

func dosageFormFromProto(form catalogv1.DosageForm) domain.DosageForm {
	switch form {
	case catalogv1.DosageForm_DOSAGE_FORM_TABLET:
		return domain.DosageFormTablet
	case catalogv1.DosageForm_DOSAGE_FORM_CAPSULE:
		return domain.DosageFormCapsule
	case catalogv1.DosageForm_DOSAGE_FORM_SYRUP:
		return domain.DosageFormSyrup
	case catalogv1.DosageForm_DOSAGE_FORM_SUSPENSION:
		return domain.DosageFormSuspension
	case catalogv1.DosageForm_DOSAGE_FORM_INJECTION:
		return domain.DosageFormInjection
	case catalogv1.DosageForm_DOSAGE_FORM_CREAM:
		return domain.DosageFormCream
	case catalogv1.DosageForm_DOSAGE_FORM_OINTMENT:
		return domain.DosageFormOintment
	case catalogv1.DosageForm_DOSAGE_FORM_DROPS:
		return domain.DosageFormDrops
	case catalogv1.DosageForm_DOSAGE_FORM_INHALER:
		return domain.DosageFormInhaler
	case catalogv1.DosageForm_DOSAGE_FORM_OTHER:
		return domain.DosageFormOther
	default:
		return domain.DosageFormUnspecified
	}
}
