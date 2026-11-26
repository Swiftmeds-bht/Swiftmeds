//go:build integration

package compose

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

// TestPharmacyOnboarding covers SCRUM-141 end to end (SCRUM-155): a new
// pharmacy registers and is pending and off the map; an admin approves it and
// it is on the map; suspending takes it off again. A second pharmacy is
// rejected with a reason. Needs only GATEWAY_URL (Compose or the k3d cluster).
func TestPharmacyOnboarding(t *testing.T) {
	apiURL := strings.TrimRight(os.Getenv("GATEWAY_URL"), "/")
	if apiURL == "" {
		t.Skip("set GATEWAY_URL to run against Compose or the cluster")
	}
	adminEmail := envOr("ADMIN_EMAIL", "admin@demo.swiftmeds.test")
	api := &client{base: apiURL, http: &http.Client{Timeout: 20 * time.Second}}
	stamp := time.Now().UTC().Format("20060102150405.000000")

	// A spot in Thimphu that no seeded pharmacy uses.
	lat, lng := 27.44+rand.Float64()*0.01, 89.66+rand.Float64()*0.01
	register := func(t *testing.T, token, name, licence string, want int) map[string]any {
		t.Helper()
		var out map[string]any
		api.json(t, http.MethodPost, "/api/v1/pharmacies", token, map[string]any{
			"name": name, "licenseNumber": licence, "phone": "02334455",
			"address": "Norzin Lam, Thimphu", "dzongkhag": "Thimphu",
			"location": map[string]float64{"latitude": lat, "longitude": lng},
		}, want, &out)
		return out
	}
	nearby := func(t *testing.T) []string {
		t.Helper()
		var out struct {
			Pharmacies []struct {
				PharmacyID string `json:"pharmacyId"`
			} `json:"pharmacies"`
		}
		api.json(t, http.MethodGet, fmt.Sprintf("/api/v1/pharmacies/nearby?lat=%f&lng=%f&radius_m=1000", lat, lng), "", nil, http.StatusOK, &out)
		ids := make([]string, 0, len(out.Pharmacies))
		for _, p := range out.Pharmacies {
			ids = append(ids, p.PharmacyID)
		}
		return ids
	}
	verify := func(t *testing.T, token, id, status, reason string, want int) map[string]any {
		t.Helper()
		body := map[string]string{"status": status}
		if reason != "" {
			body["reason"] = reason
		}
		var out map[string]any
		api.json(t, http.MethodPut, "/api/v1/pharmacies/"+id+"/verification", token, body, want, &out)
		return out
	}

	owner := api.signUp(t, "onboarding-"+stamp+"@example.test")
	licence := "E2E-" + stamp
	name := "Integration Pharmacy " + stamp
	admin := api.login(t, adminEmail)

	created := register(t, owner.access, name, licence, http.StatusCreated)
	id, _ := created["pharmacyId"].(string)
	if id == "" || created["status"] != "VERIFICATION_STATUS_PENDING" {
		t.Fatalf("registered pharmacy = %v, want an ID and PENDING", created)
	}
	t.Cleanup(func() {
		// Leave the demo map as it was, whatever happened above.
		var p map[string]any
		api.json(t, http.MethodGet, "/api/v1/pharmacies/"+id, "", nil, http.StatusOK, &p)
		if p["status"] == "VERIFICATION_STATUS_VERIFIED" {
			verify(t, admin, id, "VERIFICATION_STATUS_SUSPENDED", "integration test clean-up", http.StatusOK)
		}
	})

	t.Run("registering again returns the same pharmacy", func(t *testing.T) {
		again := register(t, owner.access, name, licence, http.StatusCreated)
		if again["pharmacyId"] != id {
			t.Fatalf("second registration made %v, want %s", again["pharmacyId"], id)
		}
	})

	t.Run("another account cannot take the licence number", func(t *testing.T) {
		other := api.signUp(t, "onboarding-other-"+stamp+"@example.test")
		register(t, other.access, "Copy Pharmacy", licence, http.StatusConflict)
	})

	t.Run("the refreshed token is staff of the new pharmacy", func(t *testing.T) {
		claims := api.refreshClaims(t, owner.refresh)
		if claims["role"] != "PHARMACIST" || claims["pharmacy_id"] != id {
			t.Fatalf("claims = %v, want PHARMACIST of %s", claims, id)
		}
	})

	t.Run("pending: off the map, waiting for the admin, hidden from others", func(t *testing.T) {
		if contains(nearby(t), id) {
			t.Fatal("a pending pharmacy is on the map")
		}
		var list struct {
			Pharmacies []map[string]any `json:"pharmacies"`
		}
		api.json(t, http.MethodGet, "/api/v1/pharmacies?status=VERIFICATION_STATUS_PENDING", admin, nil, http.StatusOK, &list)
		found := false
		for _, p := range list.Pharmacies {
			found = found || p["pharmacyId"] == id
		}
		if !found {
			t.Fatal("the admin does not see the pharmacy waiting for review")
		}
		api.json(t, http.MethodGet, "/api/v1/pharmacies?status=VERIFICATION_STATUS_PENDING", owner.access, nil, http.StatusForbidden, nil)
	})

	t.Run("approved: on the map; suspended: off again", func(t *testing.T) {
		if got := verify(t, admin, id, "VERIFICATION_STATUS_VERIFIED", "", http.StatusOK); got["status"] != "VERIFICATION_STATUS_VERIFIED" {
			t.Fatalf("approve = %v", got)
		}
		if !contains(nearby(t), id) {
			t.Fatal("an approved pharmacy is not on the map")
		}
		verify(t, admin, id, "VERIFICATION_STATUS_SUSPENDED", "", http.StatusBadRequest) // a reason is required
		verify(t, admin, id, "VERIFICATION_STATUS_SUSPENDED", "Licence under review", http.StatusOK)
		if contains(nearby(t), id) {
			t.Fatal("a suspended pharmacy is still on the map")
		}
	})

	t.Run("rejected: the reason reaches the pharmacy", func(t *testing.T) {
		owner2 := api.signUp(t, "onboarding-rejected-"+stamp+"@example.test")
		second := register(t, owner2.access, "Rejected Pharmacy "+stamp, "E2E-R-"+stamp, http.StatusCreated)
		id2, _ := second["pharmacyId"].(string)
		verify(t, admin, id2, "VERIFICATION_STATUS_REJECTED", "Licence number not found", http.StatusOK)
		var p map[string]any
		api.json(t, http.MethodGet, "/api/v1/pharmacies/"+id2, "", nil, http.StatusOK, &p)
		if p["status"] != "VERIFICATION_STATUS_REJECTED" || p["statusReason"] != "Licence number not found" {
			t.Fatalf("rejected pharmacy = %v", p)
		}
		verify(t, admin, id2, "VERIFICATION_STATUS_VERIFIED", "", http.StatusBadRequest) // rejected stays rejected
	})
}

type tokens struct{ access, refresh string }

// signUp registers a patient account and logs in, keeping the refresh token.
func (c *client) signUp(t *testing.T, email string) tokens {
	t.Helper()
	c.json(t, http.MethodPost, "/api/v1/auth/register", "", map[string]string{
		"email": email, "password": demoPassword, "role": "ROLE_PATIENT",
	}, http.StatusCreated, nil)
	var response struct {
		Tokens struct {
			AccessToken  string `json:"accessToken"`
			RefreshToken string `json:"refreshToken"`
		} `json:"tokens"`
	}
	c.json(t, http.MethodPost, "/api/v1/auth/login", "", map[string]string{"email": email, "password": demoPassword}, http.StatusOK, &response)
	return tokens{access: response.Tokens.AccessToken, refresh: response.Tokens.RefreshToken}
}

// refreshClaims refreshes and returns the new access token's claims (read, not verified).
func (c *client) refreshClaims(t *testing.T, refresh string) map[string]any {
	t.Helper()
	var response struct {
		Tokens struct {
			AccessToken string `json:"accessToken"`
		} `json:"tokens"`
	}
	c.json(t, http.MethodPost, "/api/v1/auth/refresh", "", map[string]string{"refreshToken": refresh}, http.StatusOK, &response)
	parts := strings.Split(response.Tokens.AccessToken, ".")
	if len(parts) != 3 {
		t.Fatalf("refresh returned no JWT")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("decode token: %v", err)
	}
	var claims map[string]any
	if err := json.Unmarshal(payload, &claims); err != nil {
		t.Fatalf("decode claims: %v", err)
	}
	return claims
}

func envOr(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}
