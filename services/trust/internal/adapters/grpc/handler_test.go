package grpc

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	trustv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/trust/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/trust/internal/adapters/memory"
	"github.com/Swiftmeds-bht/Swiftmeds/services/trust/internal/app"
	"github.com/Swiftmeds-bht/Swiftmeds/services/trust/internal/domain"
)

var now = time.Date(2026, 11, 5, 9, 0, 0, 0, time.UTC)

// newClient serves a Handler over an in-memory connection, so the tests go
// through real gRPC encoding.
func newClient(t *testing.T) (trustv1.TrustServiceClient, *app.Service) {
	t.Helper()
	svc, err := app.NewService(memory.NewRepository(), domain.DefaultParams, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	lis := bufconn.Listen(1 << 20)
	srv := grpc.NewServer()
	trustv1.RegisterTrustServiceServer(srv, NewHandler(svc))
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)

	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return lis.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return trustv1.NewTrustServiceClient(conn), svc
}

func TestGetTrustForANewPharmacyIsNeutral(t *testing.T) {
	c, _ := newClient(t)
	resp, err := c.GetTrust(context.Background(), &trustv1.GetTrustRequest{PharmacyId: "ph-new"})
	if err != nil {
		t.Fatal(err)
	}
	s := resp.GetScore()
	if s.GetPharmacyId() != "ph-new" || s.GetScore() != 0.5 || s.GetFeedbackCount() != 0 || s.GetUpdatedAt() != nil {
		t.Errorf("score = %+v, want neutral 0.5 without updated_at", s)
	}
}

func TestBatchGetTrustReturnsEveryRequestedPharmacy(t *testing.T) {
	c, svc := newClient(t)
	at := now.Add(-time.Hour)
	if _, err := svc.HandleFeedbackSubmitted(context.Background(), domain.Feedback{ID: "fb-1", PharmacyID: "ph-1", WasInStock: false, OccurredAt: at}); err != nil {
		t.Fatal(err)
	}

	resp, err := c.BatchGetTrust(context.Background(), &trustv1.BatchGetTrustRequest{PharmacyIds: []string{"ph-1", "ph-2"}})
	if err != nil {
		t.Fatal(err)
	}
	scores := resp.GetScores()
	if len(scores) != 2 || scores[0].GetPharmacyId() != "ph-1" || scores[1].GetPharmacyId() != "ph-2" {
		t.Fatalf("scores = %v", scores)
	}
	ph1 := scores[0]
	if ph1.GetScore() >= 0.5 || ph1.GetScore() <= 0 || ph1.GetFeedbackCount() != 1 || ph1.GetInStockCount() != 0 ||
		!ph1.GetUpdatedAt().AsTime().Equal(at) {
		t.Errorf("ph-1 = %+v, want a score below neutral, one answer, updated_at %v", ph1, at)
	}
	if scores[1].GetScore() != 0.5 {
		t.Errorf("ph-2 = %v, want neutral", scores[1].GetScore())
	}

	empty, err := c.BatchGetTrust(context.Background(), &trustv1.BatchGetTrustRequest{})
	if err != nil || len(empty.GetScores()) != 0 {
		t.Errorf("empty batch = %v, %v", empty, err)
	}
}

func TestErrorCodes(t *testing.T) {
	c, _ := newClient(t)
	ctx := context.Background()
	tooMany := strings.Split(strings.Repeat("ph,", domain.MaxBatch+1), ",")[:domain.MaxBatch+1]
	tests := []struct {
		name string
		call func() error
	}{
		{"get without pharmacy", func() error {
			_, err := c.GetTrust(ctx, &trustv1.GetTrustRequest{})
			return err
		}},
		{"batch with a blank pharmacy", func() error {
			_, err := c.BatchGetTrust(ctx, &trustv1.BatchGetTrustRequest{PharmacyIds: []string{"ph-1", ""}})
			return err
		}},
		{"batch too large", func() error {
			_, err := c.BatchGetTrust(ctx, &trustv1.BatchGetTrustRequest{PharmacyIds: tooMany})
			return err
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := status.Code(tt.call()); got != codes.InvalidArgument {
				t.Errorf("code = %v, want InvalidArgument", got)
			}
		})
	}
}

func TestUnexpectedErrorsAreInternal(t *testing.T) {
	err := toStatus(errors.New("database is down"))
	if status.Code(err) != codes.Internal || status.Convert(err).Message() != "internal error" {
		t.Errorf("toStatus = %v", err)
	}
}

type brokenRepo struct{}

func (brokenRepo) ApplyFeedback(context.Context, domain.Feedback, func(domain.Record) domain.Record) (bool, error) {
	return false, errors.New("down")
}
func (brokenRepo) Records(context.Context, []string) (map[string]domain.Record, error) {
	return nil, errors.New("down")
}

func TestRepositoryFailureIsInternal(t *testing.T) {
	svc, err := app.NewService(brokenRepo{}, domain.DefaultParams, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	h := NewHandler(svc)
	if _, err := h.GetTrust(context.Background(), &trustv1.GetTrustRequest{PharmacyId: "ph-1"}); status.Code(err) != codes.Internal {
		t.Errorf("GetTrust = %v, want Internal", err)
	}
	if _, err := h.BatchGetTrust(context.Background(), &trustv1.BatchGetTrustRequest{PharmacyIds: []string{"ph-1"}}); status.Code(err) != codes.Internal {
		t.Errorf("BatchGetTrust = %v, want Internal", err)
	}
}
