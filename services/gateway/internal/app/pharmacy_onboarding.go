package app

import (
	"context"
	"log/slog"
	"math"
	"net/http"
	"strings"
	"unicode/utf8"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	authv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/auth/v1"
	commonv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/common/v1"
	geov1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/geo/v1"
	pharmacyregistryv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/pharmacy-registry/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/gateway/internal/httpx"
	"github.com/Swiftmeds-bht/Swiftmeds/services/gateway/internal/ports"
)

// maxFieldLength keeps the free-text pharmacy fields to a sensible size.
const maxFieldLength = 200

type geoPointBody struct {
	Latitude  *float64 `json:"latitude"`
	Longitude *float64 `json:"longitude"`
}

type registerPharmacyBody struct {
	Name          string        `json:"name"`
	LicenseNumber string        `json:"licenseNumber"`
	Phone         string        `json:"phone"`
	Address       string        `json:"address"`
	Dzongkhag     string        `json:"dzongkhag"`
	Location      *geoPointBody `json:"location"`
}

// registerPharmacyHandler is registerPharmacy (SCRUM-141). Any signed-in
// account without a pharmacy may register one; it starts pending and stays out
// of search until an administrator verifies it. The account then becomes the
// pharmacy's staff account (Auth.LinkPharmacy), and its next token refresh
// carries the staff role and the pharmacy. Sending the same request again is
// safe: Registry returns the owner's existing pharmacy and the link is
// idempotent.
func registerPharmacyHandler(registry ports.PharmacyOnboarding, linker ports.PharmacyLinker, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		identity := identityFrom(r)
		if identity.UserID == "" || identity.Role == authv1.Role_ROLE_ADMIN {
			writeError(w, http.StatusForbidden, "forbidden")
			return
		}
		if identity.PharmacyID != "" {
			writeError(w, http.StatusConflict, "this account already belongs to a pharmacy")
			return
		}
		var body registerPharmacyBody
		if !decodeBody(w, r, &body) {
			return
		}
		req, problem := body.toRequest(identity.UserID)
		if problem != "" {
			writeError(w, http.StatusBadRequest, problem)
			return
		}

		ctx, cancel := callContext(r)
		defer cancel()
		resp, err := registry.RegisterPharmacy(ctx, req)
		switch status.Code(err) {
		case codes.OK:
		case codes.AlreadyExists:
			writeError(w, http.StatusConflict, "a pharmacy with this licence number is already registered")
			return
		case codes.InvalidArgument:
			writeError(w, http.StatusBadRequest, status.Convert(err).Message())
			return
		default:
			writeDownstreamError(w, logger, r, err)
			return
		}
		pharmacy := resp.GetPharmacy()
		if pharmacy.GetPharmacyId() == "" || pharmacy.GetOwnerUserId() != identity.UserID {
			writeError(w, http.StatusBadGateway, "pharmacy registry returned an unexpected pharmacy")
			return
		}
		_, err = linker.LinkPharmacy(ctx, &authv1.LinkPharmacyRequest{UserId: identity.UserID, PharmacyId: pharmacy.GetPharmacyId()})
		if status.Code(err) == codes.FailedPrecondition {
			writeError(w, http.StatusConflict, "this account cannot be linked to a pharmacy")
			return
		}
		if err != nil {
			writeDownstreamError(w, logger, r, err)
			return
		}
		writeProtoStatus(w, http.StatusCreated, pharmacy)
	}
}

func (b registerPharmacyBody) toRequest(ownerID string) (*pharmacyregistryv1.RegisterPharmacyRequest, string) {
	fields := []struct{ name, value string }{
		{"name", b.Name}, {"licenseNumber", b.LicenseNumber}, {"phone", b.Phone},
		{"address", b.Address}, {"dzongkhag", b.Dzongkhag},
	}
	for _, f := range fields {
		value := strings.TrimSpace(f.value)
		if value == "" {
			return nil, f.name + " is required"
		}
		if utf8.RuneCountInString(value) > maxFieldLength {
			return nil, f.name + " is too long"
		}
	}
	loc := b.Location
	if loc == nil || loc.Latitude == nil || loc.Longitude == nil ||
		math.IsNaN(*loc.Latitude) || math.IsNaN(*loc.Longitude) ||
		*loc.Latitude < -90 || *loc.Latitude > 90 || *loc.Longitude < -180 || *loc.Longitude > 180 {
		return nil, "location must have a latitude (-90 to 90) and a longitude (-180 to 180)"
	}
	return &pharmacyregistryv1.RegisterPharmacyRequest{
		OwnerUserId:   ownerID,
		Name:          strings.TrimSpace(b.Name),
		LicenseNumber: strings.TrimSpace(b.LicenseNumber),
		Phone:         strings.TrimSpace(b.Phone),
		Address:       strings.TrimSpace(b.Address),
		Dzongkhag:     strings.TrimSpace(b.Dzongkhag),
		Location:      &commonv1.GeoPoint{Latitude: *loc.Latitude, Longitude: *loc.Longitude},
	}, ""
}

// listPharmaciesHandler is listPharmacies (SCRUM-141): administrators review
// pharmacies, optionally by status (?status=VERIFICATION_STATUS_PENDING).
func listPharmaciesHandler(registry ports.PharmacyOnboarding, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if identityFrom(r).Role != authv1.Role_ROLE_ADMIN {
			writeError(w, http.StatusForbidden, "forbidden")
			return
		}
		req := &pharmacyregistryv1.ListPharmaciesRequest{}
		if value := r.URL.Query().Get("status"); value != "" {
			number, ok := pharmacyregistryv1.VerificationStatus_value[value]
			if !ok || number == 0 {
				writeError(w, http.StatusBadRequest, "status must be a VERIFICATION_STATUS_ value")
				return
			}
			req.Status = pharmacyregistryv1.VerificationStatus(number)
		}
		page, ok := pageRequest(r)
		if !ok {
			writeError(w, http.StatusBadRequest, "invalid page parameters")
			return
		}
		req.Page = page
		ctx, cancel := callContext(r)
		defer cancel()
		resp, err := registry.ListPharmacies(ctx, req)
		if err != nil {
			writeDownstreamError(w, logger, r, err)
			return
		}
		writeProtoStatus(w, http.StatusOK, resp)
	}
}

type verificationBody struct {
	Status string `json:"status"`
	Reason string `json:"reason"`
}

// verificationHandler is setPharmacyVerification (SCRUM-141): an administrator
// approves, rejects or suspends a pharmacy. Approving puts the pharmacy on
// Geo's map, so search finds it; suspending or rejecting takes it off.
//
// Sending the status the pharmacy already has does not change Registry; it
// only puts Geo in step again. So if Geo was down when a pharmacy was approved,
// the administrator can simply approve it again.
func verificationHandler(registry ports.PharmacyOnboarding, locations ports.PharmacyLocations, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		identity := identityFrom(r)
		if identity.Role != authv1.Role_ROLE_ADMIN {
			writeError(w, http.StatusForbidden, "forbidden")
			return
		}
		var body verificationBody
		if !decodeBody(w, r, &body) {
			return
		}
		next, ok := settableStatus(body.Status)
		if !ok {
			writeError(w, http.StatusBadRequest,
				"status must be VERIFICATION_STATUS_VERIFIED, VERIFICATION_STATUS_REJECTED or VERIFICATION_STATUS_SUSPENDED")
			return
		}
		reason := strings.TrimSpace(body.Reason)
		if next != pharmacyregistryv1.VerificationStatus_VERIFICATION_STATUS_VERIFIED && reason == "" {
			writeError(w, http.StatusBadRequest, "a reason is required to reject or suspend a pharmacy")
			return
		}
		if utf8.RuneCountInString(reason) > 500 {
			writeError(w, http.StatusBadRequest, "reason is too long")
			return
		}

		id := r.PathValue("pharmacyId")
		ctx, cancel := callContext(r)
		defer cancel()
		current, err := registry.GetPharmacy(ctx, &pharmacyregistryv1.GetPharmacyRequest{PharmacyId: id})
		if status.Code(err) == codes.NotFound || (err == nil && current.GetPharmacy() == nil) {
			writeError(w, http.StatusNotFound, "pharmacy not found")
			return
		}
		if err != nil {
			writeDownstreamError(w, logger, r, err)
			return
		}
		pharmacy := current.GetPharmacy()
		if pharmacy.GetStatus() != next {
			updated, err := registry.SetVerificationStatus(ctx, &pharmacyregistryv1.SetVerificationStatusRequest{
				PharmacyId: id, AdminUserId: identity.UserID, Status: next, Reason: reason,
			})
			if status.Code(err) == codes.InvalidArgument {
				writeError(w, http.StatusBadRequest, "a "+statusWord(pharmacy.GetStatus())+" pharmacy cannot be "+statusVerb(next))
				return
			}
			if err != nil {
				writeDownstreamError(w, logger, r, err)
				return
			}
			pharmacy = updated.GetPharmacy()
		}

		if err := syncLocation(ctx, locations, pharmacy); err != nil {
			logRPCError(logger, "pharmacy location sync failed", httpx.RequestIDFromContext(r.Context()), err)
			writeError(w, http.StatusBadGateway, "the status was saved, but the map could not be updated; send the same request again")
			return
		}
		writeProtoStatus(w, http.StatusOK, pharmacy)
	}
}

// syncLocation puts a verified pharmacy on Geo's map and takes any other off.
func syncLocation(ctx context.Context, locations ports.PharmacyLocations, pharmacy *pharmacyregistryv1.Pharmacy) error {
	if pharmacy.GetStatus() == pharmacyregistryv1.VerificationStatus_VERIFICATION_STATUS_VERIFIED {
		_, err := locations.UpsertPharmacyLocation(ctx, &geov1.UpsertPharmacyLocationRequest{
			PharmacyId: pharmacy.GetPharmacyId(), Location: pharmacy.GetLocation(),
		})
		return err
	}
	_, err := locations.RemovePharmacyLocation(ctx, &geov1.RemovePharmacyLocationRequest{PharmacyId: pharmacy.GetPharmacyId()})
	if status.Code(err) == codes.NotFound {
		return nil // never was on the map
	}
	return err
}

func settableStatus(value string) (pharmacyregistryv1.VerificationStatus, bool) {
	switch value {
	case "VERIFICATION_STATUS_VERIFIED":
		return pharmacyregistryv1.VerificationStatus_VERIFICATION_STATUS_VERIFIED, true
	case "VERIFICATION_STATUS_REJECTED":
		return pharmacyregistryv1.VerificationStatus_VERIFICATION_STATUS_REJECTED, true
	case "VERIFICATION_STATUS_SUSPENDED":
		return pharmacyregistryv1.VerificationStatus_VERIFICATION_STATUS_SUSPENDED, true
	default:
		return 0, false
	}
}

func statusWord(s pharmacyregistryv1.VerificationStatus) string {
	switch s {
	case pharmacyregistryv1.VerificationStatus_VERIFICATION_STATUS_PENDING:
		return "pending"
	case pharmacyregistryv1.VerificationStatus_VERIFICATION_STATUS_VERIFIED:
		return "verified"
	case pharmacyregistryv1.VerificationStatus_VERIFICATION_STATUS_REJECTED:
		return "rejected"
	case pharmacyregistryv1.VerificationStatus_VERIFICATION_STATUS_SUSPENDED:
		return "suspended"
	default:
		return "unknown"
	}
}

func statusVerb(s pharmacyregistryv1.VerificationStatus) string {
	switch s {
	case pharmacyregistryv1.VerificationStatus_VERIFICATION_STATUS_VERIFIED:
		return "approved"
	case pharmacyregistryv1.VerificationStatus_VERIFICATION_STATUS_REJECTED:
		return "rejected"
	default:
		return "suspended"
	}
}
