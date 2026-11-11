// Package ports defines the interfaces the app layer needs from the outside world.
// Adapters (postgres, memory, clients) implement them; tests use fakes.
package ports

import (
	"context"
	"time"

	"github.com/Swiftmeds-bht/Swiftmeds/services/watchlist/internal/domain"
)

// SubscriptionRepository stores subscriptions and their alerts. Every
// implementation must pass the contract tests in internal/adapters/repotest.
type SubscriptionRepository interface {
	// Create saves s, unless the user already has an active (not yet notified)
	// subscription for the same product: then it returns that one and created=false.
	// The check and the insert are one atomic step.
	Create(ctx context.Context, s domain.Subscription) (got domain.Subscription, created bool, err error)
	// Delete removes the user's subscription and its alerts. It returns
	// domain.ErrNotFound if there is none with that ID owned by userID.
	Delete(ctx context.Context, subscriptionID, userID string) error
	// ListByUser returns all of the user's subscriptions, newest first.
	ListByUser(ctx context.Context, userID string) ([]domain.Subscription, error)
	// ActiveForProduct returns the subscriptions for productID that have not been notified yet.
	ActiveForProduct(ctx context.Context, productID string) ([]domain.Subscription, error)
	// Notify saves the alert and marks its subscription notified at alert.OccurredAt,
	// both or neither. It returns false (and saves nothing) if the subscription is
	// gone or was already notified, so a redelivered event cannot alert twice.
	Notify(ctx context.Context, alert domain.Alert) (bool, error)
	// PendingAlerts returns the user's alerts that have not been delivered yet, oldest first.
	PendingAlerts(ctx context.Context, userID string) ([]domain.Alert, error)
	// MarkDelivered records that an alert reached the user. Marking it again changes nothing.
	MarkDelivered(ctx context.Context, alertID string, at time.Time) error
}

// PharmacyLocator answers "is this pharmacy within radiusM of center?" (Geo).
type PharmacyLocator interface {
	WithinRadius(ctx context.Context, pharmacyID string, center domain.Point, radiusM int32) (bool, error)
}
