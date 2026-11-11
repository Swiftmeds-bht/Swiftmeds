package ports

import (
	"context"

	feedbackv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/feedback/v1"
)

type FeedbackClient interface {
	SubmitFeedback(context.Context, *feedbackv1.SubmitFeedbackRequest) (*feedbackv1.SubmitFeedbackResponse, error)
}
