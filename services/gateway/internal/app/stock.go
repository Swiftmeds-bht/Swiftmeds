package app

import (
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"strings"

	authv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/auth/v1"
	commonv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/common/v1"
	inventoryv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/inventory/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/gateway/internal/ports"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type setStockBody struct {
	// A pointer so a missing field is rejected instead of read as zero.
	QuantityOnHand *int64 `json:"quantityOnHand"`
}

// stockHandler serves the pharmacy dashboard's stock list and stock edit
// (SCRUM-58). The Auth middleware already limits /api/v1/pharmacies/* to
// pharmacy staff and admins; this handler also requires the pharmacy in the
// path to be the caller's own pharmacy.
func stockHandler(client ports.InventoryClient, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		identity := identityFrom(r)
		if !ownPharmacy(w, r, identity) {
			return
		}
		pharmacyID := r.PathValue("pharmacyId")

		switch {
		case r.Method == http.MethodGet && r.PathValue("productId") == "":
			page, ok := pageRequest(r)
			if !ok {
				writeError(w, http.StatusBadRequest, "invalid stock list parameters")
				return
			}
			ctx, cancel := callContext(r)
			defer cancel()
			resp, err := client.ListPharmacyStock(ctx, &inventoryv1.ListPharmacyStockRequest{PharmacyId: pharmacyID, Page: page})
			if err != nil {
				writeDownstreamError(w, logger, r, err)
				return
			}
			writeProtoStatus(w, http.StatusOK, resp)
		case r.Method == http.MethodPut && r.PathValue("productId") != "":
			// Only the pharmacy's own staff record stock changes.
			if identity.Role != authv1.Role_ROLE_PHARMACY_STAFF {
				writeError(w, http.StatusForbidden, "forbidden")
				return
			}
			productID := strings.TrimSpace(r.PathValue("productId"))
			var body setStockBody
			if !decodeBody(w, r, &body) {
				return
			}
			if productID == "" || body.QuantityOnHand == nil || *body.QuantityOnHand < 0 || *body.QuantityOnHand > math.MaxInt32 {
				writeError(w, http.StatusBadRequest, "quantityOnHand must be a whole number of zero or more")
				return
			}
			ctx, cancel := callContext(r)
			defer cancel()
			resp, err := client.SetStock(ctx, &inventoryv1.SetStockRequest{
				PharmacyId:      pharmacyID,
				ProductId:       productID,
				QuantityOnHand:  int32(*body.QuantityOnHand),
				UpdatedByUserId: identity.UserID,
			})
			if status.Code(err) == codes.FailedPrecondition {
				// Inventory refuses to drop on-hand stock below what active holds reserve.
				writeError(w, http.StatusBadRequest, "quantity on hand cannot be lower than the quantity reserved by active holds")
				return
			}
			if err != nil {
				writeDownstreamError(w, logger, r, err)
				return
			}
			writeProtoStatus(w, http.StatusOK, resp.GetStock())
		default:
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	}
}

// ownPharmacy writes 403 unless the caller is staff or an admin of the pharmacy
// named in the path.
func ownPharmacy(w http.ResponseWriter, r *http.Request, identity requestIdentity) bool {
	if !pharmacyOnly(w, identity) {
		return false
	}
	if identity.PharmacyID == "" || r.PathValue("pharmacyId") != identity.PharmacyID {
		writeError(w, http.StatusForbidden, "forbidden")
		return false
	}
	return true
}

// pageRequest reads the optional pageSize (0-100) and pageToken query values.
func pageRequest(r *http.Request) (*commonv1.PageRequest, bool) {
	page := &commonv1.PageRequest{PageToken: r.URL.Query().Get("pageToken")}
	if value := r.URL.Query().Get("pageSize"); value != "" {
		size, err := strconv.Atoi(value)
		if err != nil || size < 0 || size > 100 {
			return nil, false
		}
		page.PageSize = int32(size)
	}
	return page, true
}
