package reviewer

import (
	"context"
	"time"

	"go.uber.org/zap"
	"sys-ai/client"
	"sys-ai/llm"
)

// ContributionReviewer polls for pending AI reviews and processes them.
type ContributionReviewer struct {
	restaurantClient *client.RestaurantClient
	llmClient        *llm.Client
	batchSize        int
	logger           *zap.Logger
}

func New(
	restaurantClient *client.RestaurantClient,
	llmClient *llm.Client,
	batchSize int,
	logger *zap.Logger,
) *ContributionReviewer {
	return &ContributionReviewer{
		restaurantClient: restaurantClient,
		llmClient:        llmClient,
		batchSize:        batchSize,
		logger:           logger,
	}
}

// Run starts the polling loop. It blocks until ctx is cancelled.
func (r *ContributionReviewer) Run(ctx context.Context, pollInterval time.Duration) {
	r.logger.Info("contribution reviewer started", zap.Duration("poll_interval", pollInterval))

	// Run once immediately, then on ticker.
	r.processBatch(ctx)

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			r.logger.Info("contribution reviewer stopping")
			return
		case <-ticker.C:
			r.processBatch(ctx)
		}
	}
}

func (r *ContributionReviewer) processBatch(ctx context.Context) {
	contributions, err := r.restaurantClient.ListPendingAIReview(ctx, r.batchSize)
	if err != nil {
		r.logger.Error("failed to fetch pending contributions", zap.Error(err))
		return
	}

	if len(contributions) == 0 {
		r.logger.Debug("no contributions pending AI review")
		return
	}

	r.logger.Info("processing contributions", zap.Int("count", len(contributions)))

	for _, c := range contributions {
		select {
		case <-ctx.Done():
			return
		default:
		}

		log := r.logger.With(
			zap.String("contribution_id", c.ContributionID.String()),
			zap.String("entity_type", c.EntityType),
			zap.String("change_type", c.ChangeType),
		)

		result, err := r.llmClient.ReviewContribution(ctx, c.EntityType, c.ChangeType, c.ProposedChanges)
		if err != nil {
			log.Error("LLM review failed", zap.Error(err))
			// Post a safe fallback: flag for human review so the contribution isn't stuck.
			result = &llm.ReviewResult{
				Decision:       "flag_for_human",
				Confidence:     0,
				Reasoning:      "AI review failed; routed for human review.",
				FlaggedReasons: []string{"ai_error"},
			}
		}

		log.Info("review completed",
			zap.String("decision", result.Decision),
			zap.Float64("confidence", result.Confidence),
			zap.String("reasoning", result.Reasoning),
		)

		if err := r.restaurantClient.RecordAIReview(ctx, c.ContributionID, result); err != nil {
			log.Error("failed to record AI review result", zap.Error(err))
		}
	}
}
