package app

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	authv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/auth/v1"
	commonv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/common/v1"
	holdsv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/holds/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/gateway/internal/httpx"
	"github.com/Swiftmeds-bht/Swiftmeds/services/gateway/internal/ports"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type holdBody struct {
	PharmacyID string `json:"pharmacyId"`
	ProductID  string `json:"productId"`
	Quantity   int32  `json:"quantity"`
	Reason     string `json:"reason"`
	PickupCode string `json:"pickupCode"`
}

func holdsHandler(client ports.HoldsClient, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		identity := identityFrom(r)
		switch {
		case r.Method == http.MethodPost && r.PathValue("holdId") == "" && r.URL.Path == "/api/v1/holds":
			if !patientOnly(w, identity) {
				return
			}
			var body holdBody
			if !decodeBody(w, r, &body) || strings.TrimSpace(body.PharmacyID) == "" || strings.TrimSpace(body.ProductID) == "" || body.Quantity < 1 || body.Quantity > 5 {
				writeError(w, http.StatusBadRequest, "invalid hold request")
				return
			}
			resp, err := client.CreateHold(callContext(r), &holdsv1.CreateHoldRequest{
				UserId: identity.UserID, PharmacyId: body.PharmacyID, ProductId: body.ProductID, Quantity: body.Quantity,
			})
			if err != nil {
				writeDownstreamError(w, logger, r, err)
				return
			}
			writeProtoStatus(w, http.StatusAccepted, resp.GetHold())
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/holds":
			req := &holdsv1.ListHoldsRequest{Page: &commonv1.PageRequest{}}
			if identity.Role == authv1.Role_ROLE_PATIENT {
				req.UserId = identity.UserID
			} else if identity.Role == authv1.Role_ROLE_PHARMACY_STAFF || identity.Role == authv1.Role_ROLE_ADMIN {
				if identity.PharmacyID == "" {
					writeError(w, http.StatusForbidden, "forbidden")
					return
				}
				req.PharmacyId = identity.PharmacyID
			} else {
				writeError(w, http.StatusForbidden, "forbidden")
				return
			}
			if err := fillHoldListQuery(r, req); err != nil {
				writeError(w, http.StatusBadRequest, "invalid hold list parameters")
				return
			}
			resp, err := client.ListHolds(callContext(r), req)
			if err != nil {
				writeDownstreamError(w, logger, r, err)
				return
			}
			writeProtoStatus(w, http.StatusOK, resp)
		case r.Method == http.MethodGet && r.PathValue("holdId") != "":
			if !holdIDHandler(w, r, client, logger, identity) {
				return
			}
		case r.Method == http.MethodPost && r.PathValue("holdId") != "" && strings.HasSuffix(r.URL.Path, "/cancel"):
			if !patientOnly(w, identity) {
				return
			}
			var body holdBody
			if r.Body != nil && r.ContentLength != 0 && !decodeBody(w, r, &body) {
				return
			}
			resp, err := client.CancelHold(callContext(r), &holdsv1.CancelHoldRequest{HoldId: r.PathValue("holdId"), UserId: identity.UserID, Reason: body.Reason})
			if err != nil {
				writeDownstreamError(w, logger, r, err)
				return
			}
			writeProtoStatus(w, http.StatusOK, resp.GetHold())
		case r.Method == http.MethodPost && r.PathValue("holdId") != "" && strings.HasSuffix(r.URL.Path, "/collect"):
			if !pharmacyOnly(w, identity) {
				return
			}
			var body holdBody
			if !decodeBody(w, r, &body) || strings.TrimSpace(body.PickupCode) == "" || identity.PharmacyID == "" {
				writeError(w, http.StatusBadRequest, "invalid collect request")
				return
			}
			resp, err := client.CollectHold(callContext(r), &holdsv1.CollectHoldRequest{HoldId: r.PathValue("holdId"), PharmacyId: identity.PharmacyID, PickupCode: body.PickupCode})
			if err != nil {
				writeDownstreamError(w, logger, r, err)
				return
			}
			writeProtoStatus(w, http.StatusOK, resp.GetHold())
		}
	}
}

// holdIDHandler fetches before returning a hold because GetHold has no owner
// field in its request. This prevents a patient from reading another patient's
// hold even if the downstream service exposes the lookup by ID.
func holdIDHandler(w http.ResponseWriter, r *http.Request, client ports.HoldsClient, logger *slog.Logger, identity requestIdentity) bool {
	resp, err := client.GetHold(callContext(r), &holdsv1.GetHoldRequest{HoldId: r.PathValue("holdId")})
	if err != nil {
		writeDownstreamError(w, logger, r, err)
		return false
	}
	hold := resp.GetHold()
	allowed := identity.Role == authv1.Role_ROLE_PATIENT && hold.GetUserId() == identity.UserID
	allowed = allowed || (identity.Role == authv1.Role_ROLE_PHARMACY_STAFF || identity.Role == authv1.Role_ROLE_ADMIN) && hold.GetPharmacyId() == identity.PharmacyID
	if !allowed {
		writeError(w, http.StatusForbidden, "forbidden")
		return false
	}
	if identity.Role != authv1.Role_ROLE_PATIENT {
		hold.PickupCode = ""
	}
	writeProtoStatus(w, http.StatusOK, hold)
	return true
}

type requestIdentity struct {
	UserID, PharmacyID string
	Role               authv1.Role
}

func identityFrom(r *http.Request) requestIdentity {
	return requestIdentity{UserID: httpx.UserIDFromContext(r.Context()), PharmacyID: httpx.PharmacyIDFromContext(r.Context()), Role: httpx.RoleFromContext(r.Context())}
}

func patientOnly(w http.ResponseWriter, identity requestIdentity) bool {
	if identity.Role != authv1.Role_ROLE_PATIENT {
		writeError(w, http.StatusForbidden, "forbidden")
		return false
	}
	return true
}

func pharmacyOnly(w http.ResponseWriter, identity requestIdentity) bool {
	if identity.Role != authv1.Role_ROLE_PHARMACY_STAFF && identity.Role != authv1.Role_ROLE_ADMIN {
		writeError(w, http.StatusForbidden, "forbidden")
		return false
	}
	return true
}

func fillHoldListQuery(r *http.Request, req *holdsv1.ListHoldsRequest) error {
	if value := r.URL.Query().Get("pageSize"); value != "" {
		pageSize, err := strconv.Atoi(value)
		if err != nil || pageSize < 0 || pageSize > 100 {
			return strconv.ErrSyntax
		}
		req.Page.PageSize = int32(pageSize)
	}
	req.Page.PageToken = r.URL.Query().Get("pageToken")
	if value := r.URL.Query().Get("status"); value != "" {
		number, ok := holdsv1.HoldStatus_value[value]
		if !ok {
			return strconv.ErrSyntax
		}
		req.Status = holdsv1.HoldStatus(number)
	}
	return nil
}

func callContext(r *http.Request) context.Context {
	ctx, _ := context.WithTimeout(r.Context(), downstreamTimeout)
	return ctx
}

func decodeBody(w http.ResponseWriter, r *http.Request, dst any) bool {
	if r.Body == nil {
		writeError(w, http.StatusBadRequest, "request body is required")
		return false
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(dst); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return false
	}
	return true
}

func writeProtoStatus(w http.ResponseWriter, status int, message interface{ ProtoReflect() protoreflect.Message }) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	data, err := (protojson.MarshalOptions{}).Marshal(message)
	if err == nil {
		_, _ = w.Write(data)
	}
}
