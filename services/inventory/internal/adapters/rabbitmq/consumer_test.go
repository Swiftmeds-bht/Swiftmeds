package rabbitmq

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Swiftmeds-bht/Swiftmeds/pkg/messaging"
	eventsv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/events/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/inventory/internal/app"
	"github.com/Swiftmeds-bht/Swiftmeds/services/inventory/internal/domain"
	"github.com/Swiftmeds-bht/Swiftmeds/services/inventory/internal/ports"
	amqp "github.com/rabbitmq/amqp091-go"
	"google.golang.org/protobuf/proto"
)

var errConsumerTest = errors.New("reservation test failure")

type consumerRepo struct {
	reserveFn func(context.Context, string, ports.StockKey, int32) (domain.Stock, bool, error)
}

func (r *consumerRepo) Get(context.Context, ports.StockKey) (domain.Stock, bool, error) {
	return domain.Stock{}, false, nil
}

func (r *consumerRepo) GetMany(context.Context, []string, []string, bool) ([]domain.Stock, error) {
	return nil, nil
}

func (r *consumerRepo) ListByPharmacy(context.Context, string, int32, string) ([]domain.Stock, string, error) {
	return nil, "", nil
}

func (r *consumerRepo) Set(context.Context, domain.Stock, string) (int32, domain.Stock, error) {
	return 0, domain.Stock{}, nil
}

func (r *consumerRepo) BulkSet(context.Context, string, []ports.StockUpdate, string) ([]ports.StockChange, []ports.StockUpdateError, error) {
	return nil, nil, nil
}

func (r *consumerRepo) Reserve(ctx context.Context, hold string, key ports.StockKey, quantity int32) (domain.Stock, bool, error) {
	if r.reserveFn != nil {
		return r.reserveFn(ctx, hold, key, quantity)
	}
	return domain.Stock{}, false, nil
}

func (r *consumerRepo) CommitReservation(context.Context, string) (domain.Stock, error) {
	return domain.Stock{}, nil
}

func (r *consumerRepo) ReleaseReservation(context.Context, string) (domain.Stock, error) {
	return domain.Stock{}, nil
}

type consumerPublisher struct{}

func (consumerPublisher) PublishStockReserved(context.Context, string, string, string, int32) error {
	return nil
}

func (consumerPublisher) PublishStockUnavailable(context.Context, string, string, string, int32, int32) error {
	return nil
}

func (consumerPublisher) PublishStockIncreased(context.Context, string, string, int32, int32) error {
	return nil
}

func consumerTestService(t *testing.T, repo *consumerRepo) *app.Service {
	t.Helper()

	service, err := app.NewService(repo, consumerPublisher{}, time.Now)
	if err != nil {
		t.Fatalf("create app service: %v", err)
	}
	return service
}

func TestHoldRequestedConsumerRejectsNilServiceAndStartOnNil(t *testing.T) {
	if _, err := NewHoldRequestedConsumer(nil, nil, nil); err == nil {
		t.Fatal("constructor accepted a nil service")
	}

	var consumer *HoldRequestedConsumer
	if err := consumer.Start(context.Background()); err == nil {
		t.Fatal("Start on nil consumer returned no error")
	}

	if err := (&HoldRequestedConsumer{}).Start(context.Background()); err == nil {
		t.Fatal("Start without underlying consumer returned no error")
	}
}

func TestHandleHoldRequestedValidatesAndDecodesEvent(t *testing.T) {
	service := consumerTestService(t, &consumerRepo{})

	err := handleHoldRequested(
		context.Background(),
		amqp.Delivery{ContentType: "text/plain"},
		service,
	)
	if err == nil {
		t.Fatal("handler accepted a non-protobuf delivery")
	}

	err = handleHoldRequested(
		context.Background(),
		amqp.Delivery{
			ContentType: messaging.ProtobufType,
			Body:        []byte{0xff},
		},
		service,
	)
	if err == nil {
		t.Fatal("handler accepted an invalid protobuf body")
	}

	err = handleHoldRequested(
		context.Background(),
		amqp.Delivery{ContentType: messaging.ProtobufType},
		nil,
	)
	if err == nil {
		t.Fatal("handler accepted a nil service")
	}
}

func TestHandleHoldRequestedReservesStock(t *testing.T) {
	var gotHold string
	var gotKey ports.StockKey
	var gotQuantity int32

	stock, err := domain.NewStock(
		"pharmacy-1",
		"product-1",
		10,
		2,
		time.Now(),
	)
	if err != nil {
		t.Fatalf("create stock: %v", err)
	}

	repo := &consumerRepo{
		reserveFn: func(
			_ context.Context,
			hold string,
			key ports.StockKey,
			quantity int32,
		) (domain.Stock, bool, error) {
			gotHold = hold
			gotKey = key
			gotQuantity = quantity
			return stock, true, nil
		},
	}
	service := consumerTestService(t, repo)

	body, err := proto.Marshal(&eventsv1.HoldRequested{
		HoldId:     "hold-1",
		PharmacyId: "pharmacy-1",
		ProductId:  "product-1",
		Quantity:   2,
	})
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}

	delivery := amqp.Delivery{
		ContentType: messaging.ProtobufType,
		Body:        body,
	}
	if err := handleHoldRequested(context.Background(), delivery, service); err != nil {
		t.Fatalf("handle event: %v", err)
	}

	wantKey := ports.StockKey{
		PharmacyID: "pharmacy-1",
		ProductID:  "product-1",
	}
	if gotHold != "hold-1" || gotKey != wantKey || gotQuantity != 2 {
		t.Fatalf("Reserve args = %q, %+v, %d", gotHold, gotKey, gotQuantity)
	}
}

func TestHandleHoldRequestedReturnsReservationError(t *testing.T) {
	repo := &consumerRepo{
		reserveFn: func(context.Context, string, ports.StockKey, int32) (domain.Stock, bool, error) {
			return domain.Stock{}, false, errConsumerTest
		},
	}
	service := consumerTestService(t, repo)

	body, err := proto.Marshal(&eventsv1.HoldRequested{
		HoldId:     "hold-1",
		PharmacyId: "pharmacy-1",
		ProductId:  "product-1",
		Quantity:   1,
	})
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}

	err = handleHoldRequested(
		context.Background(),
		amqp.Delivery{
			ContentType: messaging.ProtobufType,
			Body:        body,
		},
		service,
	)
	if !errors.Is(err, errConsumerTest) {
		t.Fatalf("handler error = %v, want wrapped reservation error", err)
	}
}
