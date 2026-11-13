// Package ports is everything Search needs from other services. The gRPC
// adapters in internal/adapters/clients implement them; tests use fakes.
package ports

import (
	"context"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/Swiftmeds-bht/Swiftmeds/services/search/internal/domain"
)

// Catalog resolves what the patient typed into products (controlled medicines excluded).
type Catalog interface {
	ResolveName(ctx context.Context, query string, limit int) ([]domain.Product, error)
}

type NearbyPharmacy struct {
	PharmacyID string
	DistanceM  int32
}

// Geo finds verified pharmacies near a point, nearest first.
type Geo interface {
	FindNearby(ctx context.Context, center domain.Point, radiusM int32, limit int) ([]NearbyPharmacy, error)
}

type Stock struct {
	domain.Key
	Available   int32
	ConfirmedAt time.Time
}

// Inventory returns the available stock of the products at the pharmacies.
type Inventory interface {
	AvailableStock(ctx context.Context, pharmacyIDs, productIDs []string) ([]Stock, error)
}

// Freshness returns a 0..1 confidence score per stock key. Missing keys mean "never confirmed".
type Freshness interface {
	Scores(ctx context.Context, keys []domain.Key) (map[domain.Key]float64, error)
}

// Pricing returns prices in chhertum. Missing keys mean "no price listed".
type Pricing interface {
	Prices(ctx context.Context, keys []domain.Key) (map[domain.Key]int64, error)
}

// Trust returns each pharmacy's 0..1 accuracy score.
type Trust interface {
	Scores(ctx context.Context, pharmacyIDs []string) (map[string]float64, error)
}

type PharmacyInfo struct {
	Name, Address string
	Location      *domain.Point
}

// Registry returns display details of pharmacies.
type Registry interface {
	Pharmacies(ctx context.Context, pharmacyIDs []string) (map[string]PharmacyInfo, error)
}

type RankCandidate struct {
	domain.Key
	DistanceM     int32
	Freshness     float64
	Trust         float64
	PriceChhertum int64
	Available     int32
}

type Ranked struct {
	domain.Key
	Score float64
}

// Ranking orders candidates, best first.
type Ranking interface {
	Rank(ctx context.Context, candidates []RankCandidate) ([]Ranked, error)
}

// SearchEvents publishes analytics events for completed searches.
type SearchEvents interface {
	SearchRecorded(ctx context.Context, searchID, dzongkhag string, result domain.ProductResult) error
}

// EventPublisher is the narrow interface implemented by messaging.Publisher.
type EventPublisher interface {
	Publish(ctx context.Context, routingKey string, event proto.Message) error
}

// Substitution returns products with the same molecule, strength and form.
type Substitution interface {
	Substitutes(ctx context.Context, productID string, limit int) ([]domain.Product, error)
}
