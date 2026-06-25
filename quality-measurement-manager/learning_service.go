package learning

import (
	"context"
	"database/sql"
	"log"
	"time"
)

type RuleMetadata struct {
	RuleID             int64
	RuleType           string // "negative" for v0
	IsEnabled          bool
	IsDisabledByUser   bool
	WorkspaceID        int64
	Version            int
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

type RulesRepository interface {
	CreateRule(ctx context.Context, wsID int64, ruleData *RuleData, isEnabled bool) (int64, error)
	UpdateRuleStatus(ctx context.Context, ruleID int64, isEnabled bool, isDisabledByUser bool) error
	GetRule(ctx context.Context, ruleID int64) (*RuleMetadata, error)
	GetRulesByWorkspace(ctx context.Context, wsID int64) ([]*RuleMetadata, error)
}

type RuleData struct {
	Pattern      string
	ReviewText   string
	PRNumber     int64
	WorkspaceID  int64
	AgentID      string
	Confidence   float64
	ReactionType string
}

type QMMLearningService struct {
	repo              RulesRepository
	configProvider    ConfigProvider
	rulesService      RulesService
	autoEnableGated   bool // True when learned_rules_disabled_by_default is ON
}

type ConfigProvider interface {
	GetLearnedRulesDisabledByDefault(ctx context.Context, wsID int64) (bool, error)
}

type RulesService interface {
	EnableRuleOnRuleIntensity(ctx context.Context, ruleID int64, intensity int) error
	UpdateRuleIntensity(ctx context.Context, ruleID int64, increment int) error
}

func NewQMMLearningService(repo RulesRepository, configProvider ConfigProvider, rulesService RulesService) *QMMLearningService {
	return &QMMLearningService{
		repo:           repo,
		configProvider: configProvider,
		rulesService:   rulesService,
	}
}

// SaveRule creates a new learned rule, respecting the learned_rules_disabled_by_default flag
func (qs *QMMLearningService) SaveRule(ctx context.Context, ruleData *RuleData) (int64, error) {
	// v0 only produces negative rules
	disabledByDefault, err := qs.configProvider.GetLearnedRulesDisabledByDefault(ctx, ruleData.WorkspaceID)
	if err != nil {
		log.Printf("error getting learned_rules_disabled_by_default config for ws=%d: %v", ruleData.WorkspaceID, err)
		disabledByDefault = false
	}

	// When flag is ON, new negative rules are created disabled
	isEnabled := !disabledByDefault

	log.Printf("QMM SaveRule: learned_rules_disabled_by_default=%v, creating rule with isEnabled=%v for ws=%d",
		disabledByDefault, isEnabled, ruleData.WorkspaceID)

	if !disabledByDefault {
		log.Printf("Rule created as enabled (flag OFF)")
	} else {
		log.Printf("Negative rule created as disabled (flag ON) for workspace %d - requires manual review before activation", ruleData.WorkspaceID)
	}

	ruleID, err := qs.repo.CreateRule(ctx, ruleData.WorkspaceID, ruleData, isEnabled)
	if err != nil {
		return 0, err
	}

	// Append rule to PR comment
	if err := qs.appendRuleToPRComment(ctx, ruleData, ruleID, isEnabled); err != nil {
		log.Printf("error appending rule to PR comment: %v", err)
	}

	return ruleID, nil
}

// EnableRuleOnRuleIntensity is gated by the learned_rules_disabled_by_default flag
func (qs *QMMLearningService) IncrementRuleIntensityAndCheckAutoEnable(ctx context.Context, ruleID int64, increment int) error {
	rule, err := qs.repo.GetRule(ctx, ruleID)
	if err != nil {
		return err
	}

	// Always increment intensity
	if err := qs.rulesService.UpdateRuleIntensity(ctx, ruleID, increment); err != nil {
		return err
	}

	// Check if flag is ON for this workspace
	disabledByDefault, err := qs.configProvider.GetLearnedRulesDisabledByDefault(ctx, rule.WorkspaceID)
	if err != nil {
		log.Printf("error getting learned_rules_disabled_by_default: %v", err)
		disabledByDefault = false
	}

	// If flag is ON, auto-enable is gated (skip EnableRuleOnRuleIntensity)
	if disabledByDefault {
		log.Printf("Auto-enable gated: flag is ON for ws=%d - skipping EnableRuleOnRuleIntensity for rule %d",
			rule.WorkspaceID, ruleID)
		return nil
	}

	// Flag OFF: run the existing auto-enable logic
	if !rule.IsDisabledByUser {
		if err := qs.rulesService.EnableRuleOnRuleIntensity(ctx, ruleID, 0); err != nil {
			log.Printf("error in auto-enable on intensity: %v", err)
		}
	}

	return nil
}

func (qs *QMMLearningService) appendRuleToPRComment(ctx context.Context, ruleData *RuleData, ruleID int64, isEnabled bool) error {
	// Placeholder for PR comment update logic
	// In real implementation, this would format and append to the PR comment
	status := "enabled"
	if !isEnabled {
		status = "disabled (review in dashboard before enabling)"
	}
	log.Printf("PR Comment: Rule %d created (%s) - Label: QMM Rule [%s]", ruleID, status, ruleData.ReactionType)
	return nil
}

// BulkUpdateRuleStatus updates multiple rules - returns count of actually updated rows
func (qs *QMMLearningService) BulkUpdateRuleStatus(ctx context.Context, wsID int64, ruleIDs []int64, action string) (int, error) {
	// action: "enable" or "disable"
	// This is called by the bulk endpoint handler
	count := 0
	for _, ruleID := range ruleIDs {
		rule, err := qs.repo.GetRule(ctx, ruleID)
		if err != nil || rule == nil || rule.WorkspaceID != wsID {
			continue
		}

		isEnabled := action == "enable"
		isDisabledByUser := action == "disable"

		if err := qs.repo.UpdateRuleStatus(ctx, ruleID, isEnabled, isDisabledByUser); err == nil {
			count++
			log.Printf("BulkUpdate: rule %d set to %s", ruleID, action)
		}
	}
	return count, nil
}
