//go:build integration

package saga

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	tcrabbit "github.com/testcontainers/testcontainers-go/modules/rabbitmq"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/Swiftmeds-bht/Swiftmeds/pkg/grpcx"
	holdsv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/holds/v1"
	inventoryv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/inventory/v1"
)

// HoldDuration is short so the expiry scenario does not have to wait long.
const holdDuration = 3 * time.Second

// stack is everything the scenarios use. One stack is shared by all tests;
// each test uses its own pharmacy, product and user IDs, so they do not interfere.
var stack struct {
	holds     holdsv1.HoldsServiceClient
	inventory inventoryv1.InventoryServiceClient
	events    *eventLog
	amqpURL   string
	logs      []*serviceLog
}

func TestMain(m *testing.M) {
	code, err := run(m)
	if err != nil {
		fmt.Fprintln(os.Stderr, "saga integration setup:", err)
		code = 1
	}
	os.Exit(code)
}

func run(m *testing.M) (int, error) {
	ctx := context.Background()

	// Same images as deploy/docker-compose.yml.
	pg, err := tcpostgres.Run(ctx, "postgres:16-alpine",
		tcpostgres.WithDatabase("postgres"),
		tcpostgres.WithUsername("swiftmed"),
		tcpostgres.WithPassword("change-me"),
		tcpostgres.BasicWaitStrategies(),
	)
	defer terminate(pg)
	if err != nil {
		return 1, fmt.Errorf("start postgres: %w", err)
	}
	rmq, err := tcrabbit.Run(ctx, "rabbitmq:3.13-management",
		tcrabbit.WithAdminUsername("swiftmed"),
		tcrabbit.WithAdminPassword("change-me"),
	)
	defer terminate(rmq)
	if err != nil {
		return 1, fmt.Errorf("start rabbitmq: %w", err)
	}

	adminURL, err := pg.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		return 1, err
	}
	holdsDB, err := createDatabase(ctx, adminURL, "holds_db")
	if err != nil {
		return 1, err
	}
	inventoryDB, err := createDatabase(ctx, adminURL, "inventory_db")
	if err != nil {
		return 1, err
	}
	if stack.amqpURL, err = rmq.AmqpURL(ctx); err != nil {
		return 1, err
	}

	// Listen to every event before the services start, so none is missed.
	if stack.events, err = watchEvents(stack.amqpURL); err != nil {
		return 1, fmt.Errorf("watch events: %w", err)
	}
	defer stack.events.Close()

	binDir, err := os.MkdirTemp("", "swiftmed-saga-")
	if err != nil {
		return 1, err
	}
	defer func() { _ = os.RemoveAll(binDir) }()

	inventoryPort, holdsPort := freePort(), freePort()
	inventory, err := startService(ctx, binDir, "inventory", inventoryPort, map[string]string{
		"INVENTORY_GRPC_PORT":    strconv.Itoa(inventoryPort),
		"INVENTORY_DATABASE_URL": inventoryDB,
		"RABBITMQ_URL":           stack.amqpURL,
	})
	if err != nil {
		return 1, err
	}
	defer inventory.stop()
	holds, err := startService(ctx, binDir, "holds", holdsPort, map[string]string{
		"HOLDS_GRPC_PORT":     strconv.Itoa(holdsPort),
		"HOLDS_DATABASE_URL":  holdsDB,
		"RABBITMQ_URL":        stack.amqpURL,
		"HOLDS_UPSTREAM":      fmt.Sprintf("localhost:%d", inventoryPort), // straight to Inventory, no Traefik
		"HOLDS_HOLD_DURATION": holdDuration.String(),
	})
	if err != nil {
		return 1, err
	}
	defer holds.stop()
	stack.logs = []*serviceLog{inventory.log, holds.log}

	inventoryConn, err := dial(inventoryPort)
	if err != nil {
		return 1, err
	}
	defer func() { _ = inventoryConn.Close() }()
	holdsConn, err := dial(holdsPort)
	if err != nil {
		return 1, err
	}
	defer func() { _ = holdsConn.Close() }()
	stack.inventory = inventoryv1.NewInventoryServiceClient(inventoryConn)
	stack.holds = holdsv1.NewHoldsServiceClient(holdsConn)

	return m.Run(), nil
}

func terminate(c testcontainers.Container) {
	if err := testcontainers.TerminateContainer(c); err != nil {
		fmt.Fprintln(os.Stderr, "terminate container:", err)
	}
}

// createDatabase makes one database per service (as deploy/postgres/init does)
// and returns its connection URL.
func createDatabase(ctx context.Context, adminURL, name string) (string, error) {
	conn, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		return "", fmt.Errorf("connect to postgres: %w", err)
	}
	defer func() { _ = conn.Close(ctx) }()
	if _, err := conn.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
		return "", fmt.Errorf("create %s: %w", name, err)
	}
	u, err := url.Parse(adminURL)
	if err != nil {
		return "", err
	}
	u.Path = "/" + name
	return u.String(), nil
}

func freePort() int {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(err)
	}
	defer func() { _ = l.Close() }()
	return l.Addr().(*net.TCPAddr).Port
}

func dial(port int) (*grpc.ClientConn, error) {
	return grpc.NewClient(fmt.Sprintf("localhost:%d", port), grpc.WithTransportCredentials(insecure.NewCredentials()))
}

// repoRoot is two levels above this package (tests/integration/saga).
func repoRoot() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..", "..")
}

// serviceLog keeps a service's output, printed when a test fails.
type serviceLog struct {
	name string
	mu   sync.Mutex
	buf  bytes.Buffer
}

func (l *serviceLog) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.Write(p)
}

func (l *serviceLog) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.String()
}

type service struct {
	cmd *exec.Cmd
	log *serviceLog
}

// startService builds services/<name> from source and runs it until it reports healthy.
func startService(ctx context.Context, binDir, name string, port int, env map[string]string) (*service, error) {
	bin := filepath.Join(binDir, name)
	build := exec.CommandContext(ctx, "go", "build", "-race", "-o", bin, "./cmd/server")
	build.Dir = filepath.Join(repoRoot(), "services", name)
	build.Env = append(os.Environ(), "GOWORK=off") // each service builds on its own go.mod, like its Dockerfile
	if out, err := build.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("build %s: %w\n%s", name, err, out)
	}

	log := &serviceLog{name: name}
	cmd := exec.Command(bin)
	cmd.Env = append(os.Environ(), "LOG_LEVEL=debug")
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	cmd.Stdout, cmd.Stderr = log, log
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start %s: %w", name, err)
	}
	svc := &service{cmd: cmd, log: log}

	deadline := time.Now().Add(60 * time.Second)
	for {
		hctx, cancel := context.WithTimeout(ctx, time.Second)
		err := grpcx.CheckHealth(hctx, fmt.Sprintf("localhost:%d", port))
		cancel()
		if err == nil {
			return svc, nil
		}
		if time.Now().After(deadline) {
			svc.stop()
			return nil, fmt.Errorf("%s did not become healthy: %w\n%s", name, err, log)
		}
		time.Sleep(250 * time.Millisecond)
	}
}

func (s *service) stop() {
	if s == nil || s.cmd.Process == nil {
		return
	}
	_ = s.cmd.Process.Signal(os.Interrupt)
	done := make(chan struct{})
	go func() { _ = s.cmd.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		_ = s.cmd.Process.Kill()
	}
}

// dumpLogsOnFailure prints both services' output if the test failed, which is
// usually the fastest way to see why an event was not handled.
func dumpLogsOnFailure(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		if !t.Failed() {
			return
		}
		for _, l := range stack.logs {
			t.Logf("----- %s log -----\n%s", l.name, l)
		}
	})
}
