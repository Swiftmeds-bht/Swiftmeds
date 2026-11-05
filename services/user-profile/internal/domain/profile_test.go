package domain

import (
	"errors"
	"testing"
	"time"
)

func TestNewProfileValidation(t *testing.T) {
	now := time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		uid  string
		n    string
		p    string
		err  error
	}{
		{"valid", "user-1", "Tashi", "+97517123456", nil},
		{"empty user id", "", "Tashi", "+97517123456", ErrMissingUserID},
		{"empty name", "user-1", " ", "+97517123456", ErrMissingName},
		{"invalid phone", "user-1", "Tashi", "not-a-phone", ErrInvalidPhone},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewProfile(tt.uid, tt.n, tt.p, "Thimphu", "en", now)
			if !errors.Is(err, tt.err) {
				t.Fatalf("error = %v, want %v", err, tt.err)
			}
			if tt.err == nil && (got.UserID != tt.uid || !got.CreatedAt.Equal(now) || got.Name != tt.n) {
				t.Fatalf("profile = %+v", got)
			}
		})
	}
}

func TestProfileValidate(t *testing.T) {
	valid := Profile{UserID: "u1", Name: "Tashi", Phone: "+97517123456"}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid profile error = %v", err)
	}
	for name, profile := range map[string]Profile{
		"missing user": {Name: "Tashi", Phone: "+97517123456"},
		"missing name": {UserID: "u1", Phone: "+97517123456"},
		"bad phone":    {UserID: "u1", Name: "Tashi", Phone: "bad"},
	} {
		t.Run(name, func(t *testing.T) {
			if err := profile.Validate(); err == nil {
				t.Fatal("Validate() succeeded for invalid profile")
			}
		})
	}
}
