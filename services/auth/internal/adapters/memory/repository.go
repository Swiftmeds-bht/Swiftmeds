package memory

import (
	"context"
	"sync"
	"time"

	"github.com/Swiftmeds-bht/Swiftmeds/services/auth/internal/domain"
	"github.com/Swiftmeds-bht/Swiftmeds/services/auth/internal/ports"
)

var _ ports.UserRepository = (*Repository)(nil)

type Repository struct {
	mu      sync.Mutex
	users   map[string]domain.User
	byEmail map[string]string
	tokens  map[string]refreshToken
}

type refreshToken struct {
	userID    string
	expiresAt time.Time
	revoked   bool
}

func NewRepository() *Repository {
	return &Repository{users: map[string]domain.User{}, byEmail: map[string]string{}, tokens: map[string]refreshToken{}}
}
func (r *Repository) CreateUser(_ context.Context, u domain.User) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.users[u.ID]; ok {
		return domain.ErrUserExists
	}
	if _, ok := r.byEmail[u.Email]; ok {
		return domain.ErrUserExists
	}
	r.users[u.ID], r.byEmail[u.Email] = u, u.ID
	return nil
}
func (r *Repository) FindUserByEmail(_ context.Context, email string) (domain.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	id, ok := r.byEmail[email]
	if !ok {
		return domain.User{}, domain.ErrUserNotFound
	}
	return r.users[id], nil
}
func (r *Repository) FindUserByID(_ context.Context, id string) (domain.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	u, ok := r.users[id]
	if !ok {
		return domain.User{}, domain.ErrUserNotFound
	}
	return u, nil
}
func (r *Repository) SaveRefreshToken(_ context.Context, hash, userID string, exp time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.users[userID]; !ok {
		return domain.ErrUserNotFound
	}
	if _, ok := r.tokens[hash]; ok {
		return domain.ErrUserExists
	}
	r.tokens[hash] = refreshToken{userID: userID, expiresAt: exp}
	return nil
}
func (r *Repository) ConsumeRefreshToken(_ context.Context, hash string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	tok, ok := r.tokens[hash]
	if !ok || tok.revoked || !tok.expiresAt.After(time.Now()) {
		return "", domain.ErrRefreshRevoked
	}
	tok.revoked = true
	r.tokens[hash] = tok
	return tok.userID, nil
}
func (r *Repository) LinkPharmacy(_ context.Context, userID, pharmacyID string) (domain.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	u, ok := r.users[userID]
	switch {
	case !ok:
		return domain.User{}, domain.ErrUserNotFound
	case u.Role == domain.RoleAdmin:
		return domain.User{}, domain.ErrAdminLink
	case u.PharmacyID != "" && u.PharmacyID != pharmacyID:
		return domain.User{}, domain.ErrAlreadyLinked
	}
	u.Role, u.PharmacyID = domain.RolePharmacist, pharmacyID
	r.users[userID] = u
	return u, nil
}

func (r *Repository) RevokeRefreshToken(_ context.Context, hash string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	tok, ok := r.tokens[hash]
	if ok {
		tok.revoked = true
		r.tokens[hash] = tok
	}
	return nil
}
