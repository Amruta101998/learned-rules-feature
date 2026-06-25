package learning

import (
	"context"
	"log"
	"time"
)

type ClusterUpdateEvent struct {
	ClusterID         string
	WorkspaceID       int64
	AgentID           string
	PreviousType      string // positive/negative
	CurrentType       string // positive/negative
	Sentiment         float64
	ConfidenceScore   float64
	DisabledByDefault bool // Resolved flag from agent-exec-mgmt
}

type EmbeddingService struct {
	rulesRepository     AdaptiveRulesRepository
	configProvider      ConfigProvider
}

type AdaptiveRulesRepository interface {
	InsertLearnedRuleAndMetadata(ctx context.Context, wsID int64, ruleData *AdaptiveRuleData, isEnabled bool) (int64, error)
	UpdateRuleType(ctx context.Context, ruleID int64, newType string) error
	UpdateRuleEnabledState(ctx context.Context, ruleID int64, isEnabled bool, isDisabledByUser bool) error
	GetRuleByClusterID(ctx context.Context, clusterID string) (*AdaptiveRuleMetadata, error)
}

type AdaptiveRuleData struct {
	ClusterID       string
	Pattern         string
	WorkspaceID     int64
	AgentID         string
	RuleType        string // positive or negative
	ConfidenceScore float64
	SampleFeedback  []string
}

type AdaptiveRuleMetadata struct {
	RuleID          int64
	ClusterID       string
	RuleType        string // positive or negative
	IsEnabled       bool
	IsDisabledByUser bool
	WorkspaceID     int64
	Version         int
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

func NewEmbeddingService(rulesRepo AdaptiveRulesRepository, configProvider ConfigProvider) *EmbeddingService {
	return &EmbeddingService{
		rulesRepository: rulesRepo,
		configProvider:  configProvider,
	}
}

// InsertLearnedRuleAndMetadata creates a new learned rule from a cluster
func (es *EmbeddingService) InsertLearnedRuleAndMetadata(ctx context.Context, clusterEvent *ClusterUpdateEvent, ruleData *AdaptiveRuleData) (int64, error) {
	// Determine if new negative rules should be disabled
	disableNewNegatives := clusterEvent.DisabledByDefault

	log.Printf("EmbeddingService: disableNewNegatives flag for workspace %d = %v", clusterEvent.WorkspaceID, disableNewNegatives)

	// Determine initial enabled state
	isEnabled := true // default: all rules start enabled
	if ruleData.RuleType == "negative" && disableNewNegatives {
		isEnabled = false
		log.Printf("New negative rule created with isEnabled=false for cluster %s, workspace %d", clusterEvent.ClusterID, clusterEvent.WorkspaceID)
	}

	ruleID, err := es.rulesRepository.InsertLearnedRuleAndMetadata(ctx, clusterEvent.WorkspaceID, ruleData, isEnabled)
	if err != nil {
		return 0, err
	}

	log.Printf("Adaptive rule %d created: type=%s, enabled=%v, cluster=%s", ruleID, ruleData.RuleType, isEnabled, clusterEvent.ClusterID)

	return ruleID, nil
}

// SyncLearnedRuleWithCluster handles rule type flips and updates enabled state accordingly
func (es *EmbeddingService) SyncLearnedRuleWithCluster(ctx context.Context, clusterEvent *ClusterUpdateEvent) error {
	rule, err := es.rulesRepository.GetRuleByClusterID(ctx, clusterEvent.ClusterID)
	if err != nil {
		return err
	}

	if rule == nil {
		log.Printf("No rule found for cluster %s", clusterEvent.ClusterID)
		return nil
	}

	// Check for type flip
	if rule.RuleType != clusterEvent.CurrentType {
		log.Printf("Rule type flip detected for cluster %s: %s → %s", clusterEvent.ClusterID, rule.RuleType, clusterEvent.CurrentType)

		if err := es.rulesRepository.UpdateRuleType(ctx, rule.RuleID, clusterEvent.CurrentType); err != nil {
			log.Printf("error updating rule type: %v", err)
			return err
		}

		// Handle flip logic
		if clusterEvent.PreviousType == "positive" && clusterEvent.CurrentType == "negative" {
			// positive → negative: apply the disable-by-default flag
			// But preserve is_disabled_by_user=1 (don't override manual OFF)
			if !rule.IsDisabledByUser {
				newIsEnabled := !clusterEvent.DisabledByDefault
				log.Printf("Rule %d flipped positive→negative: setting isEnabled=%v (disabledByDefault=%v)",
					rule.RuleID, newIsEnabled, clusterEvent.DisabledByDefault)
				if err := es.rulesRepository.UpdateRuleEnabledState(ctx, rule.RuleID, newIsEnabled, false); err != nil {
					log.Printf("error updating rule enabled state: %v", err)
					return err
				}
			} else {
				log.Printf("Rule %d flipped positive→negative but isDisabledByUser=1, preserving state", rule.RuleID)
			}
		} else if clusterEvent.PreviousType == "negative" && clusterEvent.CurrentType == "positive" {
			// negative → positive: always enable (positives apply by default)
			// Clear is_disabled_by_user since system is now re-enabling
			log.Printf("Rule %d flipped negative→positive: enabling rule", rule.RuleID)
			if err := es.rulesRepository.UpdateRuleEnabledState(ctx, rule.RuleID, true, false); err != nil {
				log.Printf("error enabling rule after flip: %v", err)
				return err
			}
		}
	}

	return nil
}
