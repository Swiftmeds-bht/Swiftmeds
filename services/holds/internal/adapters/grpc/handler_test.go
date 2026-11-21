package grpc

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	commonv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/common/v1"
	holdsv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/holds/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/holds/internal/adapters/memory"
	"github.com/Swiftmeds-bht/Swiftmeds/services/holds/internal/app"
	"github.com/Swiftmeds-bht/Swiftmeds/services/holds/internal/domain"
)

var t0 = time.Date(2026, 11, 3, 9, 0, 0, 0, time.UTC)

const testCode = "ABC234"

// fixture is a Handler on an in-memory repository and a clock the test moves.
type fixture struct {
	h   *Handler
	svc *app.Service
	now time.Time
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	f := &fixture{now: t0}
	n := 0
	ids := func() (string, error) {
		n++
		return fmt.Sprintf("h-%d", n), nil
	}
	newCode := func() (string, error) { return testCode, nil }
	cfg := app.Config{MaxActivePerUser: 3, HoldDuration: 2 * time.Hour}
	f.svc = app.NewService(memory.NewRepository(), cfg, func() time.Time { return f.now },
		app.WithIDGenerator(ids), app.WithCodeGenerator(newCode))
	f.h = NewHandler(f.svc)
	return f
}

// createHold makes a PENDING hold for u-1 at ph-1 through the RPC.
func (f *fixture) createHold(t *testing.T) *holdsv1.Hold {
	t.Helper()
	res, err := f.h.CreateHold(context.Background(), &holdsv1.CreateHoldRequest{
		UserId: "u-1", PharmacyId: "ph-1", ProductId: "med-1", Quantity: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	return res.GetHold()
}

// activeHold makes a hold ACTIVE, the way the reservation saga will.
func (f *fixture) activeHold(t *testing.T) *holdsv1.Hold {
	t.Helper()
	pending := f.createHold(t)
	if _, err := f.svc.ReserveHold(context.Background(), pending.GetHoldId()); err != nil {
		t.Fatal(err)
	}
	res, err := f.h.GetHold(context.Background(), &holdsv1.GetHoldRequest{HoldId: pending.GetHoldId()})
	if err != nil {
		t.Fatal(err)
	}
	return res.GetHold()
}

func wantCode(t *testing.T, err error, want codes.Code) {
	t.Helper()
	if got := status.Code(err); got != want {
		t.Fatalf("code = %s (err = %v), want %s", got, err, want)
	}
}

func TestCreateHoldReturnsAPendingHoldWithoutACode(t *testing.T) {
	f := newFixture(t)

	hold := f.createHold(t)

	if hold.GetHoldId() != "h-1" || hold.GetUserId() != "u-1" || hold.GetPharmacyId() != "ph-1" ||
		hold.GetProductId() != "med-1" || hold.GetQuantity() != 2 {
		t.Errorf("fields were not kept: %v", hold)
	}
	if hold.GetStatus() != holdsv1.HoldStatus_HOLD_STATUS_PENDING {
		t.Errorf("status = %s, want PENDING", hold.GetStatus())
	}
	if hold.GetPickupCode() != "" || hold.GetExpiresAt() != nil {
		t.Errorf("pickup code = %q, expires_at = %v, want neither on a PENDING hold", hold.GetPickupCode(), hold.GetExpiresAt())
	}
	if !hold.GetCreatedAt().AsTime().Equal(t0) {
		t.Errorf("created_at = %v, want %v", hold.GetCreatedAt().AsTime(), t0)
	}
}

func TestCreateHoldMapsErrors(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	_, err := f.h.CreateHold(ctx, &holdsv1.CreateHoldRequest{UserId: "u-1", PharmacyId: "ph-1", ProductId: "med-1", Quantity: 9})
	wantCode(t, err, codes.InvalidArgument)

	for range 3 {
		f.createHold(t)
	}
	_, err = f.h.CreateHold(ctx, &holdsv1.CreateHoldRequest{UserId: "u-1", PharmacyId: "ph-1", ProductId: "med-1", Quantity: 1})
	wantCode(t, err, codes.ResourceExhausted)
}

func TestGetHoldShowsThePickupCodeOnlyWhileActive(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	pending := f.createHold(t)

	got, err := f.h.GetHold(ctx, &holdsv1.GetHoldRequest{HoldId: pending.GetHoldId()})
	if err != nil {
		t.Fatal(err)
	}
	if got.GetHold().GetPickupCode() != "" {
		t.Errorf("a PENDING hold shows the code %q", got.GetHold().GetPickupCode())
	}

	active := f.activeHold(t)
	if active.GetPickupCode() != testCode || active.GetExpiresAt() == nil {
		t.Errorf("ACTIVE hold: pickup code = %q, expires_at = %v, want %q and an expiry time", active.GetPickupCode(), active.GetExpiresAt(), testCode)
	}
	if want := t0.Add(2 * time.Hour); !active.GetExpiresAt().AsTime().Equal(want) {
		t.Errorf("expires_at = %v, want %v", active.GetExpiresAt().AsTime(), want)
	}
}

func TestGetHoldMapsErrors(t *testing.T) {
	f := newFixture(t)

	_, err := f.h.GetHold(context.Background(), &holdsv1.GetHoldRequest{HoldId: "nope"})
	wantCode(t, err, codes.NotFound)
	_, err = f.h.GetHold(context.Background(), &holdsv1.GetHoldRequest{})
	wantCode(t, err, codes.InvalidArgument)
}

func TestListHoldsOfAPatientShowsCodesButAPharmacyListDoesNot(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	f.activeHold(t)

	byUser, err := f.h.ListHolds(ctx, &holdsv1.ListHoldsRequest{UserId: "u-1"})
	if err != nil {
		t.Fatal(err)
	}
	byPharmacy, err := f.h.ListHolds(ctx, &holdsv1.ListHoldsRequest{PharmacyId: "ph-1"})
	if err != nil {
		t.Fatal(err)
	}

	if len(byUser.GetHolds()) != 1 || byUser.GetHolds()[0].GetPickupCode() != testCode {
		t.Errorf("patient's list = %v, want one hold with the code %q", byUser.GetHolds(), testCode)
	}
	if len(byPharmacy.GetHolds()) != 1 || byPharmacy.GetHolds()[0].GetPickupCode() != "" {
		t.Errorf("pharmacy's list = %v, want one hold and no code", byPharmacy.GetHolds())
	}
}

func TestListHoldsFiltersByStatusAndPages(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	for range 3 {
		f.createHold(t)
		f.now = f.now.Add(time.Minute)
	}

	pending, err := f.h.ListHolds(ctx, &holdsv1.ListHoldsRequest{
		UserId: "u-1",
		Status: holdsv1.HoldStatus_HOLD_STATUS_PENDING,
		Page:   &commonv1.PageRequest{PageSize: 2},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(pending.GetHolds()) != 2 || pending.GetHolds()[0].GetHoldId() != "h-3" || pending.GetPage().GetNextPageToken() == "" {
		t.Fatalf("first page = %v (next %q), want h-3 and h-2 and a next page", pending.GetHolds(), pending.GetPage().GetNextPageToken())
	}

	rest, err := f.h.ListHolds(ctx, &holdsv1.ListHoldsRequest{
		UserId: "u-1",
		Status: holdsv1.HoldStatus_HOLD_STATUS_PENDING,
		Page:   &commonv1.PageRequest{PageSize: 2, PageToken: pending.GetPage().GetNextPageToken()},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(rest.GetHolds()) != 1 || rest.GetHolds()[0].GetHoldId() != "h-1" || rest.GetPage().GetNextPageToken() != "" {
		t.Errorf("second page = %v (next %q), want only h-1 and no next page", rest.GetHolds(), rest.GetPage().GetNextPageToken())
	}

	none, err := f.h.ListHolds(ctx, &holdsv1.ListHoldsRequest{UserId: "u-1", Status: holdsv1.HoldStatus_HOLD_STATUS_COLLECTED})
	if err != nil || len(none.GetHolds()) != 0 {
		t.Errorf("COLLECTED list = %v (err %v), want none", none.GetHolds(), err)
	}
}

func TestListHoldsMapsErrors(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	_, err := f.h.ListHolds(ctx, &holdsv1.ListHoldsRequest{})
	wantCode(t, err, codes.InvalidArgument)
	_, err = f.h.ListHolds(ctx, &holdsv1.ListHoldsRequest{UserId: "u-1", PharmacyId: "ph-1"})
	wantCode(t, err, codes.InvalidArgument)
	_, err = f.h.ListHolds(ctx, &holdsv1.ListHoldsRequest{UserId: "u-1", Status: holdsv1.HoldStatus(99)})
	wantCode(t, err, codes.InvalidArgument)
	_, err = f.h.ListHolds(ctx, &holdsv1.ListHoldsRequest{UserId: "u-1", Page: &commonv1.PageRequest{PageToken: "!!"}})
	wantCode(t, err, codes.InvalidArgument)
}

func TestCancelHold(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	active := f.activeHold(t)

	_, err := f.h.CancelHold(ctx, &holdsv1.CancelHoldRequest{HoldId: active.GetHoldId(), UserId: "u-2"})
	wantCode(t, err, codes.PermissionDenied)

	res, err := f.h.CancelHold(ctx, &holdsv1.CancelHoldRequest{HoldId: active.GetHoldId(), UserId: "u-1", Reason: "changed my mind"})
	if err != nil {
		t.Fatal(err)
	}
	if res.GetHold().GetStatus() != holdsv1.HoldStatus_HOLD_STATUS_CANCELLED || res.GetHold().GetStatusReason() != "changed my mind" {
		t.Errorf("hold = %v, want CANCELLED with the reason", res.GetHold())
	}
	if res.GetHold().GetPickupCode() != "" {
		t.Error("a cancelled hold still shows its pickup code")
	}

	_, err = f.h.CancelHold(ctx, &holdsv1.CancelHoldRequest{HoldId: active.GetHoldId(), UserId: "u-1"})
	wantCode(t, err, codes.FailedPrecondition)
}

// The ticket: "Wrong pickup code is rejected; collecting twice fails."
func TestCollectHoldRejectsAWrongCodeAndCollectingTwice(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	active := f.activeHold(t)
	collect := func(pharmacy, code string) (*holdsv1.CollectHoldResponse, error) {
		return f.h.CollectHold(ctx, &holdsv1.CollectHoldRequest{HoldId: active.GetHoldId(), PharmacyId: pharmacy, PickupCode: code})
	}

	_, err := collect("ph-1", "ZZZ999")
	wantCode(t, err, codes.PermissionDenied)
	_, err = collect("ph-1", "")
	wantCode(t, err, codes.InvalidArgument)
	_, err = collect("ph-2", testCode)
	wantCode(t, err, codes.PermissionDenied)

	res, err := collect("ph-1", testCode)
	if err != nil {
		t.Fatalf("the right code was refused: %v", err)
	}
	if res.GetHold().GetStatus() != holdsv1.HoldStatus_HOLD_STATUS_COLLECTED {
		t.Errorf("status = %s, want COLLECTED", res.GetHold().GetStatus())
	}
	if res.GetHold().GetPickupCode() != "" {
		t.Error("the collect reply still carries the pickup code")
	}

	_, err = collect("ph-1", testCode)
	wantCode(t, err, codes.FailedPrecondition)
}

func TestCollectHoldAfterTheHoldRanOutOfTime(t *testing.T) {
	f := newFixture(t)
	active := f.activeHold(t)
	f.now = f.now.Add(2 * time.Hour)

	_, err := f.h.CollectHold(context.Background(), &holdsv1.CollectHoldRequest{HoldId: active.GetHoldId(), PharmacyId: "ph-1", PickupCode: testCode})

	wantCode(t, err, codes.FailedPrecondition)
}

func TestExpireHold(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	active := f.activeHold(t)

	_, err := f.h.ExpireHold(ctx, &holdsv1.ExpireHoldRequest{HoldId: active.GetHoldId()})
	wantCode(t, err, codes.FailedPrecondition)

	f.now = f.now.Add(2 * time.Hour)
	res, err := f.h.ExpireHold(ctx, &holdsv1.ExpireHoldRequest{HoldId: active.GetHoldId()})
	if err != nil {
		t.Fatal(err)
	}
	if res.GetHold().GetStatus() != holdsv1.HoldStatus_HOLD_STATUS_EXPIRED || res.GetHold().GetExpiresAt() == nil {
		t.Errorf("hold = %v, want EXPIRED and still carrying its expiry time", res.GetHold())
	}

	_, err = f.h.ExpireHold(ctx, &holdsv1.ExpireHoldRequest{HoldId: "nope"})
	wantCode(t, err, codes.NotFound)
}

func TestToStatus(t *testing.T) {
	cases := []struct {
		err  error
		want codes.Code
	}{
		{domain.ErrMissingHoldID, codes.InvalidArgument},
		{domain.ErrMissingUserID, codes.InvalidArgument},
		{domain.ErrMissingPharmacyID, codes.InvalidArgument},
		{domain.ErrMissingProductID, codes.InvalidArgument},
		{domain.ErrMissingPickupCode, codes.InvalidArgument},
		{domain.ErrInvalidQuantity, codes.InvalidArgument},
		{domain.ErrInvalidListFilter, codes.InvalidArgument},
		{domain.ErrInvalidPageToken, codes.InvalidArgument},
		{domain.ErrNotFound, codes.NotFound},
		{domain.ErrAlreadyExists, codes.AlreadyExists},
		{domain.ErrLimitReached, codes.ResourceExhausted},
		{domain.ErrPickupLocked, codes.ResourceExhausted},
		{domain.ErrNotStocked, codes.NotFound},
		{domain.ErrWrongPickupCode, codes.PermissionDenied},
		{domain.ErrWrongPharmacy, codes.PermissionDenied},
		{domain.ErrNotHoldOwner, codes.PermissionDenied},
		{domain.ErrInvalidTransition, codes.FailedPrecondition},
		{domain.ErrHoldExpired, codes.FailedPrecondition},
		{domain.ErrNotYetExpired, codes.FailedPrecondition},
		{domain.ErrConflict, codes.Aborted},
		{domain.ErrPickupCodeTaken, codes.Aborted},
		{fmt.Errorf("wrapped: %w", domain.ErrNotFound), codes.NotFound},
	}
	for _, c := range cases {
		if got := status.Code(toStatus(c.err)); got != c.want {
			t.Errorf("toStatus(%v) = %s, want %s", c.err, got, c.want)
		}
	}
}

func TestToStatusHidesUnexpectedErrors(t *testing.T) {
	err := toStatus(errors.New("pq: password authentication failed for user swiftmed"))

	if status.Code(err) != codes.Internal || status.Convert(err).Message() != "internal error" {
		t.Fatalf("got %v, want a plain internal error that leaks nothing", err)
	}
}

func TestStatusConversionCoversEveryStatus(t *testing.T) {
	for _, s := range []domain.Status{domain.Pending, domain.Active, domain.Collected, domain.Expired, domain.Cancelled, domain.Rejected} {
		back, err := statusFromProto(statusToProto(s))
		if err != nil || back != s {
			t.Errorf("%s -> %s -> (%q, %v), want the same status back", s, statusToProto(s), back, err)
		}
	}
	if got := statusToProto("SOMETHING"); got != holdsv1.HoldStatus_HOLD_STATUS_UNSPECIFIED {
		t.Errorf("unknown status = %s, want UNSPECIFIED", got)
	}
	if got, err := statusFromProto(holdsv1.HoldStatus_HOLD_STATUS_UNSPECIFIED); err != nil || got != "" {
		t.Errorf("UNSPECIFIED = (%q, %v), want an empty status and no error", got, err)
	}
}
