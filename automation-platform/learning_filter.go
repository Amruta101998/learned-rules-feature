package learning

import (
	"context"
	"log"
)

type Review struct {
	ReviewID    string
	PRNumber    int64
	WorkspaceID int64
	ReviewText  string
	Feedbacks   []*Feedback
}

type Feedback struct {
	FeedbackID string
	RuleID     int64
	Applied    bool
	Suggestion string
}

type LearnedRule struct {
	RuleID    int64
	Version   int // 0 for QMM, 1+ for adaptive
	Pattern   string
	IsEnabled bool
	RuleType  string // negative or positive
}

type RulesProvider interface {
	GetEnabledRules(ctx context.Context, wsID int64, version int) ([]*LearnedRule, error)
}

type LearningFilter struct {
	rulesProvider RulesProvider
}

func NewLearningFilter(rulesProvider RulesProvider) *LearningFilter {
	return &LearningFilter{
		rulesProvider: rulesProvider,
	}
}

// ApplyLearningToFeedbacks applies both v0 and v1 learned rules to review feedbacks
// Both queries (GetRulesV0/GetRulesV1Plus) filter is_enabled=1, so disabled rules never apply
func (lf *LearningFilter) ApplyLearningToFeedbacks(ctx context.Context, review *Review) error {
	// Get v0 rules (QMM - all negative)
	v0Rules, err := lf.rulesProvider.GetEnabledRules(ctx, review.WorkspaceID, 0)
	if err != nil {
		log.Printf("error getting v0 rules for ws=%d: %v", review.WorkspaceID, err)
		return err
	}

	// Get v1+ rules (adaptive - positive and negative)
	v1Rules, err := lf.rulesProvider.GetEnabledRules(ctx, review.WorkspaceID, 1)
	if err != nil {
		log.Printf("error getting v1 rules for ws=%d: %v", review.WorkspaceID, err)
		return err
	}

	allRules := append(v0Rules, v1Rules...)

	log.Printf("ApplyLearningToFeedbacks: applying %d v0 + %d v1 learned rules to review %s (ws=%d)",
		len(v0Rules), len(v1Rules), review.ReviewID, review.WorkspaceID)

	// Apply rules that are enabled (is_enabled=1)
	for _, rule := range allRules {
		if !rule.IsEnabled {
			// Disabled rules (by default or by user) are not applied
			log.Printf("Skipping disabled rule %d in review %s", rule.RuleID, review.ReviewID)
			continue
		}

		// Match pattern and apply feedback
		if lf.matchPattern(rule.Pattern, review.ReviewText) {
			feedback := &Feedback{
				FeedbackID: generateFeedbackID(),
				RuleID:     rule.RuleID,
				Applied:    true,
				Suggestion: generateSuggestion(rule),
			}
			review.Feedbacks = append(review.Feedbacks, feedback)
			log.Printf("Applied rule %d (version %d) to review %s", rule.RuleID, rule.Version, review.ReviewID)
		}
	}

	return nil
}

// matchPattern is a placeholder for the actual pattern matching logic
func (lf *LearningFilter) matchPattern(pattern string, reviewText string) bool {
	// This would be the actual pattern matching implementation
	// For now, a simple substring match
	if len(pattern) > 0 && len(reviewText) > 0 {
		return pattern != "" // Placeholder logic
	}
	return false
}

func generateFeedbackID() string {
	// Generate unique feedback ID
	return "feedback_" + getCurrentTimestamp()
}

func generateSuggestion(rule *LearnedRule) string {
	return "Suggestion based on learned rule " + rule.Pattern
}

func getCurrentTimestamp() string {
	// Placeholder
	return "12345"
}
