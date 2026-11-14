package app

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	authv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/auth/v1"
	commonv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/common/v1"
	notificationv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/notification/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/gateway/internal/ports"
)

type notificationTestAuth struct{}

func (notificationTestAuth) ValidateToken(context.Context, string) (ports.Identity, error) {
	return ports.Identity{UserID: "patient-1", Role: authv1.Role_ROLE_PATIENT}, nil
}

type notificationTestClient struct {
	request *notificationv1.ListNotificationsRequest
}

func (c *notificationTestClient) ListNotifications(_ context.Context, req *notificationv1.ListNotificationsRequest) (*notificationv1.ListNotificationsResponse, error) {
	c.request = req
	return &notificationv1.ListNotificationsResponse{
		Notifications: []*notificationv1.Notification{{NotificationId: "n-1", UserId: req.GetUserId()}},
		Page:          &commonv1.PageResponse{NextPageToken: "next"},
	}, nil
}

func TestNotificationsRouteUsesAuthenticatedPatientAndPagination(t *testing.T) {
	client := &notificationTestClient{}
	handler := NewRouter(
		Config{CORSOrigins: defaultCORSOrigins},
		slog.Default(),
		WithAuth(notificationTestAuth{}),
		WithNotifications(client),
	)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/notifications?pageSize=10&pageToken=cursor-1", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/notifications status = %d, want 200: %s", recorder.Code, recorder.Body.String())
	}
	if client.request == nil || client.request.GetUserId() != "patient-1" {
		t.Fatalf("downstream user_id = %v, want authenticated patient-1", client.request)
	}
	if client.request.GetPage().GetPageSize() != 10 || client.request.GetPage().GetPageToken() != "cursor-1" {
		t.Errorf("downstream page = %+v, want page size 10 and token cursor-1", client.request.GetPage())
	}
}
