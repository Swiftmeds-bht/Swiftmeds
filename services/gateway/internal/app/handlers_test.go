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
	"google.golang.org/protobuf/types/known/timestamppb"
)

type fakeHoldsClient struct {
	get      *holdsv1.GetHoldRequest
	cancel   *holdsv1.CancelHoldRequest
	collect  *holdsv1.CollectHoldRequest
	response *holdsv1.Hold
}

func (f *fakeHoldsClient) CreateHold(context.Context, *holdsv1.CreateHoldRequest) (*holdsv1.CreateHoldResponse, error) {
	return &holdsv1.CreateHoldResponse{Hold: f.response}, nil
}
func (f *fakeHoldsClient) GetHold(_ context.Context, req *holdsv1.GetHoldRequest) (*holdsv1.GetHoldResponse, error) {
	f.get = req
	return &holdsv1.GetHoldResponse{Hold: f.response}, nil
}
func (f *fakeHoldsClient) ListHolds(context.Context, *holdsv1.ListHoldsRequest) (*holdsv1.ListHoldsResponse, error) {
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

type fakeWatchlistClient struct {
	subscribe *watchlistv1.SubscribeRequest
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
