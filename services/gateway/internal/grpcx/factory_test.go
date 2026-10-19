package grpcx

import (
	"context"
	"testing"
)

func TestConnectionRequiresServiceName(t *testing.T) {
	f := NewClientFactory("localhost:1")
	t.Cleanup(func() { _ = f.Close() })

	if _, err := f.Connection(context.Background(), ""); err == nil {
		t.Fatal("Connection(\"\") returned nil error, want error")
	}
}

func TestConnectionIsLazyAndCachedPerService(t *testing.T) {
	// Nothing listens on this port: NewClient must not dial until the first RPC.
	f := NewClientFactory("localhost:1")
	t.Cleanup(func() { _ = f.Close() })

	auth1, err := f.Connection(context.Background(), "auth")
	if err != nil {
		t.Fatalf("Connection(auth) error = %v, want nil (connection is lazy)", err)
	}
	auth2, err := f.Connection(context.Background(), "auth")
	if err != nil {
		t.Fatal(err)
	}
	if auth1 != auth2 {
		t.Error("second Connection(auth) returned a new conn, want the cached one")
	}

	catalog, err := f.Connection(context.Background(), "catalog")
	if err != nil {
		t.Fatal(err)
	}
	if catalog == auth1 {
		t.Error("Connection(catalog) returned the auth conn, want one per service")
	}
}

func TestCloseClearsCache(t *testing.T) {
	f := NewClientFactory("localhost:1")

	first, err := f.Connection(context.Background(), "auth")
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("Close() = %v", err)
	}
	second, err := f.Connection(context.Background(), "auth")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	if first == second {
		t.Error("Connection after Close returned the closed conn, want a new one")
	}
}
