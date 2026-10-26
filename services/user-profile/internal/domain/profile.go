package domain

import (
	"errors"
	"regexp"
	"strings"
	"time"
)

var (
	ErrMissingUserID   = errors.New("user_id is required")
	ErrMissingName     = errors.New("name is required")
	ErrInvalidPhone    = errors.New("phone number is invalid")
	ErrProfileNotFound = errors.New("profile not found")
	ErrProfileExists   = errors.New("profile already exists")
)

var phonePattern = regexp.MustCompile(`^\+?[1-9][0-9]{7,14}$`)

type Profile struct {
	UserID            string
	Name              string
	Phone             string
	HomeLocation      string
	PreferredLanguage string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

func NewProfile(userID, name, phone, homeLocation, preferredLanguage string, now time.Time) (Profile, error) {
	userID = strings.TrimSpace(userID)
	name = strings.TrimSpace(name)
	phone = strings.TrimSpace(phone)
	if userID == "" {
		return Profile{}, ErrMissingUserID
	}
	if name == "" {
		return Profile{}, ErrMissingName
	}
	if !phonePattern.MatchString(phone) {
		return Profile{}, ErrInvalidPhone
	}
	return Profile{
		UserID: userID, Name: name, Phone: phone, HomeLocation: strings.TrimSpace(homeLocation),
		PreferredLanguage: strings.TrimSpace(preferredLanguage), CreatedAt: now, UpdatedAt: now,
	}, nil
}

func (p Profile) Validate() error {
	if strings.TrimSpace(p.UserID) == "" {
		return ErrMissingUserID
	}
	if strings.TrimSpace(p.Name) == "" {
		return ErrMissingName
	}
	if !phonePattern.MatchString(strings.TrimSpace(p.Phone)) {
		return ErrInvalidPhone
	}
	return nil
}
