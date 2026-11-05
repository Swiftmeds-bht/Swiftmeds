// Package app holds Holds' use cases. It depends only on domain and ports.
package app

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"time"

	"github.com/Swiftmeds-bht/Swiftmeds/services/holds/internal/domain"
	"github.com/Swiftmeds-bht/Swiftmeds/services/holds/internal/ports"
)

// DefaultMaxActivePerUser is how many open holds (PENDING or ACTIVE) one patient may have.
const DefaultMaxActivePerUser = 3

// DefaultHoldDuration is how long an ACTIVE hold lasts before it expires.
const DefaultHoldDuration = 2 * time.Hour

// DefaultPageSize is the page size of ListHolds when the caller does not ask for one.
const DefaultPageSize = 20

// MaxPageSize is the largest page ListHolds returns.
const MaxPageSize = 100

// maxCodeAttempts is how many pickup codes ReserveHold tries before it gives up.
// A clash is rare (one in a billion per pair), so more than one retry is already unlikely.
const maxCodeAttempts = 5

// Config holds the settings the use cases need.
type Config struct {
	// MaxActivePerUser is how many open holds (PENDING or ACTIVE) one patient may have at once.
	MaxActivePerUser int

	// HoldDuration is how long an ACTIVE hold lasts before it expires.
	HoldDuration time.Duration
}

// Service implements the Holds use cases.
type Service struct {
	repo    ports.HoldRepository
	cfg     Config
	now     func() time.Time
	newID   func() (string, error)
	newCode func() (string, error)
}

// Option changes how a Service is built. Tests use them to fix IDs and codes.
type Option func(*Service)

// WithIDGenerator replaces the hold ID generator (a random UUID by default).
func WithIDGenerator(f func() (string, error)) Option {
	return func(s *Service) { s.newID = f }
}

// WithCodeGenerator replaces the pickup code generator (random codes by default).
func WithCodeGenerator(f func() (string, error)) Option {
	return func(s *Service) { s.newCode = f }
}

// NewService wires the use cases. Pass time.Now in production and a fixed clock in tests.
func NewService(repo ports.HoldRepository, cfg Config, now func() time.Time, opts ...Option) *Service {
	s := &Service{repo: repo, cfg: cfg, now: now, newID: newUUID, newCode: newPickupCode}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// CreateHold validates the request and stores a PENDING hold. It fails with
// domain.ErrLimitReached when the patient already has the most open holds
// that are allowed. A PENDING hold becomes ACTIVE through ReserveHold.
func (s *Service) CreateHold(ctx context.Context, userID, pharmacyID, productID string, quantity int) (domain.Hold, error) {
	id, err := s.newID()
	if err != nil {
		return domain.Hold{}, err
	}
	h, err := domain.NewHold(id, userID, pharmacyID, productID, quantity, s.now())
	if err != nil {
		return domain.Hold{}, err
	}
	if err := s.repo.Create(ctx, h, s.cfg.MaxActivePerUser); err != nil {
		return domain.Hold{}, fmt.Errorf("create hold: %w", err)
	}
	return h, nil
}

// GetHold returns one hold, or domain.ErrNotFound.
func (s *Service) GetHold(ctx context.Context, id string) (domain.Hold, error) {
	if id == "" {
		return domain.Hold{}, domain.ErrMissingHoldID
	}
	h, err := s.repo.Get(ctx, id)
	if err != nil {
		return domain.Hold{}, fmt.Errorf("get hold: %w", err)
	}
	return h, nil
}

// ListQuery asks for the holds of one patient or one pharmacy.
type ListQuery struct {
	UserID     string
	PharmacyID string
	// Status is empty to list every status.
	Status    domain.Status
	PageSize  int
	PageToken string
}

// ListResult is one page of holds, newest first.
type ListResult struct {
	Holds []domain.Hold
	// NextPageToken is empty on the last page.
	NextPageToken string
}

// ListHolds lists the holds of a patient (UserID) or of a pharmacy (PharmacyID).
// Exactly one of the two must be set.
func (s *Service) ListHolds(ctx context.Context, q ListQuery) (ListResult, error) {
	if (q.UserID == "") == (q.PharmacyID == "") {
		return ListResult{}, domain.ErrInvalidListFilter
	}
	if q.Status != "" && !q.Status.Valid() {
		return ListResult{}, domain.ErrInvalidListFilter
	}
	after, err := decodeCursor(q.PageToken)
	if err != nil {
		return ListResult{}, err
	}
	size := q.PageSize
	if size <= 0 {
		size = DefaultPageSize
	}
	if size > MaxPageSize {
		size = MaxPageSize
	}

	// One more than the page, to find out whether there is a next page.
	holds, err := s.repo.List(ctx, domain.ListFilter{
		UserID:     q.UserID,
		PharmacyID: q.PharmacyID,
		Status:     q.Status,
		After:      after,
		Limit:      size + 1,
	})
	if err != nil {
		return ListResult{}, fmt.Errorf("list holds: %w", err)
	}
	if len(holds) <= size {
		return ListResult{Holds: holds}, nil
	}
	page := holds[:size]
	last := page[size-1]
	return ListResult{
		Holds:         page,
		NextPageToken: encodeCursor(domain.Cursor{CreatedAt: last.CreatedAt, ID: last.ID}),
	}, nil
}

// ReserveHold is the stock.reserved event: PENDING -> ACTIVE. It makes the
// pickup code and fixes the expiry time. The reservation saga (SCRUM-61)
// calls it when Inventory has reserved the stock.
func (s *Service) ReserveHold(ctx context.Context, id string) (domain.Hold, error) {
	h, err := s.GetHold(ctx, id)
	if err != nil {
		return domain.Hold{}, err
	}
	for range maxCodeAttempts {
		code, err := s.newCode()
		if err != nil {
			return domain.Hold{}, err
		}
		active, err := h.Reserve(s.now(), s.cfg.HoldDuration, code)
		if err != nil {
			return domain.Hold{}, err
		}
		err = s.repo.Update(ctx, active, domain.Pending)
		if err == nil {
			return active, nil
		}
		if !errors.Is(err, domain.ErrPickupCodeTaken) {
			return domain.Hold{}, fmt.Errorf("reserve hold: %w", err)
		}
		// Another ACTIVE hold at this pharmacy has the same code: try a new one.
	}
	return domain.Hold{}, fmt.Errorf("reserve hold: %w", domain.ErrPickupCodeTaken)
}

// RejectHold is the stock.unavailable event: PENDING -> REJECTED.
func (s *Service) RejectHold(ctx context.Context, id, reason string) (domain.Hold, error) {
	return s.change(ctx, id, func(h domain.Hold) (domain.Hold, error) {
		return h.Reject(s.now(), reason)
	})
}

// CancelHold lets the patient who made the hold cancel it.
func (s *Service) CancelHold(ctx context.Context, id, userID, reason string) (domain.Hold, error) {
	if userID == "" {
		return domain.Hold{}, domain.ErrMissingUserID
	}
	return s.change(ctx, id, func(h domain.Hold) (domain.Hold, error) {
		if h.UserID != userID {
			return domain.Hold{}, domain.ErrNotHoldOwner
		}
		return h.Cancel(s.now(), reason)
	})
}

// CollectHold hands the medicine over. It needs the hold's pharmacy and the
// patient's pickup code. A wrong code is refused and the hold stays ACTIVE;
// a hold that was already collected (or is not ACTIVE) cannot be collected.
func (s *Service) CollectHold(ctx context.Context, id, pharmacyID, pickupCode string) (domain.Hold, error) {
	return s.change(ctx, id, func(h domain.Hold) (domain.Hold, error) {
		return h.Collect(s.now(), pharmacyID, pickupCode)
	})
}

// ExpireHold is called by Expiry once expires_at has passed. It does nothing
// (and returns the hold as it is) unless the hold is ACTIVE, so Expiry may
// call it again safely. It fails with domain.ErrNotYetExpired if called early.
func (s *Service) ExpireHold(ctx context.Context, id string) (domain.Hold, error) {
	h, err := s.GetHold(ctx, id)
	if err != nil {
		return domain.Hold{}, err
	}
	if h.Status != domain.Active {
		return h, nil
	}
	now := s.now()
	if now.Before(h.ExpiresAt) {
		return domain.Hold{}, domain.ErrNotYetExpired
	}
	expired, err := h.Expire(now)
	if err != nil {
		return domain.Hold{}, err
	}
	if err := s.repo.Update(ctx, expired, domain.Active); err != nil {
		if errors.Is(err, domain.ErrConflict) {
			// Collected or cancelled at the same moment: nothing is left to expire.
			return s.GetHold(ctx, id)
		}
		return domain.Hold{}, fmt.Errorf("expire hold: %w", err)
	}
	return expired, nil
}

// change reads the hold, applies one transition, and stores the result only if
// nobody changed the hold in between. If someone did, it returns domain.ErrConflict.
func (s *Service) change(ctx context.Context, id string, step func(domain.Hold) (domain.Hold, error)) (domain.Hold, error) {
	h, err := s.GetHold(ctx, id)
	if err != nil {
		return domain.Hold{}, err
	}
	next, err := step(h)
	if err != nil {
		return domain.Hold{}, err
	}
	if err := s.repo.Update(ctx, next, h.Status); err != nil {
		return domain.Hold{}, fmt.Errorf("update hold: %w", err)
	}
	return next, nil
}

// newUUID makes a random (version 4) UUID.
func newUUID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("generate hold id: %w", err)
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:]), nil
}

func newPickupCode() (string, error) {
	return domain.NewPickupCode(rand.Reader)
}
