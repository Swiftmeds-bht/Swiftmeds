package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	authv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/auth/v1"
	feedbackv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/feedback/v1"
	holdsv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/holds/v1"
	watchlistv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/watchlist/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/gateway/internal/httpx"
	"github.com/Swiftmeds-bht/Swiftmeds/services/gateway/internal/ports"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type fakeHoldsClient struct {
	get      *holdsv1.GetHoldRequest
	create   *holdsv1.CreateHoldRequest
	list     *holdsv1.ListHoldsRequest
	cancel   *holdsv1.CancelHoldRequest
	collect  *holdsv1.CollectHoldRequest
	response *holdsv1.Hold
}

func (f *fakeHoldsClient) CreateHold(_ context.Context, req *holdsv1.CreateHoldRequest) (*holdsv1.CreateHoldResponse, error) {
	f.create = req
	return &holdsv1.CreateHoldResponse{Hold: f.response}, nil
}
func (f *fakeHoldsClient) GetHold(_ context.Context, req *holdsv1.GetHoldRequest) (*holdsv1.GetHoldResponse, error) {
	f.get = req
	return &holdsv1.GetHoldResponse{Hold: f.response}, nil
}
func (f *fakeHoldsClient) ListHolds(_ context.Context, req *holdsv1.ListHoldsRequest) (*holdsv1.ListHoldsResponse, error) {
	f.list = req
	return &holdsv1.ListHoldsResponse{}, nil
}
func (f *fakeHoldsClient) CancelHold(_ context.Context, req *holdsv1.CancelHoldRequest) (*holdsv1.CancelHoldResponse, error) {
	f.cancel = req
	return &holdsv1.CancelHoldResponse{Hold: f.response}, nil
}
func (f *fakeHoldsClient) CollectHold(_ context.Context, req *holdsv1.CollectHoldRequest) (*holdsv1.CollectHoldResponse, error) {
	f.collect = req
	return &holdsv1.CollectHoldResponse{Hold: f.response}, nil
}

func withIdentity(r *http.Request, identity ports.Identity) *http.Request {
	return r.WithContext(httpx.WithIdentity(r.Context(), identity))
}

func TestHoldHandlersEnforceOwnership(t *testing.T) {
	fake := &fakeHoldsClient{response: &holdsv1.Hold{HoldId: "h-1", UserId: "user-2", PharmacyId: "pharmacy-1", CreatedAt: timestamppb.Now()}}
	handler := holdsHandler(fake, nil)
	req := withIdentity(httptest.NewRequest(http.MethodGet, "/api/v1/holds/h-1", nil), ports.Identity{UserID: "user-1", Role: authv1.Role_ROLE_PATIENT})
	req.SetPathValue("holdId", "h-1")
	rec := httptest.NewRecorder()
	handler(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("got %d, want 403", rec.Code)
	}
}

func TestHoldHandlersMapAuthenticatedIdentity(t *testing.T) {
	fake := &fakeHoldsClient{response: &holdsv1.Hold{HoldId: "h-1", UserId: "user-1", PharmacyId: "pharmacy-1"}}
	handler := holdsHandler(fake, nil)

	cancel := withIdentity(httptest.NewRequest(http.MethodPost, "/api/v1/holds/h-1/cancel", strings.NewReader(`{"reason":"changed mind"}`)), ports.Identity{UserID: "user-1", Role: authv1.Role_ROLE_PATIENT})
	cancel.SetPathValue("holdId", "h-1")
	handler(httptest.NewRecorder(), cancel)
	if fake.cancel == nil || fake.cancel.GetUserId() != "user-1" {
		t.Fatalf("cancel request did not use token identity: %v", fake.cancel)
	}

	collect := withIdentity(httptest.NewRequest(http.MethodPost, "/api/v1/holds/h-1/collect", strings.NewReader(`{"pickupCode":"482913"}`)), ports.Identity{UserID: "staff-1", PharmacyID: "pharmacy-1", Role: authv1.Role_ROLE_PHARMACY_STAFF})
	collect.SetPathValue("holdId", "h-1")
	handler(httptest.NewRecorder(), collect)
	if fake.collect == nil || fake.collect.GetPharmacyId() != "pharmacy-1" || fake.collect.GetPickupCode() != "482913" {
		t.Fatalf("collect request did not use token pharmacy: %v", fake.collect)
	}
}

func TestHoldHandlersCreateAndList(t *testing.T) {
	fake := &fakeHoldsClient{response: &holdsv1.Hold{HoldId: "h-1"}}
	handler := holdsHandler(fake, nil)

	create := withIdentity(httptest.NewRequest(http.MethodPost, "/api/v1/holds", strings.NewReader(`{"pharmacyId":"ph-1","productId":"p-1","quantity":2}`)), ports.Identity{UserID: "user-1", Role: authv1.Role_ROLE_PATIENT})
	createRec := httptest.NewRecorder()
	handler(createRec, create)
	if createRec.Code != http.StatusAccepted || fake.create == nil || fake.create.GetUserId() != "user-1" {
		t.Fatalf("create got %d and request %v", createRec.Code, fake.create)
	}

	list := withIdentity(httptest.NewRequest(http.MethodGet, "/api/v1/holds?pageSize=10&pageToken=next&status=HOLD_STATUS_ACTIVE", nil), ports.Identity{UserID: "user-1", Role: authv1.Role_ROLE_PATIENT})
	listRec := httptest.NewRecorder()
	handler(listRec, list)
	if listRec.Code != http.StatusOK || fake.list == nil || fake.list.GetUserId() != "user-1" || fake.list.GetPage().GetPageSize() != 10 || fake.list.GetPage().GetPageToken() != "next" || fake.list.GetStatus() != holdsv1.HoldStatus_HOLD_STATUS_ACTIVE {
		t.Fatalf("list got %d and request %v", listRec.Code, fake.list)
	}
}

func TestHoldHandlersRejectInvalidRequests(t *testing.T) {
	fake := &fakeHoldsClient{response: &holdsv1.Hold{HoldId: "h-1"}}
	handler := holdsHandler(fake, nil)
	tests := []struct {
		name string
		req  *http.Request
	}{
		{"create without identity", httptest.NewRequest(http.MethodPost, "/api/v1/holds", strings.NewReader(`{}`))},
		{"create invalid JSON", withIdentity(httptest.NewRequest(http.MethodPost, "/api/v1/holds", strings.NewReader(`{`)), ports.Identity{Role: authv1.Role_ROLE_PATIENT})},
		{"list invalid page size", withIdentity(httptest.NewRequest(http.MethodGet, "/api/v1/holds?pageSize=bad", nil), ports.Identity{UserID: "u-1", Role: authv1.Role_ROLE_PATIENT})},
		{"list invalid status", withIdentity(httptest.NewRequest(http.MethodGet, "/api/v1/holds?status=unknown", nil), ports.Identity{UserID: "u-1", Role: authv1.Role_ROLE_PATIENT})},
		{"collect missing pickup code", withIdentity(httptest.NewRequest(http.MethodPost, "/api/v1/holds/h-1/collect", strings.NewReader(`{}`)), ports.Identity{PharmacyID: "ph-1", Role: authv1.Role_ROLE_PHARMACY_STAFF})},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if strings.Contains(tt.name, "collect") {
				tt.req.SetPathValue("holdId", "h-1")
			}
			rec := httptest.NewRecorder()
			handler(rec, tt.req)
			if rec.Code != http.StatusBadRequest && rec.Code != http.StatusForbidden {
				t.Fatalf("got %d, want 400 or 403", rec.Code)
			}
		})
	}
}

func TestHoldHandlersListForPharmacyAndRedactsPickupCode(t *testing.T) {
	fake := &fakeHoldsClient{response: &holdsv1.Hold{HoldId: "h-1", PharmacyId: "ph-1", PickupCode: "482913"}}
	handler := holdsHandler(fake, nil)

	list := withIdentity(httptest.NewRequest(http.MethodGet, "/api/v1/holds", nil), ports.Identity{PharmacyID: "ph-1", Role: authv1.Role_ROLE_PHARMACY_STAFF})
	rec := httptest.NewRecorder()
	handler(rec, list)
	if rec.Code != http.StatusOK || fake.list == nil || fake.list.GetPharmacyId() != "ph-1" {
		t.Fatalf("list got %d and request %v", rec.Code, fake.list)
	}

	get := withIdentity(httptest.NewRequest(http.MethodGet, "/api/v1/holds/h-1", nil), ports.Identity{PharmacyID: "ph-1", Role: authv1.Role_ROLE_ADMIN})
	get.SetPathValue("holdId", "h-1")
	rec = httptest.NewRecorder()
	handler(rec, get)
	if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), "482913") {
		t.Fatalf("get got %d with body %q", rec.Code, rec.Body.String())
	}
}

func TestHoldHandlersRejectForbiddenRoles(t *testing.T) {
	handler := holdsHandler(&fakeHoldsClient{}, nil)
	tests := []struct {
		name string
		req  *http.Request
	}{
		{"unknown list role", httptest.NewRequest(http.MethodGet, "/api/v1/holds", nil)},
		{"staff without pharmacy", withIdentity(httptest.NewRequest(http.MethodGet, "/api/v1/holds", nil), ports.Identity{Role: authv1.Role_ROLE_PHARMACY_STAFF})},
		{"patient collect", withIdentity(httptest.NewRequest(http.MethodPost, "/api/v1/holds/h-1/collect", strings.NewReader(`{"pickupCode":"482913"}`)), ports.Identity{Role: authv1.Role_ROLE_PATIENT})},
		{"staff cancel", withIdentity(httptest.NewRequest(http.MethodPost, "/api/v1/holds/h-1/cancel", nil), ports.Identity{Role: authv1.Role_ROLE_PHARMACY_STAFF})},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.req.SetPathValue("holdId", "h-1")
			rec := httptest.NewRecorder()
			handler(rec, tt.req)
			if rec.Code != http.StatusForbidden {
				t.Fatalf("got %d, want 403", rec.Code)
			}
		})
	}
}

type fakeWatchlistClient struct {
	subscribe *watchlistv1.SubscribeRequest
	stream    *fakeAlertStream
	openErr   error
	streamReq *watchlistv1.StreamAlertsRequest
}

func (f *fakeWatchlistClient) StreamAlerts(ctx context.Context, req *watchlistv1.StreamAlertsRequest) (grpc.ServerStreamingClient[watchlistv1.StockAlert], error) {
	f.streamReq = req
	if f.openErr != nil {
		return nil, f.openErr
	}
	f.stream.ctx = ctx
	return f.stream, nil
}

func (f *fakeWatchlistClient) Subscribe(_ context.Context, req *watchlistv1.SubscribeRequest) (*watchlistv1.SubscribeResponse, error) {
	f.subscribe = req
	return &watchlistv1.SubscribeResponse{Subscription: &watchlistv1.Subscription{SubscriptionId: "s-1"}}, nil
}
func (*fakeWatchlistClient) Unsubscribe(context.Context, *watchlistv1.UnsubscribeRequest) (*watchlistv1.UnsubscribeResponse, error) {
	return &watchlistv1.UnsubscribeResponse{}, nil
}
func (*fakeWatchlistClient) ListSubscriptions(context.Context, *watchlistv1.ListSubscriptionsRequest) (*watchlistv1.ListSubscriptionsResponse, error) {
	return &watchlistv1.ListSubscriptionsResponse{}, nil
}

func TestWatchlistHandlerUsesTokenUser(t *testing.T) {
	fake := &fakeWatchlistClient{}
	req := withIdentity(httptest.NewRequest(http.MethodPost, "/api/v1/watchlist", strings.NewReader(`{"productId":"p-1","location":{"latitude":27,"longitude":89},"radiusM":1000}`)), ports.Identity{UserID: "user-1", Role: authv1.Role_ROLE_PATIENT})
	rec := httptest.NewRecorder()
	watchlistHandler(fake, nil)(rec, req)
	if rec.Code != http.StatusCreated || fake.subscribe == nil || fake.subscribe.GetUserId() != "user-1" {
		t.Fatalf("got %d and request %v", rec.Code, fake.subscribe)
	}
}

func TestWatchlistHandlerSupportsListAndDelete(t *testing.T) {
	handler := watchlistHandler(&fakeWatchlistClient{}, nil)

	list := withIdentity(httptest.NewRequest(http.MethodGet, "/api/v1/watchlist", nil), ports.Identity{UserID: "user-1", Role: authv1.Role_ROLE_PATIENT})
	rec := httptest.NewRecorder()
	handler(rec, list)
	if rec.Code != http.StatusOK {
		t.Fatalf("list got %d, want 200", rec.Code)
	}

	deleteRequest := withIdentity(httptest.NewRequest(http.MethodDelete, "/api/v1/watchlist/s-1", nil), ports.Identity{UserID: "user-1", Role: authv1.Role_ROLE_PATIENT})
	deleteRequest.SetPathValue("subscriptionId", "s-1")
	rec = httptest.NewRecorder()
	handler(rec, deleteRequest)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete got %d, want 204", rec.Code)
	}
}

func TestWatchlistHandlerRejectsInvalidRequests(t *testing.T) {
	handler := watchlistHandler(&fakeWatchlistClient{}, nil)
	tests := []struct {
		name string
		req  *http.Request
	}{
		{"wrong role", httptest.NewRequest(http.MethodPost, "/api/v1/watchlist", strings.NewReader(`{}`))},
		{"invalid JSON", withIdentity(httptest.NewRequest(http.MethodPost, "/api/v1/watchlist", strings.NewReader(`{`)), ports.Identity{Role: authv1.Role_ROLE_PATIENT})},
		{"missing location", withIdentity(httptest.NewRequest(http.MethodPost, "/api/v1/watchlist", strings.NewReader(`{"productId":"p-1"}`)), ports.Identity{Role: authv1.Role_ROLE_PATIENT})},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			handler(rec, tt.req)
			if rec.Code != http.StatusBadRequest && rec.Code != http.StatusForbidden {
				t.Fatalf("got %d, want 400 or 403", rec.Code)
			}
		})
	}
}

type fakeFeedbackClient struct {
	request *feedbackv1.SubmitFeedbackRequest
}

func (f *fakeFeedbackClient) SubmitFeedback(_ context.Context, req *feedbackv1.SubmitFeedbackRequest) (*feedbackv1.SubmitFeedbackResponse, error) {
	f.request = req
	return &feedbackv1.SubmitFeedbackResponse{Feedback: &feedbackv1.Feedback{FeedbackId: "f-1", HoldId: req.GetHoldId()}}, nil
}

func TestFeedbackHandlerUsesTokenUser(t *testing.T) {
	fake := &fakeFeedbackClient{}
	req := withIdentity(httptest.NewRequest(http.MethodPost, "/api/v1/holds/h-1/feedback", strings.NewReader(`{"wasInStock":true,"rating":5}`)), ports.Identity{UserID: "user-1", Role: authv1.Role_ROLE_PATIENT})
	req.SetPathValue("holdId", "h-1")
	rec := httptest.NewRecorder()
	feedbackHandler(fake, nil)(rec, req)
	if rec.Code != http.StatusCreated || fake.request == nil || fake.request.GetUserId() != "user-1" || fake.request.GetHoldId() != "h-1" {
		t.Fatalf("got %d and request %v", rec.Code, fake.request)
	}
}

func TestFeedbackHandlerRejectsInvalidRequests(t *testing.T) {
	handler := feedbackHandler(&fakeFeedbackClient{}, nil)
	tests := []string{
		`{`,
		`{"rating":6}`,
		`{"rating":1,"comment":"` + strings.Repeat("x", 501) + `"}`,
	}
	for _, body := range tests {
		req := withIdentity(httptest.NewRequest(http.MethodPost, "/api/v1/holds/h-1/feedback", strings.NewReader(body)), ports.Identity{Role: authv1.Role_ROLE_PATIENT})
		req.SetPathValue("holdId", "h-1")
		rec := httptest.NewRecorder()
		handler(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("body %q got %d, want 400", body, rec.Code)
		}
	}
}
