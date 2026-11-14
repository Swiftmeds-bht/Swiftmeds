package app

import (
	"log/slog"
	"net/http"
	"strconv"

	commonv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/common/v1"
	notificationv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/notification/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/gateway/internal/ports"
)

func notificationsHandler(client ports.NotificationClient, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		identity := identityFrom(r)
		if !patientOnly(w, identity) {
			return
		}
		req := &notificationv1.ListNotificationsRequest{
			UserId: identity.UserID,
			Page:   &commonv1.PageRequest{PageToken: r.URL.Query().Get("pageToken")},
		}
		if value := r.URL.Query().Get("pageSize"); value != "" {
			size, err := strconv.Atoi(value)
			if err != nil || size < 1 || size > 100 {
				writeError(w, http.StatusBadRequest, "invalid page size")
				return
			}
			req.Page.PageSize = int32(size)
		}
		ctx, cancel := callContext(r)
		defer cancel()
		resp, err := client.ListNotifications(ctx, req)
		if err != nil {
			writeDownstreamError(w, logger, r, err)
			return
		}
		writeProtoStatus(w, http.StatusOK, resp)
	}
}
