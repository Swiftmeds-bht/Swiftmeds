package domain

import (
	"errors"
	"strings"
	"testing"
)

func TestNormalizeEmail(t *testing.T) {
	cases := map[string]string{
		"  Test@Example.COM ": "test@example.com",
		"a@b.bt":              "a@b.bt",
		"":                    "",
	}
	for in, want := range cases {
		if got := NormalizeEmail(in); got != want {
			t.Errorf("NormalizeEmail(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestValidEmail(t *testing.T) {
	cases := []struct {
		email string
		want  bool
	}{
		{"patient@example.com", true},
		{" Patient@Example.com ", true}, // normalised before checking
		{"a@b.bt", true},
		{"", false},
		{"no-at-sign.com", false},
		{"@example.com", false},      // empty local part
		{"patient@", false},          // empty domain
		{"patient@localhost", false}, // domain without a dot
		{"a@b@example.com", true},    // last @ splits; domain has a dot
	}
	for _, tc := range cases {
		if got := ValidEmail(tc.email); got != tc.want {
			t.Errorf("ValidEmail(%q) = %v, want %v", tc.email, got, tc.want)
		}
	}
}

// Password length boundaries: 8 is the minimum, 72 bytes is bcrypt's maximum.
func TestValidateRegistration(t *testing.T) {
	const email = "patient@example.com"
	pw := func(n int) string { return strings.Repeat("p", n) }

	cases := []struct {
		name     string
		email    string
		password string
		role     Role
		want     error
	}{
		{"valid patient", email, pw(12), RolePatient, nil},
		{"valid pharmacist", email, pw(12), RolePharmacist, nil},

		{"password 7 (below min)", email, pw(MinPasswordLength - 1), RolePatient, ErrInvalidPassword},
		{"password 8 (min)", email, pw(MinPasswordLength), RolePatient, nil},
		{"password 72 (max)", email, pw(MaxPasswordLength), RolePatient, nil},
		{"password 73 (above max)", email, pw(MaxPasswordLength + 1), RolePatient, ErrInvalidPassword},
		{"multi-byte password over 72 bytes", email, strings.Repeat("ཀ", 25), RolePatient, ErrInvalidPassword}, // 25 runes, 75 bytes

		{"invalid email", "not-an-email", pw(12), RolePatient, ErrInvalidEmail},
		{"email checked before password", "not-an-email", pw(1), RolePatient, ErrInvalidEmail},

		{"admin cannot self-register", email, pw(12), RoleAdmin, ErrAdminRegistration},
		{"unknown role", email, pw(12), Role("SUPERUSER"), ErrInvalidRole},
		{"empty role", email, pw(12), Role(""), ErrInvalidRole},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := ValidateRegistration(tc.email, tc.password, tc.role); !errors.Is(err, tc.want) {
				t.Errorf("ValidateRegistration() = %v, want %v", err, tc.want)
			}
		})
	}
}
