package main

import (
	"context"
	"errors"
	"testing"
)

func runtimeResult(t *testing.T, cancelled bool, first, second error) error {
	t.Helper()
	ctx, stop := context.WithCancel(context.Background())
	if cancelled {
		stop()
	}
	errs := make(chan error, 2)
	errs <- first
	errs <- second
	return waitForRuntime(ctx, stop, errs)
}

func TestWaitForRuntime(t *testing.T) {
	boom := errors.New("boom")
	tests := []struct {
		name          string
		cancelled     bool
		first, second error
		wantErr       bool
	}{
		{"clean shutdown", true, nil, context.Canceled, false},
		{"error during shutdown", true, nil, boom, true},
		{"one runtime fails", false, boom, nil, true},
		{"one runtime stops without error", false, nil, nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := runtimeResult(t, tt.cancelled, tt.first, tt.second)
			if (err != nil) != tt.wantErr {
				t.Errorf("err = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
