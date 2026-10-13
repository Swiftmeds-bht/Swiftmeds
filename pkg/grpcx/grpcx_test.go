package grpcx

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

func testLogger(buf *bytes.Buffer) *slog.Logger {
	return slog.New(slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

func unaryInfo(method string) *grpc.UnaryServerInfo {
	return &grpc.UnaryServerInfo{FullMethod: method}
}

// fakeStream is the minimum grpc.ServerStream the interceptors need.
type fakeStream struct{ grpc.ServerStream }

func (fakeStream) Context() context.Context { return context.Background() }

func TestRecoveryUnaryTurnsPanicIntoInternal(t *testing.T) {
	var buf bytes.Buffer
	panicking := func(context.Context, any) (any, error) { panic("boom") }

	_, err := RecoveryUnary(testLogger(&buf))(context.Background(), nil, unaryInfo("/svc/Method"), panicking)

	if status.Code(err) != codes.Internal {
		t.Fatalf("code = %v, want Internal", status.Code(err))
	}
	if strings.Contains(status.Convert(err).Message(), "boom") {
		t.Error("panic value leaked to the caller")
	}
	if !strings.Contains(buf.String(), "boom") || !strings.Contains(buf.String(), "stack") {
		t.Errorf("panic not logged with stack: %s", buf.String())
	}
}

func TestRecoveryUnaryPassesThroughNormalCalls(t *testing.T) {
	var buf bytes.Buffer
	ok := func(context.Context, any) (any, error) { return "resp", nil }

	resp, err := RecoveryUnary(testLogger(&buf))(context.Background(), nil, unaryInfo("/svc/Method"), ok)

	if err != nil || resp != "resp" {
		t.Fatalf("got (%v, %v), want (resp, nil)", resp, err)
	}
}

func TestRecoveryStreamTurnsPanicIntoInternal(t *testing.T) {
	var buf bytes.Buffer
	panicking := func(any, grpc.ServerStream) error { panic("boom") }
	info := &grpc.StreamServerInfo{FullMethod: "/svc/Stream"}

	err := RecoveryStream(testLogger(&buf))(nil, fakeStream{}, info, panicking)

	if status.Code(err) != codes.Internal {
		t.Fatalf("code = %v, want Internal", status.Code(err))
	}
}

func TestLoggingUnaryLogsMethodCodeAndLevel(t *testing.T) {
	cases := []struct {
		name      string
		err       error
		wantCode  string
		wantLevel string
	}{
		{"ok", nil, "OK", "INFO"},
		{"client error", status.Error(codes.NotFound, "no price"), "NotFound", "WARN"},
		{"server error", status.Error(codes.Internal, "db down"), "Internal", "ERROR"},
		{"plain error", errors.New("oops"), "Unknown", "ERROR"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			handler := func(context.Context, any) (any, error) { return nil, tc.err }

			_, err := LoggingUnary(testLogger(&buf))(context.Background(), nil, unaryInfo("/svc/Method"), handler)
			if !errors.Is(err, tc.err) {
				t.Fatalf("error changed: got %v, want %v", err, tc.err)
			}

			var entry map[string]any
			if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
				t.Fatalf("log is not JSON: %v (%q)", err, buf.String())
			}
			if entry["method"] != "/svc/Method" || entry["code"] != tc.wantCode || entry["level"] != tc.wantLevel {
				t.Errorf("entry = %v, want method=/svc/Method code=%s level=%s", entry, tc.wantCode, tc.wantLevel)
			}
			if _, ok := entry["duration_ms"]; !ok {
				t.Error("duration_ms missing")
			}
		})
	}
}

func TestLoggingSkipsHealthChecks(t *testing.T) {
	var buf bytes.Buffer
	handler := func(context.Context, any) (any, error) { return nil, nil }

	_, _ = LoggingUnary(testLogger(&buf))(context.Background(), nil, unaryInfo(healthPrefix+"Check"), handler)

	if buf.Len() != 0 {
		t.Errorf("health check was logged: %s", buf.String())
	}
}

func TestLoggingStreamLogsWhenStreamEnds(t *testing.T) {
	var buf bytes.Buffer
	handler := func(any, grpc.ServerStream) error { return nil }
	info := &grpc.StreamServerInfo{FullMethod: "/svc/Stream"}

	if err := LoggingStream(testLogger(&buf))(nil, fakeStream{}, info, handler); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "/svc/Stream") {
		t.Errorf("stream call not logged: %s", buf.String())
	}
}

// TestServeReportsHealthAndShutsDown runs a real server over an in-memory connection.
func TestServeReportsHealthAndShutsDown(t *testing.T) {
	var buf bytes.Buffer
	srv := NewServer(testLogger(&buf))
	lis := bufconn.Listen(1 << 20)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- srv.ServeListener(ctx, lis) }()

	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	// Serve sets SERVING just after starting; allow a moment for that.
	client := healthpb.NewHealthClient(conn)
	deadline := time.Now().Add(2 * time.Second)
	for {
		resp, err := client.Check(context.Background(), &healthpb.HealthCheckRequest{})
		if err == nil && resp.GetStatus() == healthpb.HealthCheckResponse_SERVING {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("server never reported SERVING (last: %v, %v)", resp, err)
		}
		time.Sleep(10 * time.Millisecond)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Serve returned %v, want nil after shutdown", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Serve did not return after context was cancelled")
	}
}

func TestServeFailsOnBadAddress(t *testing.T) {
	var buf bytes.Buffer
	err := NewServer(testLogger(&buf)).Serve(context.Background(), "not-an-address")
	if err == nil {
		t.Fatal("Serve on a bad address returned nil")
	}
}
