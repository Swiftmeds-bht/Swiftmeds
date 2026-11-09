package clients

import (
	"context"

	feedbackv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/feedback/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/gateway/internal/ports"
)

var _ ports.FeedbackClient = (*FeedbackClient)(nil)

type FeedbackClient struct {
	client feedbackv1.FeedbackServiceClient
}

func NewFeedbackClient(client feedbackv1.FeedbackServiceClient) *FeedbackClient {
	return &FeedbackClient{client: client}
}
func (c *FeedbackClient) SubmitFeedback(ctx context.Context, req *feedbackv1.SubmitFeedbackRequest) (*feedbackv1.SubmitFeedbackResponse, error) {
	return c.client.SubmitFeedback(ctx, req)
}
