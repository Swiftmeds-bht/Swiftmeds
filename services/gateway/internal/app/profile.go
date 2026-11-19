package app

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"unicode/utf8"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	userprofilev1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/user-profile/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/gateway/internal/ports"
)

const (
	// ProfilePath is the signed-in user's own profile (SCRUM-137), as in
	// docs/api/openapi.yaml (getMyProfile, updateMyProfile).
	ProfilePath = "/api/v1/me/profile"

	maxDisplayNameLength = 80
	maxProfileBodyBytes  = 4 << 10
)

// The same rule as the User Profile service (8 to 15 digits, optional "+"),
// checked here so the app gets a clear message instead of "bad request".
var profilePhonePattern = regexp.MustCompile(`^\+?[1-9][0-9]{7,14}$`)

// Spaces, dashes, dots and brackets people type in phone numbers.
var phoneSeparators = strings.NewReplacer(" ", "", "-", "", ".", "", "(", "", ")", "")

type profileRequest struct {
	DisplayName string `json:"displayName"`
	Phone       string `json:"phone"`
}

// profileHandler serves GET and PUT of the caller's own profile. The user ID
// always comes from the token, so nobody can read or change another profile.
// GET answers 404 when the user has not saved a profile yet; PUT creates it or
// updates it.
func profileHandler(client ports.ProfileClient, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		identity := identityFrom(r)
		if identity.UserID == "" {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		ctx, cancel := callContext(r)
		defer cancel()

		if r.Method == http.MethodGet {
			resp, err := client.GetProfile(ctx, &userprofilev1.GetProfileRequest{UserId: identity.UserID})
			if status.Code(err) == codes.NotFound {
				// Normal for a new user, so not logged as an error.
				writeError(w, http.StatusNotFound, "profile not found")
				return
			}
			if err != nil {
				writeDownstreamError(w, logger, r, err)
				return
			}
			// The contract's response is the Profile itself, not a wrapper.
			writeProtoStatus(w, http.StatusOK, resp.GetProfile())
			return
		}

		var body profileRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxProfileBodyBytes)).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		name := strings.TrimSpace(body.DisplayName)
		phone := phoneSeparators.Replace(strings.TrimSpace(body.Phone))
		switch {
		case name == "":
			writeError(w, http.StatusBadRequest, "enter your name")
			return
		case utf8.RuneCountInString(name) > maxDisplayNameLength:
			writeError(w, http.StatusBadRequest, "use at most 80 characters for your name")
			return
		case !profilePhonePattern.MatchString(phone):
			writeError(w, http.StatusBadRequest, "enter a phone number with 8 to 15 digits, for example 17123456 or +97517123456")
			return
		}

		resp, err := client.UpdateProfile(ctx, &userprofilev1.UpdateProfileRequest{
			UserId: identity.UserID, DisplayName: name, Phone: phone,
		})
		if err != nil {
			writeDownstreamError(w, logger, r, err)
			return
		}
		writeProtoStatus(w, http.StatusOK, resp.GetProfile())
	}
}
