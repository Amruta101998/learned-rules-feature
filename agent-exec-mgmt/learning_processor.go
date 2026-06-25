package learning

import (
	"context"
	"log"

	"github.com/bito/agent-exec-mgmt/config"
)

type LearningEvent struct {
	WorkspaceID                   int64
	AgentID                       string
	LearningType                  string // "qmm" or "adaptive"
	RuleData                      interface{}
	LearnedRulesDisabledByDefault bool
}

type LearningProcessor struct {
	configResolver config.ConfigResolver
}

func NewLearningProcessor(configResolver config.ConfigResolver) *LearningProcessor {
	return &LearningProcessor{
		configResolver: configResolver,
	}
}

// ProcessLearningEvent resolves the flag and creates a learning event with the resolved value
func (lp *LearningProcessor) ProcessLearningEvent(ctx context.Context, wsID int64, agentID string, ruleData interface{}) (*LearningEvent, error) {
	// Resolve the flag: agent > workspace > default (false)
	agentVal, err := lp.configResolver.GetAgentConfig(ctx, wsID, agentID, "learned_rules_disabled_by_default")
	if err != nil {
		log.Printf("error retrieving agent config: %v", err)
	}

	wsVal, err := lp.configResolver.GetWorkspaceConfig(ctx, wsID, "agent-config.public.learned_rules_disabled_by_default")
	if err != nil {
		log.Printf("error retrieving workspace config: %v", err)
	}

	resolvedFlag := config.ResolveLearnedRulesDisabledByDefault(agentVal, wsVal)

	log.Printf("LearningProcessor: resolved learned_rules_disabled_by_default=%v for ws=%d, agent=%s (agent=%v, ws=%v)",
		resolvedFlag, wsID, agentID, agentVal, wsVal)

	return &LearningEvent{
		WorkspaceID:                   wsID,
		AgentID:                       agentID,
		LearnedRulesDisabledByDefault: resolvedFlag,
		RuleData:                      ruleData,
	}, nil
}

// For QMM learning pipeline
type QMMLearningEvent struct {
	*LearningEvent
	ReactionType string // "dislike", "avoid", etc.
	PRNumber     int64
	ReviewID     string
}

// For Adaptive learning pipeline
type AdaptiveLearningEvent struct {
	*LearningEvent
	ClusterID         string
	ClusterSentiment  string // "positive" or "negative"
	ConfidenceScore   float64
	IncrementalUpdate bool
}
