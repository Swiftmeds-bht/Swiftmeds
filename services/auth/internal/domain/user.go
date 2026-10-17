package domain

import (
	"errors"
	"strings"
	"time"
)

type Role string

const (
	RolePatient    Role = "PATIENT"
	RolePharmacist Role = "PHARMACIST"
	RoleAdmin      Role = "ADMIN"
)

var (
	ErrInvalidEmail       = errors.New("email is required and must be valid")
	ErrInvalidPassword    = errors.New("password must be at least 8 characters")
	ErrInvalidRole        = errors.New("role is invalid")
	ErrAdminRegistration  = errors.New("admin accounts cannot self-register")
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrUserExists         = errors.New("user already exists")
	ErrUserNotFound       = errors.New("user not found")
	ErrInvalidToken       = errors.New("invalid or expired token")
	ErrRefreshRevoked     = errors.New("refresh token is revoked")
)

type User struct {
	ID           string
	Email        string
	PasswordHash string
	Role         Role
	PharmacyID   string
	CreatedAt    time.Time
}

func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func ValidEmail(email string) bool {
	email = NormalizeEmail(email)
	at := strings.LastIndexByte(email, '@')
	return at > 0 && at < len(email)-1 && strings.Contains(email[at+1:], ".")
}

func ValidateRegistration(email, password string, role Role) error {
	if !ValidEmail(email) {
		return ErrInvalidEmail
	}
	if len(password) < 8 {
		return ErrInvalidPassword
	}
	if role != RolePatient && role != RolePharmacist {
		if role == RoleAdmin {
			return ErrAdminRegistration
		}
		return ErrInvalidRole
	}
	return nil
}
