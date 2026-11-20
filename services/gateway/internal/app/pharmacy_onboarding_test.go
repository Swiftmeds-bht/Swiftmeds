package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	authv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/auth/v1"
	commonv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/common/v1"
	geov1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/geo/v1"
	pharmacyregistryv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/pharmacy-registry/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/gateway/internal/ports"
)

type fakeOnboarding struct {
	pharmacies  map[string]*pharmacyregistryv1.Pharmacy
	registered  *pharmacyregistryv1.RegisterPharmacyRequest
	listed      *pharmacyregistryv1.ListPharmaciesRequest
	set         []*pharmacyregistryv1.SetVerificationStatusRequest
	registerErr error
}

func newFakeOnboarding(existing ...*pharmacyregistryv1.Pharmacy) *fakeOnboarding {
	f := &fakeOnboarding{pharmacies: map[string]*pharmacyregistryv1.Pharmacy{}}
	for _, p := range existing {
		f.pharmacies[p.GetPharmacyId()] = p
	}
	return f
}

func (f *fakeOnboarding) RegisterPharmacy(_ context.Context, req *pharmacyregistryv1.RegisterPharmacyRequest) (*pharmacyregistryv1.RegisterPharmacyResponse, error) {
	f.registered = req
	if f.registerErr != nil {
		return nil, f.registerErr
	}
	p := &pharmacyregistryv1.Pharmacy{
		PharmacyId: "ph-new", OwnerUserId: req.GetOwnerUserId(), Name: req.GetName(), LicenseNumber: req.GetLicenseNumber(),
		Phone: req.GetPhone(), Address: req.GetAddress(), Dzongkhag: req.GetDzongkhag(), Location: req.GetLocation(),
		Status: pharmacyregistryv1.VerificationStatus_VERIFICATION_STATUS_PENDING,
	}
	f.pharmacies[p.GetPharmacyId()] = p
	return &pharmacyregistryv1.RegisterPharmacyResponse{Pharmacy: p}, nil
}

func (f *fakeOnboarding) GetPharmacy(_ context.Context, req *pharmacyregistryv1.GetPharmacyRequest) (*pharmacyregistryv1.GetPharmacyResponse, error) {
	p, ok := f.pharmacies[req.GetPharmacyId()]
	if !ok {
		return nil, status.Error(codes.NotFound, "pharmacy not found")
	}
	return &pharmacyregistryv1.GetPharmacyResponse{Pharmacy: p}, nil
}

func (f *fakeOnboarding) ListPharmacies(_ context.Context, req *pharmacyregistryv1.ListPharmaciesRequest) (*pharmacyregistryv1.ListPharmaciesResponse, error) {
	f.listed = req
	out := &pharmacyregistryv1.ListPharmaciesResponse{}
	for _, p := range f.pharmacies {
		if req.GetStatus() == 0 || p.GetStatus() == req.GetStatus() {
			out.Pharmacies = append(out.Pharmacies, p)
		}
	}
	return out, nil
}

// SetVerificationStatus follows Registry's rules: pending → verified or
// rejected, verified → suspended.
func (f *fakeOnboarding) SetVerificationStatus(_ context.Context, req *pharmacyregistryv1.SetVerificationStatusRequest) (*pharmacyregistryv1.SetVerificationStatusResponse, error) {
	f.set = append(f.set, req)
	p := f.pharmacies[req.GetPharmacyId()]
	from, to := p.GetStatus(), req.GetStatus()
	allowed := from == pharmacyregistryv1.VerificationStatus_VERIFICATION_STATUS_PENDING &&
		(to == pharmacyregistryv1.VerificationStatus_VERIFICATION_STATUS_VERIFIED || to == pharmacyregistryv1.VerificationStatus_VERIFICATION_STATUS_REJECTED) ||
		from == pharmacyregistryv1.VerificationStatus_VERIFICATION_STATUS_VERIFIED && to == pharmacyregistryv1.VerificationStatus_VERIFICATION_STATUS_SUSPENDED
	if !allowed {
		return nil, status.Error(codes.InvalidArgument, "invalid verification status transition")
	}
	next := &pharmacyregistryv1.Pharmacy{
		PharmacyId: p.GetPharmacyId(), OwnerUserId: p.GetOwnerUserId(), Name: p.GetName(), LicenseNumber: p.GetLicenseNumber(),
		Location: p.GetLocation(), Status: to, StatusReason: req.GetReason(),
	}
	f.pharmacies[p.GetPharmacyId()] = next
	return &pharmacyregistryv1.SetVerificationStatusResponse{Pharmacy: next}, nil
}

type fakeLinker struct {
	linked *authv1.LinkPharmacyRequest
	err    error
}

func (f *fakeLinker) LinkPharmacy(_ context.Context, req *authv1.LinkPharmacyRequest) (*authv1.LinkPharmacyResponse, error) {
	f.linked = req
	if f.err != nil {
		return nil, f.err
	}
	return &authv1.LinkPharmacyResponse{UserId: req.GetUserId(), PharmacyId: req.GetPharmacyId(), Role: authv1.Role_ROLE_PHARMACY_STAFF}, nil
}

type fakeLocations struct {
	upserted []*geov1.UpsertPharmacyLocationRequest
	removed  []string
	err      error
}

func (f *fakeLocations) UpsertPharmacyLocation(_ context.Context, req *geov1.UpsertPharmacyLocationRequest) (*geov1.UpsertPharmacyLocationResponse, error) {
	if f.err != nil {
		return nil, f.err
	}
	f.upserted = append(f.upserted, req)
	return &geov1.UpsertPharmacyLocationResponse{}, nil
}

func (f *fakeLocations) RemovePharmacyLocation(_ context.Context, req *geov1.RemovePharmacyLocationRequest) (*geov1.RemovePharmacyLocationResponse, error) {
	if f.err != nil {
		return nil, f.err
	}
	f.removed = append(f.removed, req.GetPharmacyId())
	return &geov1.RemovePharmacyLocationResponse{}, nil
}

var (
	newOwner = ports.Identity{UserID: "u-new", Role: authv1.Role_ROLE_PATIENT}
	admin    = ports.Identity{UserID: "u-admin", Role: authv1.Role_ROLE_ADMIN}
)

type onboardingDeps struct {
	registry  *fakeOnboarding
	linker    *fakeLinker
	locations *fakeLocations
}

// onboardingRouter builds the real router, with the public pharmacy lookup
// too, so clashing route patterns fail here rather than at startup.
func onboardingRouter(identity ports.Identity, deps onboardingDeps) http.Handler {
	return NewRouter(Config{}, testLogger(),
		WithAuth(staticAuthenticator{identity: identity}),
		WithPharmacyRegistry(&fakePharmacyRegistryClient{}),
		WithPharmacyOnboarding(deps.registry, deps.linker, deps.locations),
	)
}

func newDeps(existing ...*pharmacyregistryv1.Pharmacy) onboardingDeps {
	return onboardingDeps{registry: newFakeOnboarding(existing...), linker: &fakeLinker{}, locations: &fakeLocations{}}
}

const validRegistration = `{"name":" Thimphu Care ","licenseNumber":"BMHC-2026-114","phone":"+975 2 334 455",
	"address":"Norzin Lam","dzongkhag":"Thimphu","location":{"latitude":27.4728,"longitude":89.639}}`

func TestRegisterPharmacyMakesItPendingAndLinksTheAccount(t *testing.T) {
	deps := newDeps()
	rec := serve(onboardingRouter(newOwner, deps), http.MethodPost, "/api/v1/pharmacies", validRegistration)
	if rec.Code != http.StatusCreated {
		t.Fatalf("got %d, want 201: %s", rec.Code, rec.Body.String())
	}
	if got := deps.registry.registered; got.GetOwnerUserId() != "u-new" || got.GetName() != "Thimphu Care" || got.GetLocation().GetLatitude() != 27.4728 {
		t.Fatalf("registry got %v", got)
	}
	if got := deps.linker.linked; got.GetUserId() != "u-new" || got.GetPharmacyId() != "ph-new" {
		t.Fatalf("auth link got %v", got)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["status"] != "VERIFICATION_STATUS_PENDING" || body["licenseNumber"] != "BMHC-2026-114" {
		t.Fatalf("body = %v", body)
	}
}

func TestRegisterPharmacyValidatesTheBody(t *testing.T) {
	cases := map[string]string{
		"missing phone":   `{"name":"A","licenseNumber":"L","address":"x","dzongkhag":"Paro","location":{"latitude":27,"longitude":89}}`,
		"blank name":      `{"name":"  ","licenseNumber":"L","phone":"1","address":"x","dzongkhag":"Paro","location":{"latitude":27,"longitude":89}}`,
		"no location":     `{"name":"A","licenseNumber":"L","phone":"1","address":"x","dzongkhag":"Paro"}`,
		"bad latitude":    `{"name":"A","licenseNumber":"L","phone":"1","address":"x","dzongkhag":"Paro","location":{"latitude":95,"longitude":89}}`,
		"half a location": `{"name":"A","licenseNumber":"L","phone":"1","address":"x","dzongkhag":"Paro","location":{"latitude":27}}`,
		"unknown field":   `{"name":"A","licenseNumber":"L","phone":"1","address":"x","dzongkhag":"Paro","location":{"latitude":27,"longitude":89},"ownerUserId":"someone"}`,
		"very long name":  `{"name":"` + strings.Repeat("a", 201) + `","licenseNumber":"L","phone":"1","address":"x","dzongkhag":"Paro","location":{"latitude":27,"longitude":89}}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			deps := newDeps()
			rec := serve(onboardingRouter(newOwner, deps), http.MethodPost, "/api/v1/pharmacies", body)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("got %d, want 400: %s", rec.Code, rec.Body.String())
			}
			if deps.registry.registered != nil {
				t.Fatal("an invalid body reached Registry")
			}
		})
	}
}

func TestRegisterPharmacyRefusals(t *testing.T) {
	t.Run("an account that already has a pharmacy", func(t *testing.T) {
		rec := serve(onboardingRouter(staffOfPharmacy1, newDeps()), http.MethodPost, "/api/v1/pharmacies", validRegistration)
		if rec.Code != http.StatusConflict {
			t.Fatalf("got %d, want 409", rec.Code)
		}
	})
	t.Run("an admin", func(t *testing.T) {
		rec := serve(onboardingRouter(admin, newDeps()), http.MethodPost, "/api/v1/pharmacies", validRegistration)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("got %d, want 403", rec.Code)
		}
	})
	t.Run("a licence number someone else registered", func(t *testing.T) {
		deps := newDeps()
		deps.registry.registerErr = status.Error(codes.AlreadyExists, "taken")
		rec := serve(onboardingRouter(newOwner, deps), http.MethodPost, "/api/v1/pharmacies", validRegistration)
		if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "licence number") {
			t.Fatalf("got %d: %s", rec.Code, rec.Body.String())
		}
		if deps.linker.linked != nil {
			t.Fatal("the account was linked although registration failed")
		}
	})
	t.Run("an account Auth will not link", func(t *testing.T) {
		deps := newDeps()
		deps.linker.err = status.Error(codes.FailedPrecondition, "linked elsewhere")
		rec := serve(onboardingRouter(newOwner, deps), http.MethodPost, "/api/v1/pharmacies", validRegistration)
		if rec.Code != http.StatusConflict {
			t.Fatalf("got %d, want 409", rec.Code)
		}
	})
	t.Run("signed out", func(t *testing.T) {
		router := onboardingRouter(newOwner, newDeps())
		req, _ := http.NewRequest(http.MethodPost, "/api/v1/pharmacies", strings.NewReader(validRegistration))
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("got %d, want 401", rec.Code)
		}
	})
}

func pending(id string) *pharmacyregistryv1.Pharmacy {
	return &pharmacyregistryv1.Pharmacy{
		PharmacyId: id, OwnerUserId: "owner-" + id, Name: "Pharmacy " + id, LicenseNumber: "LIC-" + id,
		Location: &commonv1.GeoPoint{Latitude: 27.47, Longitude: 89.64}, Status: pharmacyregistryv1.VerificationStatus_VERIFICATION_STATUS_PENDING,
	}
}

func TestListPharmaciesIsForAdmins(t *testing.T) {
	deps := newDeps(pending("ph-1"))
	rec := serve(onboardingRouter(admin, deps), http.MethodGet, "/api/v1/pharmacies?status=VERIFICATION_STATUS_PENDING&pageSize=20", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"pharmacyId":"ph-1"`) {
		t.Fatalf("admin: got %d: %s", rec.Code, rec.Body.String())
	}
	if deps.registry.listed.GetStatus() != pharmacyregistryv1.VerificationStatus_VERIFICATION_STATUS_PENDING || deps.registry.listed.GetPage().GetPageSize() != 20 {
		t.Fatalf("registry got %v", deps.registry.listed)
	}
	if rec := serve(onboardingRouter(admin, deps), http.MethodGet, "/api/v1/pharmacies?status=APPROVED", ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad status: got %d, want 400", rec.Code)
	}
	for _, who := range []ports.Identity{staffOfPharmacy1, newOwner} {
		if rec := serve(onboardingRouter(who, deps), http.MethodGet, "/api/v1/pharmacies", ""); rec.Code != http.StatusForbidden {
			t.Fatalf("%v: got %d, want 403", who.Role, rec.Code)
		}
	}
}

func TestApprovingPutsThePharmacyOnTheMap(t *testing.T) {
	deps := newDeps(pending("ph-1"))
	rec := serve(onboardingRouter(admin, deps), http.MethodPut, "/api/v1/pharmacies/ph-1/verification", `{"status":"VERIFICATION_STATUS_VERIFIED"}`)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "VERIFICATION_STATUS_VERIFIED") {
		t.Fatalf("got %d: %s", rec.Code, rec.Body.String())
	}
	if len(deps.registry.set) != 1 || deps.registry.set[0].GetAdminUserId() != "u-admin" {
		t.Fatalf("registry got %v", deps.registry.set)
	}
	if len(deps.locations.upserted) != 1 || deps.locations.upserted[0].GetPharmacyId() != "ph-1" || deps.locations.upserted[0].GetLocation().GetLatitude() != 27.47 {
		t.Fatalf("geo got %v", deps.locations.upserted)
	}

	// Approving again changes nothing in Registry and only re-syncs Geo.
	rec = serve(onboardingRouter(admin, deps), http.MethodPut, "/api/v1/pharmacies/ph-1/verification", `{"status":"VERIFICATION_STATUS_VERIFIED"}`)
	if rec.Code != http.StatusOK || len(deps.registry.set) != 1 || len(deps.locations.upserted) != 2 {
		t.Fatalf("repeat: got %d, registry calls %d, geo upserts %d", rec.Code, len(deps.registry.set), len(deps.locations.upserted))
	}

	// Suspending takes it off the map.
	rec = serve(onboardingRouter(admin, deps), http.MethodPut, "/api/v1/pharmacies/ph-1/verification", `{"status":"VERIFICATION_STATUS_SUSPENDED","reason":"Licence expired"}`)
	if rec.Code != http.StatusOK || len(deps.locations.removed) != 1 {
		t.Fatalf("suspend: got %d, removed %v", rec.Code, deps.locations.removed)
	}
}

func TestVerificationRefusals(t *testing.T) {
	cases := []struct {
		name     string
		identity ports.Identity
		path     string
		body     string
		want     int
	}{
		{"staff", staffOfPharmacy1, "/api/v1/pharmacies/ph-1/verification", `{"status":"VERIFICATION_STATUS_VERIFIED"}`, http.StatusForbidden},
		{"reject without reason", admin, "/api/v1/pharmacies/ph-1/verification", `{"status":"VERIFICATION_STATUS_REJECTED"}`, http.StatusBadRequest},
		{"back to pending", admin, "/api/v1/pharmacies/ph-1/verification", `{"status":"VERIFICATION_STATUS_PENDING"}`, http.StatusBadRequest},
		{"suspend a pending pharmacy", admin, "/api/v1/pharmacies/ph-1/verification", `{"status":"VERIFICATION_STATUS_SUSPENDED","reason":"x"}`, http.StatusBadRequest},
		{"unknown pharmacy", admin, "/api/v1/pharmacies/nope/verification", `{"status":"VERIFICATION_STATUS_VERIFIED"}`, http.StatusNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			deps := newDeps(pending("ph-1"))
			rec := serve(onboardingRouter(tc.identity, deps), http.MethodPut, tc.path, tc.body)
			if rec.Code != tc.want {
				t.Fatalf("got %d, want %d: %s", rec.Code, tc.want, rec.Body.String())
			}
			if len(deps.locations.upserted) != 0 {
				t.Fatal("a refused request changed the map")
			}
		})
	}
}

func TestRejectingTakesTheReasonAndKeepsItOffTheMap(t *testing.T) {
	deps := newDeps(pending("ph-1"))
	rec := serve(onboardingRouter(admin, deps), http.MethodPut, "/api/v1/pharmacies/ph-1/verification", `{"status":"VERIFICATION_STATUS_REJECTED","reason":" Licence number not found "}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d: %s", rec.Code, rec.Body.String())
	}
	if deps.registry.set[0].GetReason() != "Licence number not found" {
		t.Fatalf("reason = %q", deps.registry.set[0].GetReason())
	}
	if len(deps.locations.upserted) != 0 || len(deps.locations.removed) != 1 {
		t.Fatalf("geo upserts %v removes %v", deps.locations.upserted, deps.locations.removed)
	}
}

func TestApprovalWhenGeoIsDownSaysToSendItAgain(t *testing.T) {
	deps := newDeps(pending("ph-1"))
	deps.locations.err = errors.New("geo unavailable")
	rec := serve(onboardingRouter(admin, deps), http.MethodPut, "/api/v1/pharmacies/ph-1/verification", `{"status":"VERIFICATION_STATUS_VERIFIED"}`)
	if rec.Code != http.StatusBadGateway || !strings.Contains(rec.Body.String(), "send the same request again") {
		t.Fatalf("got %d: %s", rec.Code, rec.Body.String())
	}
	// Once Geo is back, the same request finishes the job.
	deps.locations.err = nil
	rec = serve(onboardingRouter(admin, deps), http.MethodPut, "/api/v1/pharmacies/ph-1/verification", `{"status":"VERIFICATION_STATUS_VERIFIED"}`)
	if rec.Code != http.StatusOK || len(deps.locations.upserted) != 1 {
		t.Fatalf("retry: got %d, upserts %d", rec.Code, len(deps.locations.upserted))
	}
}
