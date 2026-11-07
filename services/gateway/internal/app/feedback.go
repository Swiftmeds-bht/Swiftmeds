package app

import (
	"log/slog"
	"net/http"
	"strings"

	feedbackv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/feedback/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/gateway/internal/ports"
)

type feedbackBody struct {
	WasInStock bool   `json:"wasInStock"`
	Rating     int32  `json:"rating"`
	Comment    string `json:"comment"`
}

func feedbackHandler(client ports.FeedbackClient, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		identity := identityFrom(r)
		if !patientOnly(w, identity) {
			return
		}
		var body feedbackBody
		if !decodeBody(w, r, &body) || body.Rating < 0 || body.Rating > 5 || len([]rune(body.Comment)) > 500 {
			writeError(w, http.StatusBadRequest, "invalid feedback request")
			return
		}
		ctx, cancel := callContext(r)
		defer cancel()
		resp, err := client.SubmitFeedback(ctx, &feedbackv1.SubmitFeedbackRequest{
			HoldId: r.PathValue("holdId"), UserId: identity.UserID, WasInStock: body.WasInStock, Rating: body.Rating, Comment: strings.TrimSpace(body.Comment),
		})
		if err != nil {
			writeDownstreamError(w, logger, r, err)
			return
		}
		writeProtoStatus(w, http.StatusCreated, resp.GetFeedback())
	}
}
