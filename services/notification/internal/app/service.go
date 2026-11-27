package app

import (
	"context"
	"crypto/rand"
	"fmt"
	"time"

	"github.com/Swiftmeds-bht/Swiftmeds/services/notification/internal/domain"
	"github.com/Swiftmeds-bht/Swiftmeds/services/notification/internal/ports"
)

type Service struct {
	repo     ports.NotificationRepository
	now      ports.Clock
}

func NewService(repo ports.NotificationRepository, now ports.Clock) *Service {
	return &Service{repo: repo, now: now}
}

func (s *Service) Send(ctx context.Context, eventID, userID string, channel, template int32, params map[string]string) (domain.Notification, error) {
	n, err := domain.NewNotification(NewID(), eventID, userID, channel, template, params, s.now())
	if err != nil {
		return domain.Notification{}, err
	}
	created, err := s.repo.Create(ctx, n)
	if err != nil {
		return domain.Notification{}, fmt.Errorf("save notification: %w", err)
	}
	if !created {
		return n, nil
	}
	return n, nil
}

func (s *Service) List(ctx context.Context, userID string, pageSize int, pageToken string) ([]domain.Notification, string, error) {
	if userID == "" {
		return nil, "", domain.ErrMissingUserID
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return s.repo.ListByUser(ctx, userID, pageSize, pageToken)
}

func NewID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

var _ ports.Clock = (func() time.Time)(time.Now)
