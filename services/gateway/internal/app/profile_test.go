package app

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	authv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/auth/v1"
	userprofilev1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/user-profile/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/gateway/internal/ports"
)

type profileTestAuth struct{ role authv1.Role }

func (a profileTestAuth) ValidateToken(context.Context, string) (ports.Identity, error) {
	return ports.Identity{UserID: "user-1", Role: a.role}, nil
}

type profileTestClient struct {
	getErr    error
	updateErr error
	gotUser   string
	saved     *userprofilev1.UpdateProfileRequest
}

func (c *profileTestClient) GetProfile(_ context.Context, req *userprofilev1.GetProfileRequest) (*userprofilev1.GetProfileResponse, error) {
	c.gotUser = req.GetUserId()
	if c.getErr != nil {
		return nil, c.getErr
	}
	return &userprofilev1.GetProfileResponse{Profile: &userprofilev1.Profile{UserId: req.GetUserId(), DisplayName: "Karma", Phone: "17123456"}}, nil
}

func (c *profileTestClient) UpdateProfile(_ context.Context, req *userprofilev1.UpdateProfileRequest) (*userprofilev1.UpdateProfileResponse, error) {
	c.saved = req
	if c.updateErr != nil {
		return nil, c.updateErr
	}
	return &userprofilev1.UpdateProfileResponse{Profile: &userprofilev1.Profile{UserId: req.GetUserId(), DisplayName: req.GetDisplayName(), Phone: req.GetPhone()}}, nil
}

func profileRequestTo(t *testing.T, client *profileTestClient, role authv1.Role, method, body string, withToken bool) *httptest.ResponseRecorder {
	t.Helper()
	h := NewRouter(Config{CORSOrigins: defaultCORSOrigins}, slog.Default(), WithAuth(profileTestAuth{role: role}), WithProfile(client))
	req := httptest.NewRequest(method, ProfilePath, strings.NewReader(body))
	if withToken {
		req.Header.Set("Authorization", "Bearer test-token")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestProfileGetReturnsTheCallersOwnProfile(t *testing.T) {
	client := &profileTestClient{}
	rec := profileRequestTo(t, client, authv1.Role_ROLE_PATIENT, http.MethodGet, "", true)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body)
	}
	if client.gotUser != "user-1" {
		t.Errorf("downstream user = %q, want the token's user-1", client.gotUser)
	}
	// The contract (openapi Profile) is the profile object itself.
	var got struct{ UserID, DisplayName, Phone string }
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || got.DisplayName != "Karma" || got.Phone != "17123456" {
		t.Errorf("body = %s, want the Profile object as JSON", rec.Body)
	}
}

func TestProfileGetAnswers404BeforeTheFirstSave(t *testing.T) {
	client := &profileTestClient{getErr: status.Error(codes.NotFound, "profile not found")}
	rec := profileRequestTo(t, client, authv1.Role_ROLE_PATIENT, http.MethodGet, "", true)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404: %s", rec.Code, rec.Body)
	}
}

func TestProfilePutSavesForTheTokenUserOnly(t *testing.T) {
	client := &profileTestClient{}
	// A user_id in the body must be ignored: the profile is always the caller's.
	body := `{"displayName":"  Karma Wangmo ","phone":"+975 17-12 34 56","userId":"someone-else"}`
	rec := profileRequestTo(t, client, authv1.Role_ROLE_PATIENT, http.MethodPut, body, true)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body)
	}
	if client.saved.GetUserId() != "user-1" {
		t.Errorf("saved user = %q, want user-1 from the token", client.saved.GetUserId())
	}
	if client.saved.GetDisplayName() != "Karma Wangmo" || client.saved.GetPhone() != "+97517123456" {
		t.Errorf("saved %q / %q, want the trimmed name and the phone without separators", client.saved.GetDisplayName(), client.saved.GetPhone())
	}
}

func TestProfilePutRejectsBadInputWithAClearMessage(t *testing.T) {
	tests := map[string]struct{ body, want string }{
		"no name":        {`{"displayName":"  ","phone":"17123456"}`, "enter your name"},
		"long name":      {`{"displayName":"` + strings.Repeat("a", 81) + `","phone":"17123456"}`, "at most 80"},
		"short phone":    {`{"displayName":"Karma","phone":"1712"}`, "8 to 15 digits"},
		"letters":        {`{"displayName":"Karma","phone":"17abc456"}`, "8 to 15 digits"},
		"not json":       {`displayName=Karma`, "invalid request body"},
		"leading zero":   {`{"displayName":"Karma","phone":"01712345"}`, "8 to 15 digits"},
		"missing phone":  {`{"displayName":"Karma"}`, "8 to 15 digits"},
		"oversized body": {`{"displayName":"` + strings.Repeat("a", 5000) + `"}`, "invalid request body"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			client := &profileTestClient{}
			rec := profileRequestTo(t, client, authv1.Role_ROLE_PATIENT, http.MethodPut, tc.body, true)

			if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), tc.want) {
				t.Fatalf("status = %d body = %s, want 400 containing %q", rec.Code, rec.Body, tc.want)
			}
			if client.saved != nil {
				t.Error("invalid input reached the User Profile service")
			}
		})
	}
}

func TestProfileWorksForStaffToo(t *testing.T) {
	rec := profileRequestTo(t, &profileTestClient{}, authv1.Role_ROLE_PHARMACY_STAFF, http.MethodGet, "", true)
	if rec.Code != http.StatusOK {
		t.Fatalf("staff GET status = %d, want 200", rec.Code)
	}
}

func TestProfileNeedsSignIn(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodPut} {
		rec := profileRequestTo(t, &profileTestClient{}, authv1.Role_ROLE_PATIENT, method, `{}`, false)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s without a token: status = %d, want 401", method, rec.Code)
		}
	}
}

func TestProfileServiceErrorsMapToHTTP(t *testing.T) {
	client := &profileTestClient{updateErr: status.Error(codes.Unavailable, "down")}
	rec := profileRequestTo(t, client, authv1.Role_ROLE_PATIENT, http.MethodPut, `{"displayName":"Karma","phone":"17123456"}`, true)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503 when User Profile is down", rec.Code)
	}
}
