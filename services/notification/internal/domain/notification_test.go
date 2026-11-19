package domain

import (
	"errors"
	"testing"
	"time"
)

func TestNewNotificationValidation(t *testing.T) {
	tests := []struct {
		name     string
		id       string
		eventID  string
		userID   string
		channel  int32
		template int32
		wantErr  error
	}{
		{name: "missing id", id: " ", eventID: "event", userID: "user", channel: 1, template: 1, wantErr: ErrMissingEventID},
		{name: "missing event id", id: "id", eventID: "\t", userID: "user", channel: 1, template: 1, wantErr: ErrMissingEventID},
		{name: "missing user id", id: "id", eventID: "event", userID: " ", channel: 1, template: 1, wantErr: ErrMissingUserID},
		{name: "channel below range", id: "id", eventID: "event", userID: "user", channel: 0, template: 1, wantErr: ErrInvalidChannel},
		{name: "channel above range", id: "id", eventID: "event", userID: "user", channel: 4, template: 1, wantErr: ErrInvalidChannel},
		{name: "invalid template", id: "id", eventID: "event", userID: "user", channel: 1, template: 0, wantErr: ErrInvalidTemplate},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewNotification(tt.id, tt.eventID, tt.userID, tt.channel, tt.template, nil, time.Time{})
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("NewNotification() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestNewNotificationPreservesValues(t *testing.T) {
	now := time.Date(2026, time.November, 20, 12, 30, 0, 0, time.UTC)
	params := map[string]string{"order": "123"}

	got, err := NewNotification("id", "event", "user", 3, 7, params, now)
	if err != nil {
		t.Fatalf("NewNotification() error = %v", err)
	}
	if got.ID != "id" || got.EventID != "event" || got.UserID != "user" || got.Channel != 3 || got.Template != 7 {
		t.Fatalf("NewNotification() returned unexpected fields: %+v", got)
	}
	if got.CreatedAt != now || got.Params["order"] != "123" {
		t.Fatalf("NewNotification() did not preserve params/time: %+v", got)
	}
}
