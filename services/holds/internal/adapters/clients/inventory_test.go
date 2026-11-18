package clients

import (
	"context"
	"errors"
	"net"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	inventoryv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/inventory/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/holds/internal/domain"
)

// fakeServer is a stand-in Inventory service that records the hold IDs it receives.
type fakeServer struct {
	inventoryv1.UnimplementedInventoryServiceServer
	mu        sync.Mutex
	released  []string
	committed []string
	checked   []string
	err       error
}

func (f *fakeServer) GetStock(_ context.Context, r *inventoryv1.GetStockRequest) (*inventoryv1.GetStockResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.checked = append(f.checked, r.GetPharmacyId()+"/"+r.GetProductId())
	return &inventoryv1.GetStockResponse{}, f.err
}

func (f *fakeServer) ReleaseReservation(_ context.Context, r *inventoryv1.ReleaseReservationRequest) (*inventoryv1.ReleaseReservationResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.released = append(f.released, r.GetHoldId())
	return &inventoryv1.ReleaseReservationResponse{}, f.err
}

func (f *fakeServer) CommitReservation(_ context.Context, r *inventoryv1.CommitReservationRequest) (*inventoryv1.CommitReservationResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.committed = append(f.committed, r.GetHoldId())
	return &inventoryv1.CommitReservationResponse{}, f.err
}

// connect starts the fake server in memory and returns a client of it.
func connect(t *testing.T, fake *fakeServer) Inventory {
	t.Helper()
	lis := bufconn.Listen(1 << 20)
	srv := grpc.NewServer()
	inventoryv1.RegisterInventoryServiceServer(srv, fake)
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)

	conn, err := grpc.NewClient("passthrough:///inventory",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return lis.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return NewInventory(conn)
}

func TestReleaseSendsTheHoldID(t *testing.T) {
	fake := &fakeServer{}
	inv := connect(t, fake)

	if err := inv.Release(context.Background(), "h-1"); err != nil {
		t.Fatal(err)
	}

	if len(fake.released) != 1 || fake.released[0] != "h-1" || len(fake.committed) != 0 {
		t.Errorf("released %v, committed %v, want only release of h-1", fake.released, fake.committed)
	}
}

func TestCommitSendsTheHoldID(t *testing.T) {
	fake := &fakeServer{}
	inv := connect(t, fake)

	if err := inv.Commit(context.Background(), "h-2"); err != nil {
		t.Fatal(err)
	}

	if len(fake.committed) != 1 || fake.committed[0] != "h-2" || len(fake.released) != 0 {
		t.Errorf("released %v, committed %v, want only commit of h-2", fake.released, fake.committed)
	}
}

// Whatever goes wrong, the app layer must see ErrInventoryUnavailable: it leaves
// the hold unchanged and the caller may try again.
func TestEveryInventoryFailureIsErrInventoryUnavailable(t *testing.T) {
	for _, code := range []codes.Code{codes.Unavailable, codes.Internal, codes.NotFound, codes.FailedPrecondition, codes.DeadlineExceeded} {
		t.Run(code.String(), func(t *testing.T) {
			fake := &fakeServer{err: status.Error(code, "inventory operation failed")}
			inv := connect(t, fake)

			if err := inv.Release(context.Background(), "h-1"); !errors.Is(err, domain.ErrInventoryUnavailable) {
				t.Errorf("Release: err = %v, want ErrInventoryUnavailable", err)
			}
			if err := inv.Commit(context.Background(), "h-1"); !errors.Is(err, domain.ErrInventoryUnavailable) {
				t.Errorf("Commit: err = %v, want ErrInventoryUnavailable", err)
			}
		})
	}
}

func TestCheckStocked(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want error
	}{
		{"stocked", nil, nil},
		{"unknown pharmacy or product", status.Error(codes.NotFound, "inventory stock not found"), domain.ErrNotStocked},
		{"inventory down", status.Error(codes.Unavailable, "down"), domain.ErrInventoryUnavailable},
		{"inventory error", status.Error(codes.Internal, "boom"), domain.ErrInventoryUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fake := &fakeServer{err: tc.err}
			inv := connect(t, fake)

			err := inv.CheckStocked(context.Background(), "ph-1", "med-1")
			if !errors.Is(err, tc.want) || (tc.want == nil) != (err == nil) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
			if len(fake.checked) != 1 || fake.checked[0] != "ph-1/med-1" {
				t.Errorf("GetStock calls = %v, want ph-1/med-1", fake.checked)
			}
		})
	}
}

func TestACancelledCallIsErrInventoryUnavailable(t *testing.T) {
	inv := connect(t, &fakeServer{})
	ctx, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
	defer cancel()
	time.Sleep(time.Millisecond)

	if err := inv.Release(ctx, "h-1"); !errors.Is(err, domain.ErrInventoryUnavailable) {
		t.Errorf("err = %v, want ErrInventoryUnavailable", err)
	}
}
