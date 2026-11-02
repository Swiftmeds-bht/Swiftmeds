package clients

import (
	"context"
	"net"
	"testing"

	catalogv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/catalog/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

type catalogServerStub struct {
	catalogv1.UnimplementedCatalogServiceServer

	product       *catalogv1.Product
	products      []*catalogv1.Product
	getErr        error
	listErr       error
	gotMoleculeID string
	gotStrength   string
}

func (s *catalogServerStub) GetProduct(
	_ context.Context,
	_ *catalogv1.GetProductRequest,
) (*catalogv1.GetProductResponse, error) {
	if s.getErr != nil {
		return nil, s.getErr
	}
	return &catalogv1.GetProductResponse{Product: s.product}, nil
}

func (s *catalogServerStub) ListProductsByMolecule(
	_ context.Context,
	request *catalogv1.ListProductsByMoleculeRequest,
) (*catalogv1.ListProductsByMoleculeResponse, error) {
	s.gotMoleculeID = request.GetMoleculeId()
	s.gotStrength = request.GetStrength()
	if s.listErr != nil {
		return nil, s.listErr
	}
	return &catalogv1.ListProductsByMoleculeResponse{Products: s.products}, nil
}

func newTestCatalogClient(
	t *testing.T,
	server catalogv1.CatalogServiceServer,
) *CatalogClient {
	t.Helper()

	listener := bufconn.Listen(1024 * 1024)
	grpcServer := grpc.NewServer()
	catalogv1.RegisterCatalogServiceServer(grpcServer, server)

	go func() {
		_ = grpcServer.Serve(listener)
	}()
	t.Cleanup(grpcServer.Stop)

	connection, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return listener.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("create test connection: %v", err)
	}
	t.Cleanup(func() {
		_ = connection.Close()
	})

	return NewCatalogClient(connection)
}

func TestGetProductMapsCatalogProduct(t *testing.T) {
	client := newTestCatalogClient(t, &catalogServerStub{
		product: &catalogv1.Product{
			ProductId:            "product-1",
			MoleculeId:           "molecule-1",
			BrandName:            "Demo brand",
			GenericName:          "Demo medicine",
			Strength:             "500mg",
			DosageForm:           catalogv1.DosageForm_DOSAGE_FORM_TABLET,
			RequiresPrescription: true,
		},
	})

	product, err := client.GetProduct(context.Background(), "product-1")
	if err != nil {
		t.Fatalf("GetProduct returned error: %v", err)
	}

	if product.ProductID != "product-1" ||
		product.MoleculeID != "molecule-1" ||
		product.BrandName != "Demo brand" ||
		product.GenericName != "Demo medicine" ||
		product.Strength != "500mg" ||
		product.DosageForm != "DOSAGE_FORM_TABLET" ||
		!product.RequiresPrescription ||
		product.Controlled {
		t.Fatalf("mapped product = %+v", product)
	}
}

func TestListProductsByMoleculeMapsProductsAndRequest(t *testing.T) {
	server := &catalogServerStub{
		products: []*catalogv1.Product{
			{
				ProductId:  "product-1",
				MoleculeId: "molecule-1",
				BrandName:  "Brand one",
				Strength:   "500mg",
			},
			{
				ProductId:  "product-2",
				MoleculeId: "molecule-1",
				BrandName:  "Brand two",
				Strength:   "250mg",
			},
		},
	}
	client := newTestCatalogClient(t, server)

	products, err := client.ListProductsByMolecule(
		context.Background(),
		"molecule-1",
		"",
	)
	if err != nil {
		t.Fatalf("ListProductsByMolecule returned error: %v", err)
	}
	if server.gotMoleculeID != "molecule-1" || server.gotStrength != "" {
		t.Fatalf("request args = (%q, %q), want (molecule-1, empty)",
			server.gotMoleculeID, server.gotStrength)
	}
	if len(products) != 2 {
		t.Fatalf("got %d products, want 2", len(products))
	}
	if products[0].ProductID != "product-1" || products[1].ProductID != "product-2" {
		t.Fatalf("mapped products = %+v", products)
	}
}

func TestCatalogClientPreservesGRPCErrors(t *testing.T) {
	t.Run("get product", func(t *testing.T) {
		client := newTestCatalogClient(t, &catalogServerStub{
			getErr: status.Error(codes.NotFound, "product not found"),
		})

		_, err := client.GetProduct(context.Background(), "missing")
		if got := status.Code(err); got != codes.NotFound {
			t.Fatalf("status code = %v, want NotFound (error: %v)", got, err)
		}
	})

	t.Run("list products", func(t *testing.T) {
		client := newTestCatalogClient(t, &catalogServerStub{
			listErr: status.Error(codes.Unavailable, "catalog unavailable"),
		})

		_, err := client.ListProductsByMolecule(context.Background(), "molecule-1", "")
		if got := status.Code(err); got != codes.Unavailable {
			t.Fatalf("status code = %v, want Unavailable (error: %v)", got, err)
		}
	})
}
