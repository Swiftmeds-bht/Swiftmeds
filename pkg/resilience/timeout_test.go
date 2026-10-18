package resilience

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestWithTimeoutEndsASlowCall(t *testing.T) {
	err := WithTimeout(context.Background(), 20*time.Millisecond, func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	})

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want DeadlineExceeded", err)
	}
}

func TestWithTimeoutLeavesAFastCallAlone(t *testing.T) {
	err := WithTimeout(context.Background(), time.Second, func(ctx context.Context) error {
		if _, ok := ctx.Deadline(); !ok {
			t.Error("call has no deadline")
		}
		return nil
	})

	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
}

func TestWithTimeoutZeroAddsNoDeadline(t *testing.T) {
	_ = WithTimeout(context.Background(), 0, func(ctx context.Context) error {
		if _, ok := ctx.Deadline(); ok {
			t.Error("call has a deadline, want none")
		}
		return nil
	})
}

func TestWithTimeoutKeepsAnEarlierParentDeadline(t *testing.T) {
	parent, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	err := WithTimeout(parent, time.Hour, func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	})

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want DeadlineExceeded", err)
	}
}
