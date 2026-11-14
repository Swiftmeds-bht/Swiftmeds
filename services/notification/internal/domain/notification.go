package domain

import (
	"errors"
	"strings"
	"time"
)

var (
	ErrMissingUserID   = errors.New("user_id is required")
	ErrMissingEventID  = errors.New("event_id is required")
	ErrInvalidChannel  = errors.New("channel is invalid")
	ErrInvalidTemplate = errors.New("template is invalid")
)

type Notification struct {
	ID        string
	EventID   string
	UserID    string
	Channel   int32
	Template  int32
	Params    map[string]string
	CreatedAt time.Time
	ReadAt    time.Time
}

func NewNotification(id, eventID, userID string, channel, template int32, params map[string]string, now time.Time) (Notification, error) {
	if strings.TrimSpace(id) == "" || strings.TrimSpace(eventID) == "" {
		return Notification{}, ErrMissingEventID
	}
	if strings.TrimSpace(userID) == "" {
		return Notification{}, ErrMissingUserID
	}
	if channel < 1 || channel > 3 {
		return Notification{}, ErrInvalidChannel
	}
	if template < 1 {
		return Notification{}, ErrInvalidTemplate
	}
	return Notification{ID: id, EventID: eventID, UserID: userID, Channel: channel, Template: template, Params: params, CreatedAt: now}, nil
}
