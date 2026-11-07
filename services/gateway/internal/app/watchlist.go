package app

import (
	"log/slog"
	"net/http"
	"strings"

	commonv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/common/v1"
	watchlistv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/watchlist/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/gateway/internal/ports"
)

type watchlistBody struct {
	ProductID string             `json:"productId"`
	Location  *commonv1.GeoPoint `json:"location"`
	RadiusM   int32              `json:"radiusM"`
}

func watchlistHandler(client ports.WatchlistClient, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		identity := identityFrom(r)
		if !patientOnly(w, identity) {
			return
		}
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/watchlist":
			var body watchlistBody
			if !decodeBody(w, r, &body) || strings.TrimSpace(body.ProductID) == "" || body.Location == nil {
				writeError(w, http.StatusBadRequest, "invalid watchlist request")
				return
			}
			resp, err := client.Subscribe(callContext(r), &watchlistv1.SubscribeRequest{
				UserId: identity.UserID, ProductId: body.ProductID, Location: body.Location, RadiusM: body.RadiusM,
			})
			if err != nil {
				writeDownstreamError(w, logger, r, err)
				return
			}
			writeProtoStatus(w, http.StatusCreated, resp.GetSubscription())
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/watchlist":
			resp, err := client.ListSubscriptions(callContext(r), &watchlistv1.ListSubscriptionsRequest{UserId: identity.UserID})
			if err != nil {
				writeDownstreamError(w, logger, r, err)
				return
			}
			writeProtoStatus(w, http.StatusOK, resp)
		case r.Method == http.MethodDelete && r.PathValue("subscriptionId") != "":
			_, err := client.Unsubscribe(callContext(r), &watchlistv1.UnsubscribeRequest{SubscriptionId: r.PathValue("subscriptionId"), UserId: identity.UserID})
			if err != nil {
				writeDownstreamError(w, logger, r, err)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		}
	}
}
