package app

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	authv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/auth/v1"
	commonv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/common/v1"
	pricingv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/pricing/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/gateway/internal/httpx"
	"github.com/Swiftmeds-bht/Swiftmeds/services/gateway/internal/ports"
)

const (
	// maxPriceUpdates is how many prices one request may set or read.
	maxPriceUpdates = 100
	// maxPriceChhertum is Nu. 1,000,000 a pack: far above any real price, low
	// enough to catch a typo with extra zeros.
	maxPriceChhertum = 100_000_000
)

type priceUpdateBody struct {
	ProductID string `json:"productId"`
	// Chhertum, as a string (the contract's int64 encoding) or a number.
	PriceChhertum json.RawMessage `json:"priceChhertum"`
}

type setPricesBody struct {
	Updates []priceUpdateBody `json:"updates"`
}

// priceJSON is the contract's Price: flat, unlike Pricing's StockKey.
type priceJSON struct {
	PharmacyID    string `json:"pharmacyId"`
	ProductID     string `json:"productId"`
	PriceChhertum string `json:"priceChhertum"`
	UpdatedAt     string `json:"updatedAt,omitempty"`
}

// pricesHandler is setPrices and getPrices (SCRUM-154): a pharmacy's own staff
// set the price of one or more medicines, or read the current ones, for the
// portal's Stock page. Admins of the pharmacy may read; only its staff set,
// as with stock.
func pricesHandler(client ports.PricingClient, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		identity := identityFrom(r)
		if !ownPharmacy(w, r, identity) {
			return
		}
		pharmacyID := r.PathValue("pharmacyId")
		switch r.Method {
		case http.MethodGet:
			getPrices(w, r, client, logger, pharmacyID)
		case http.MethodPut:
			if identity.Role != authv1.Role_ROLE_PHARMACY_STAFF {
				writeError(w, http.StatusForbidden, "forbidden")
				return
			}
			setPrices(w, r, client, logger, pharmacyID)
		default:
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	}
}

func setPrices(w http.ResponseWriter, r *http.Request, client ports.PricingClient, logger *slog.Logger, pharmacyID string) {
	var body setPricesBody
	if !decodeBody(w, r, &body) {
		return
	}
	if len(body.Updates) == 0 || len(body.Updates) > maxPriceUpdates {
		writeError(w, http.StatusBadRequest, "send 1 to 100 price updates")
		return
	}
	req := &pricingv1.SetPricesRequest{PharmacyId: pharmacyID}
	seen := make(map[string]bool, len(body.Updates))
	for _, u := range body.Updates {
		productID := strings.TrimSpace(u.ProductID)
		if productID == "" {
			writeError(w, http.StatusBadRequest, "productId is required")
			return
		}
		if seen[productID] {
			writeError(w, http.StatusBadRequest, "each medicine may appear only once")
			return
		}
		seen[productID] = true
		amount, ok := parseChhertum(u.PriceChhertum)
		if !ok || amount <= 0 || amount > maxPriceChhertum {
			writeError(w, http.StatusBadRequest, "priceChhertum must be a whole number of chhertum from 1 to 100000000")
			return
		}
		req.Updates = append(req.Updates, &pricingv1.PriceUpdate{ProductId: productID, PriceChhertum: amount})
	}

	ctx, cancel := callContext(r)
	defer cancel()
	resp, err := client.SetPrices(ctx, req)
	if status.Code(err) == codes.InvalidArgument {
		writeError(w, http.StatusBadRequest, status.Convert(err).Message())
		return
	}
	if err != nil {
		writeDownstreamError(w, logger, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string][]priceJSON{"prices": pricesJSON(resp.GetPrices())})
}

func getPrices(w http.ResponseWriter, r *http.Request, client ports.PricingClient, logger *slog.Logger, pharmacyID string) {
	ids := r.URL.Query()["productId"]
	if len(ids) == 0 || len(ids) > maxPriceUpdates {
		writeError(w, http.StatusBadRequest, "give 1 to 100 productId values")
		return
	}
	req := &pricingv1.BatchGetPricesRequest{}
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			writeError(w, http.StatusBadRequest, "productId must not be empty")
			return
		}
		req.Keys = append(req.Keys, &commonv1.StockKey{PharmacyId: pharmacyID, ProductId: id})
	}
	ctx, cancel := callContext(r)
	defer cancel()
	resp, err := client.BatchGetPrices(ctx, req)
	if err != nil {
		writeDownstreamError(w, logger, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string][]priceJSON{"prices": pricesJSON(resp.GetPrices())})
}

// parseChhertum reads "4500" or 4500 (whole numbers only).
func parseChhertum(raw json.RawMessage) (int64, bool) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return 0, false
	}
	text := string(raw)
	if raw[0] == '"' {
		if err := json.Unmarshal(raw, &text); err != nil {
			return 0, false
		}
	}
	n, err := strconv.ParseInt(strings.TrimSpace(text), 10, 64)
	return n, err == nil
}

func pricesJSON(prices []*pricingv1.Price) []priceJSON {
	out := make([]priceJSON, 0, len(prices))
	for _, p := range prices {
		item := priceJSON{
			PharmacyID:    p.GetKey().GetPharmacyId(),
			ProductID:     p.GetKey().GetProductId(),
			PriceChhertum: strconv.FormatInt(p.GetPriceChhertum(), 10),
		}
		if p.GetUpdatedAt() != nil {
			item.UpdatedAt = p.GetUpdatedAt().AsTime().UTC().Format(time.RFC3339)
		}
		out = append(out, item)
	}
	return out
}
