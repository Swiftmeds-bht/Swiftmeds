package app

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"

	"github.com/Swiftmeds-bht/Swiftmeds/services/auth/internal/domain"
	"github.com/Swiftmeds-bht/Swiftmeds/services/auth/internal/ports"
)

const (
	defaultAccessTTL  = 15 * time.Minute
	defaultRefreshTTL = 30 * 24 * time.Hour
)

type TokenPair struct {
	AccessToken          string
	RefreshToken         string
	AccessTokenExpiresAt time.Time
}

type Service struct {
	repo       ports.UserRepository
	secret     []byte
	issuer     string
	accessTTL  time.Duration
	refreshTTL time.Duration
	now        func() time.Time
}

func NewService(repo ports.UserRepository, secret []byte, issuer string, accessTTL, refreshTTL time.Duration, now func() time.Time) *Service {
	if accessTTL <= 0 {
		accessTTL = defaultAccessTTL
	}
	if refreshTTL <= 0 {
		refreshTTL = defaultRefreshTTL
	}
	if now == nil {
		now = time.Now
	}
	return &Service{
		repo: repo, secret: append([]byte(nil), secret...), issuer: issuer,
		accessTTL: accessTTL, refreshTTL: refreshTTL, now: now,
	}
}

func (s *Service) Register(ctx context.Context, email, password string, role domain.Role) (domain.User, error) {
	if err := domain.ValidateRegistration(email, password, role); err != nil {
		return domain.User{}, err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return domain.User{}, fmt.Errorf("hash password: %w", err)
	}
	id, err := newID()
	if err != nil {
		return domain.User{}, fmt.Errorf("generate user id: %w", err)
	}
	user := domain.User{
		ID: id, Email: domain.NormalizeEmail(email), PasswordHash: string(hash),
		Role: role, CreatedAt: s.now(),
	}
	if err := s.repo.CreateUser(ctx, user); err != nil {
		return domain.User{}, err
	}
	return user, nil
}

func (s *Service) Login(ctx context.Context, email, password string) (domain.User, TokenPair, error) {
	user, err := s.repo.FindUserByEmail(ctx, domain.NormalizeEmail(email))
	switch {
	case errors.Is(err, domain.ErrUserNotFound):
		// Spend the same bcrypt time as for a real account, so the response
		// time does not reveal which emails are registered.
		_ = bcrypt.CompareHashAndPassword(dummyHash(), []byte(password))
		return domain.User{}, TokenPair{}, domain.ErrInvalidCredentials
	case err != nil:
		return domain.User{}, TokenPair{}, fmt.Errorf("find user: %w", err)
	}
	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)) != nil {
		return domain.User{}, TokenPair{}, domain.ErrInvalidCredentials
	}
	tokens, err := s.issueTokens(ctx, user)
	if err != nil {
		return domain.User{}, TokenPair{}, err
	}
	return user, tokens, nil
}

func (s *Service) Refresh(ctx context.Context, refreshToken string) (TokenPair, error) {
	claims, err := s.parseToken(refreshToken, "refresh")
	if err != nil {
		return TokenPair{}, domain.ErrInvalidToken
	}
	userID, err := s.repo.ConsumeRefreshToken(ctx, hashToken(refreshToken))
	switch {
	case errors.Is(err, domain.ErrRefreshRevoked):
		return TokenPair{}, domain.ErrRefreshRevoked
	case err != nil:
		return TokenPair{}, fmt.Errorf("consume refresh token: %w", err)
	case userID != claims.Subject:
		return TokenPair{}, domain.ErrInvalidToken
	}
	user, err := s.repo.FindUserByID(ctx, userID)
	switch {
	case errors.Is(err, domain.ErrUserNotFound):
		return TokenPair{}, domain.ErrInvalidToken
	case err != nil:
		return TokenPair{}, fmt.Errorf("find user: %w", err)
	}
	return s.issueTokens(ctx, user)
}

// Logout revokes a refresh token so it can no longer be used. Logging out with
// a token that is already revoked succeeds, so clients can safely retry.
func (s *Service) Logout(ctx context.Context, refreshToken string) error {
	if _, err := s.parseToken(refreshToken, "refresh"); err != nil {
		return domain.ErrInvalidToken
	}
	if err := s.repo.RevokeRefreshToken(ctx, hashToken(refreshToken)); err != nil {
		return fmt.Errorf("revoke refresh token: %w", err)
	}
	return nil
}

type TokenIdentity struct {
	UserID     string
	Role       domain.Role
	PharmacyID string
	ExpiresAt  time.Time
}

func (s *Service) ValidateToken(_ context.Context, accessToken string) (TokenIdentity, error) {
	claims, err := s.parseToken(accessToken, "access")
	if err != nil {
		return TokenIdentity{}, domain.ErrInvalidToken
	}
	role, ok := roleFromClaim(claims.Role)
	if !ok {
		return TokenIdentity{}, domain.ErrInvalidToken
	}
	return TokenIdentity{
		UserID: claims.Subject, Role: role, PharmacyID: claims.PharmacyID, ExpiresAt: claims.ExpiresAt,
	}, nil
}

type parsedClaims struct {
	Subject    string
	Role       string
	PharmacyID string
	ExpiresAt  time.Time
}

func (s *Service) parseToken(raw, expectedType string) (parsedClaims, error) {
	if len(s.secret) == 0 {
		return parsedClaims{}, domain.ErrInvalidToken
	}
	token, err := jwt.Parse(raw, func(token *jwt.Token) (any, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, domain.ErrInvalidToken
		}
		return s.secret, nil
	}, jwt.WithIssuer(s.issuer), jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}))
	if err != nil || !token.Valid {
		return parsedClaims{}, domain.ErrInvalidToken
	}
	m, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return parsedClaims{}, domain.ErrInvalidToken
	}
	typ, ok := m["type"].(string)
	if !ok || typ != expectedType {
		return parsedClaims{}, domain.ErrInvalidToken
	}
	sub, ok := m["sub"].(string)
	if !ok || sub == "" {
		return parsedClaims{}, domain.ErrInvalidToken
	}
	exp, ok := m["exp"].(float64)
	if !ok {
		return parsedClaims{}, domain.ErrInvalidToken
	}
	role, _ := m["role"].(string)
	pharmacyID, _ := m["pharmacy_id"].(string)
	return parsedClaims{
		Subject: sub, Role: role, PharmacyID: pharmacyID, ExpiresAt: time.Unix(int64(exp), 0),
	}, nil
}

func (s *Service) issueTokens(ctx context.Context, user domain.User) (TokenPair, error) {
	now := s.now()
	accessExp := now.Add(s.accessTTL)
	refreshExp := now.Add(s.refreshTTL)
	access, err := s.sign(user, "access", accessExp)
	if err != nil {
		return TokenPair{}, err
	}
	refresh, err := s.sign(user, "refresh", refreshExp)
	if err != nil {
		return TokenPair{}, err
	}
	if err := s.repo.SaveRefreshToken(ctx, hashToken(refresh), user.ID, refreshExp); err != nil {
		return TokenPair{}, fmt.Errorf("save refresh token: %w", err)
	}
	return TokenPair{AccessToken: access, RefreshToken: refresh, AccessTokenExpiresAt: accessExp}, nil
}

func (s *Service) sign(user domain.User, tokenType string, expiresAt time.Time) (string, error) {
	id, err := newID()
	if err != nil {
		return "", err
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"iss": s.issuer, "sub": user.ID, "role": string(user.Role), "pharmacy_id": user.PharmacyID,
		"type": tokenType, "iat": s.now().Unix(), "exp": expiresAt.Unix(), "jti": id,
	}).SignedString(s.secret)
}

// dummyHash is a bcrypt hash of a random value, computed once. Login compares
// against it when the email is unknown.
var dummyHash = sync.OnceValue(func() []byte {
	h, err := bcrypt.GenerateFromPassword([]byte("swiftmed-timing-equaliser"), bcrypt.DefaultCost)
	if err != nil {
		panic(fmt.Sprintf("auth: generate dummy bcrypt hash: %v", err))
	}
	return h
})

func newID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func roleFromClaim(role string) (domain.Role, bool) {
	switch domain.Role(role) {
	case domain.RolePatient, domain.RolePharmacist, domain.RoleAdmin:
		return domain.Role(role), true
	default:
		return "", false
	}
}
