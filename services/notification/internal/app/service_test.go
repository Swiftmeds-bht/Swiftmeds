package app

import (
	"context"
	"testing"
	"time"

	"github.com/Swiftmeds-bht/Swiftmeds/services/notification/internal/domain"
)

type fakeRepo struct {
	created map[string]bool
}

func (f *fakeRepo) Create(_ context.Context, n domain.Notification) (bool, error) {
	if f.created[n.EventID] {
		return false, nil
	}
	f.created[n.EventID] = true
	return true, nil
}

func (f *fakeRepo) ListByUser(context.Context, string, int, string) ([]domain.Notification, string, error) {
	return nil, "", nil
}

func TestSendDuplicateEventCreatesOneNotification(t *testing.T) {
	repo := &fakeRepo{created: map[string]bool{}}
	svc := NewService(repo, func() time.Time { return time.Unix(0, 0).UTC() })

	_, err := svc.Send(context.Background(), "evt-1", "user-1", 2, 1, map[string]string{"x": "1"})
	if err != nil {
		t.Fatalf("first send: %v", err)
	}
	_, err = svc.Send(context.Background(), "evt-1", "user-1", 2, 1, map[string]string{"x": "1"})
	if err != nil {
		t.Fatalf("second send: %v", err)
	}
}
