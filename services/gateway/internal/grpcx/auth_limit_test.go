package grpcx

import (
	"context"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"

	"github.com/Swiftmeds-bht/Swiftmeds/services/gateway/internal/app"
)

// A sign-up that takes 1.2 s (bcrypt on a cold path) fails with the default
// 800 ms limit and succeeds with the auth limit the gateway uses (SCRUM-136).
func TestAuthLimitCoversASlowBcryptCall(t *testing.T) {
	if app.AuthCallTimeout <= DefaultCallTimeout {
		t.Fatalf("AuthCallTimeout %v must be longer than DefaultCallTimeout %v", app.AuthCallTimeout, DefaultCallTimeout)
	}
	f := NewClientFactory(slowHealthServer(t, 1200*time.Millisecond))
	t.Cleanup(func() { _ = f.Close() })
	auth, err := f.ConnectionWithTimeout(context.Background(), "auth", app.AuthCallTimeout)
	if err != nil {
		t.Fatal(err)
	}
	other, err := f.Connection(context.Background(), "catalog")
	if err != nil {
		t.Fatal(err)
	}

	_, authErr := healthpb.NewHealthClient(auth).Check(context.Background(), &healthpb.HealthCheckRequest{})
	_, otherErr := healthpb.NewHealthClient(other).Check(context.Background(), &healthpb.HealthCheckRequest{})

	if authErr != nil {
		t.Errorf("1.2 s call with the auth limit: err = %v, want success", authErr)
	}
	if status.Code(otherErr) != codes.DeadlineExceeded {
		t.Errorf("1.2 s call with the default limit: err = %v, want DeadlineExceeded", otherErr)
	}
}
