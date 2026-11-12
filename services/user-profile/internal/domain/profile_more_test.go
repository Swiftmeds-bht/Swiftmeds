package domain

import (
	"errors"
	"testing"
	"time"
)

// Phone numbers are E.164-style: an optional +, a first digit 1-9, then 7 to 14
// more digits (8 to 15 digits in all).
func TestPhoneNumberRules(t *testing.T) {
	tests := []struct {
		phone string
		valid bool
	}{
		{"+97517123456", true},
		{"97517123456", true},
		{"17123456", true},          // 8 digits: the shortest allowed
		{"1712345", false},          // 7 digits
		{"123456789012345", true},   // 15 digits: the longest allowed
		{"+123456789012345", true},  // 15 digits with +
		{"1234567890123456", false}, // 16 digits
		{"07123456", false},         // leading zero
		{"+07123456", false},        // leading zero after +
		{"++97517123456", false},    // two plus signs
		{"+975 17123456", false},    // inner space
		{"+975-17123456", false},    // dash
		{"9751712345a", false},      // letter
		{"", false},
		{"  +97517123456  ", true}, // surrounding spaces are trimmed
	}
	for _, tt := range tests {
		t.Run(tt.phone, func(t *testing.T) {
			_, err := NewProfile("u-1", "Tashi", tt.phone, "", "", time.Time{})
			if tt.valid && err != nil {
				t.Fatalf("NewProfile(%q) error = %v, want valid", tt.phone, err)
			}
			if !tt.valid && !errors.Is(err, ErrInvalidPhone) {
				t.Fatalf("NewProfile(%q) error = %v, want ErrInvalidPhone", tt.phone, err)
			}
			verr := Profile{UserID: "u-1", Name: "Tashi", Phone: tt.phone}.Validate()
			if tt.valid != (verr == nil) {
				t.Fatalf("Validate(%q) = %v, want valid=%v (must agree with NewProfile)", tt.phone, verr, tt.valid)
			}
		})
	}
}

func TestNewProfileTrimsAndKeepsEveryField(t *testing.T) {
	now := time.Date(2026, 11, 3, 9, 0, 0, 0, time.UTC)
	got, err := NewProfile("  u-1 ", "\tTashi Dorji ", " +97517123456 ", " Thimphu ", " dz ", now)
	if err != nil {
		t.Fatalf("NewProfile: %v", err)
	}
	want := Profile{
		UserID: "u-1", Name: "Tashi Dorji", Phone: "+97517123456",
		HomeLocation: "Thimphu", PreferredLanguage: "dz", CreatedAt: now, UpdatedAt: now,
	}
	if got != want {
		t.Fatalf("profile = %+v\nwant      %+v", got, want)
	}
}

func TestNewProfileOptionalFieldsMayBeEmpty(t *testing.T) {
	got, err := NewProfile("u-1", "Tashi", "+97517123456", "  ", "", time.Time{})
	if err != nil {
		t.Fatalf("NewProfile: %v", err)
	}
	if got.HomeLocation != "" || got.PreferredLanguage != "" {
		t.Fatalf("profile = %+v, want empty location and language", got)
	}
}

// Checks run in order: user, then name, then phone.
func TestNewProfileReportsTheFirstProblem(t *testing.T) {
	if _, err := NewProfile(" ", "", "bad", "", "", time.Time{}); !errors.Is(err, ErrMissingUserID) {
		t.Fatalf("error = %v, want ErrMissingUserID first", err)
	}
	if _, err := NewProfile("u-1", "", "bad", "", "", time.Time{}); !errors.Is(err, ErrMissingName) {
		t.Fatalf("error = %v, want ErrMissingName before the phone check", err)
	}
	if got, err := NewProfile("u-1", "Tashi", "bad", "", "", time.Now()); got != (Profile{}) || !errors.Is(err, ErrInvalidPhone) {
		t.Fatalf("got (%+v, %v), want (zero profile, ErrInvalidPhone)", got, err)
	}
}

func TestValidateReturnsTheMatchingError(t *testing.T) {
	tests := []struct {
		name    string
		profile Profile
		want    error
	}{
		{"blank user", Profile{UserID: "  ", Name: "Tashi", Phone: "+97517123456"}, ErrMissingUserID},
		{"blank name", Profile{UserID: "u-1", Name: "\t", Phone: "+97517123456"}, ErrMissingName},
		{"bad phone", Profile{UserID: "u-1", Name: "Tashi", Phone: "12"}, ErrInvalidPhone},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.profile.Validate(); !errors.Is(err, tt.want) {
				t.Fatalf("Validate() = %v, want %v", err, tt.want)
			}
		})
	}
}
