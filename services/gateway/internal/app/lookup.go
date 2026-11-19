package app

import (
	"log/slog"
	"net/http"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	catalogv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/catalog/v1"
	pharmacyregistryv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/pharmacy-registry/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/gateway/internal/ports"
)

// Names change rarely: the apps and the browser may reuse an answer for a while.
const lookupCacheControl = "public, max-age=300"

// productHandler is getProduct (docs/api/openapi.yaml): one medicine by ID,
// public, so both apps can show names instead of IDs (SCRUM-138). The answer is
// the contract's Product; the Catalog's internal "controlled" flag is left out.
func productHandler(client ports.ProductLookup, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("productId")
		ctx, cancel := callContext(r)
		defer cancel()
		resp, err := client.GetProduct(ctx, &catalogv1.GetProductRequest{ProductId: id})
		if status.Code(err) == codes.NotFound || (err == nil && resp.GetProduct() == nil) {
			writeError(w, http.StatusNotFound, "medicine not found")
			return
		}
		if err != nil {
			writeDownstreamError(w, logger, r, err)
			return
		}
		product := proto.Clone(resp.GetProduct()).(*catalogv1.Product)
		product.Controlled = false // not in the public contract
		w.Header().Set("Cache-Control", lookupCacheControl)
		writeProtoStatus(w, http.StatusOK, product)
	}
}

// pharmacyHandler is getPharmacy (docs/api/openapi.yaml): one pharmacy by ID,
// public, as shown on the patient app's pharmacy page and the dashboard header.
// The contract keeps ownerUserId and licenseNumber for the pharmacy's own staff
// and administrators; this route reads no token, so it never sends either.
func pharmacyHandler(client ports.PharmacyRegistryClient, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("pharmacyId")
		ctx, cancel := callContext(r)
		defer cancel()
		resp, err := client.GetPharmacy(ctx, &pharmacyregistryv1.GetPharmacyRequest{PharmacyId: id})
		if status.Code(err) == codes.NotFound || (err == nil && resp.GetPharmacy() == nil) {
			writeError(w, http.StatusNotFound, "pharmacy not found")
			return
		}
		if err != nil {
			writeDownstreamError(w, logger, r, err)
			return
		}
		pharmacy := proto.Clone(resp.GetPharmacy()).(*pharmacyregistryv1.Pharmacy)
		pharmacy.OwnerUserId = ""
		pharmacy.LicenseNumber = ""
		w.Header().Set("Cache-Control", lookupCacheControl)
		writeProtoStatus(w, http.StatusOK, pharmacy)
	}
}
