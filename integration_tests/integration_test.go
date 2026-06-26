package integration

import (
	"context"
	"testing"
)

// Integration tests for the learned_rules_disabled_by_default feature
// These tests verify the complete flow across all services

type TestHarness struct {
	workspaceID int64
	agentID     string
	configAPI   ConfigAPI
	qmmAPI      QMMAPI
	crAPI       CRAPI
}

type ConfigAPI interface {
	GetConfig(ctx context.Context, wsID int64, key string) (interface{}, error)
	SetConfig(ctx context.Context, wsID int64, key string, value interface{}) error
}

type QMMAPI interface {
	CreateLearningEvent(ctx context.Context, wsID int64, agentID string, data interface{}) (int64, error)
	GetRule(ctx context.Context, ruleID int64) (RuleInfo, error)
	BulkUpdateRules(ctx context.Context, wsID int64, action string, scope string, ruleIDs []int64) (int, error)
}

type CRAPI interface {
	ApplyRulesToReview(ctx context.Context, reviewID string) ([]Suggestion, error)
}

type RuleInfo struct {
	RuleID          int64
	IsEnabled       bool
	IsDisabledByUser bool
	RuleType        string
}

type Suggestion struct {
	RuleID  int64
	Text    string
	Applied bool
}

// TestE2E_FlagOffDefaultBehavior verifies that with flag OFF, behavior is identical to today
func TestE2E_FlagOffDefaultBehavior(t *testing.T) {
	t.Run("new negative rule enabled when flag OFF", func(t *testing.T) {
		harness := setupTestHarness(t, 978573, "agent-test-1")
		defer harness.cleanup()

		// Set flag OFF (default)
		if err := harness.configAPI.SetConfig(context.Background(), harness.workspaceID,
			"agent-config.public.learned_rules_disabled_by_default", false); err != nil {
			t.Fatalf("failed to set config: %v", err)
		}

		// Create learning event (emoji reaction)
		ruleID, err := harness.qmmAPI.CreateLearningEvent(context.Background(),
			harness.workspaceID, harness.agentID, map[string]interface{}{
				"type": "dislike",
				"pattern": "bad code pattern",
			})
		if err != nil {
			t.Fatalf("failed to create learning event: %v", err)
		}

		// Verify rule created ENABLED
		rule, err := harness.qmmAPI.GetRule(context.Background(), ruleID)
		if err != nil {
			t.Fatalf("failed to get rule: %v", err)
		}

		if !rule.IsEnabled {
			t.Errorf("expected rule to be enabled when flag OFF, got enabled=%v", rule.IsEnabled)
		}
		if rule.IsDisabledByUser {
			t.Errorf("expected is_disabled_by_user=false, got %v", rule.IsDisabledByUser)
		}
	})
}

// TestE2E_FlagOnNewRulesDisabled verifies that with flag ON, new negative rules are disabled
func TestE2E_FlagOnNewRulesDisabled(t *testing.T) {
	t.Run("new negative rule disabled when flag ON", func(t *testing.T) {
		harness := setupTestHarness(t, 978573, "agent-test-2")
		defer harness.cleanup()

		// Set flag ON
		if err := harness.configAPI.SetConfig(context.Background(), harness.workspaceID,
			"agent-config.public.learned_rules_disabled_by_default", true); err != nil {
			t.Fatalf("failed to set config: %v", err)
		}

		// Create learning event
		ruleID, err := harness.qmmAPI.CreateLearningEvent(context.Background(),
			harness.workspaceID, harness.agentID, map[string]interface{}{
				"type": "dislike",
				"pattern": "bad code pattern",
			})
		if err != nil {
			t.Fatalf("failed to create learning event: %v", err)
		}

		// Verify rule created DISABLED
		rule, err := harness.qmmAPI.GetRule(context.Background(), ruleID)
		if err != nil {
			t.Fatalf("failed to get rule: %v", err)
		}

		if rule.IsEnabled {
			t.Errorf("expected rule to be disabled when flag ON, got enabled=%v", rule.IsEnabled)
		}
		if rule.IsDisabledByUser {
			t.Errorf("expected is_disabled_by_user=false (system-disabled), got %v", rule.IsDisabledByUser)
		}
	})
}

// TestE2E_DisabledRuleNotApplied verifies that disabled rules don't apply in reviews
func TestE2E_DisabledRuleNotApplied(t *testing.T) {
	t.Run("disabled rule produces no suggestions", func(t *testing.T) {
		harness := setupTestHarness(t, 978573, "agent-test-3")
		defer harness.cleanup()

		// Set flag ON (new rules disabled)
		if err := harness.configAPI.SetConfig(context.Background(), harness.workspaceID,
			"agent-config.public.learned_rules_disabled_by_default", true); err != nil {
			t.Fatalf("failed to set config: %v", err)
		}

		// Create learning event
		ruleID, err := harness.qmmAPI.CreateLearningEvent(context.Background(),
			harness.workspaceID, harness.agentID, map[string]interface{}{
				"type": "dislike",
				"pattern": "unnecessary variable",
			})
		if err != nil {
			t.Fatalf("failed to create learning event: %v", err)
		}

		// Verify rule is disabled
		rule, err := harness.qmmAPI.GetRule(context.Background(), ruleID)
		if err != nil || rule.IsEnabled {
			t.Fatalf("expected rule to be disabled")
		}

		// Apply rules to a review
		suggestions, err := harness.crAPI.ApplyRulesToReview(context.Background(), "review-123")
		if err != nil {
			t.Fatalf("failed to apply rules: %v", err)
		}

		// Verify disabled rule NOT applied
		for _, s := range suggestions {
			if s.RuleID == ruleID && s.Applied {
				t.Errorf("expected disabled rule %d to not be applied", ruleID)
			}
		}
	})
}

// TestE2E_BulkEndpointDisableAll verifies bulk endpoint disable operation
func TestE2E_BulkEndpointDisableAll(t *testing.T) {
	t.Run("bulk disable all negative rules", func(t *testing.T) {
		harness := setupTestHarness(t, 978573, "agent-test-4")
		defer harness.cleanup()

		// Create multiple learning events with flag OFF
		if err := harness.configAPI.SetConfig(context.Background(), harness.workspaceID,
			"agent-config.public.learned_rules_disabled_by_default", false); err != nil {
			t.Fatalf("failed to set config: %v", err)
		}

		ruleID1, _ := harness.qmmAPI.CreateLearningEvent(context.Background(),
			harness.workspaceID, harness.agentID, map[string]interface{}{"type": "dislike"})
		ruleID2, _ := harness.qmmAPI.CreateLearningEvent(context.Background(),
			harness.workspaceID, harness.agentID, map[string]interface{}{"type": "avoid"})

		// Verify both rules are enabled
		rule1, _ := harness.qmmAPI.GetRule(context.Background(), ruleID1)
		rule2, _ := harness.qmmAPI.GetRule(context.Background(), ruleID2)
		if !rule1.IsEnabled || !rule2.IsEnabled {
			t.Fatalf("expected rules to be enabled")
		}

		// Use bulk endpoint to disable all
		affected, err := harness.qmmAPI.BulkUpdateRules(context.Background(),
			harness.workspaceID, "disable", "all", nil)
		if err != nil {
			t.Fatalf("bulk update failed: %v", err)
		}

		if affected < 2 {
			t.Errorf("expected at least 2 rules affected, got %d", affected)
		}

		// Verify rules are now disabled
		rule1, _ = harness.qmmAPI.GetRule(context.Background(), ruleID1)
		rule2, _ = harness.qmmAPI.GetRule(context.Background(), ruleID2)
		if rule1.IsEnabled || rule2.IsEnabled {
			t.Errorf("expected rules to be disabled after bulk update")
		}
	})
}

// TestE2E_BulkEndpointEnableSelected verifies bulk endpoint enable operation
func TestE2E_BulkEndpointEnableSelected(t *testing.T) {
	t.Run("bulk enable selected rules", func(t *testing.T) {
		harness := setupTestHarness(t, 978573, "agent-test-5")
		defer harness.cleanup()

		// Create rules with flag ON (disabled)
		if err := harness.configAPI.SetConfig(context.Background(), harness.workspaceID,
			"agent-config.public.learned_rules_disabled_by_default", true); err != nil {
			t.Fatalf("failed to set config: %v", err)
		}

		ruleID1, _ := harness.qmmAPI.CreateLearningEvent(context.Background(),
			harness.workspaceID, harness.agentID, map[string]interface{}{"type": "dislike"})
		ruleID2, _ := harness.qmmAPI.CreateLearningEvent(context.Background(),
			harness.workspaceID, harness.agentID, map[string]interface{}{"type": "avoid"})

		// Bulk enable only ruleID1
		affected, err := harness.qmmAPI.BulkUpdateRules(context.Background(),
			harness.workspaceID, "enable", "selected", []int64{ruleID1})
		if err != nil {
			t.Fatalf("bulk update failed: %v", err)
		}

		if affected != 1 {
			t.Errorf("expected 1 rule affected, got %d", affected)
		}

		// Verify ruleID1 enabled, ruleID2 still disabled
		rule1, _ := harness.qmmAPI.GetRule(context.Background(), ruleID1)
		rule2, _ := harness.qmmAPI.GetRule(context.Background(), ruleID2)

		if !rule1.IsEnabled {
			t.Errorf("expected ruleID1 to be enabled")
		}
		if rule2.IsEnabled {
			t.Errorf("expected ruleID2 to be disabled")
		}
	})
}

// TestE2E_FlagPrecedenceAgentOverWorkspace verifies agent-level config takes precedence
func TestE2E_FlagPrecedenceAgentOverWorkspace(t *testing.T) {
	t.Run("agent level overrides workspace level", func(t *testing.T) {
		harness := setupTestHarness(t, 978573, "agent-test-6")
		defer harness.cleanup()

		// Set workspace level to true (disable)
		harness.configAPI.SetConfig(context.Background(), harness.workspaceID,
			"agent-config.public.learned_rules_disabled_by_default", true)

		// Set agent level to false (enable) - should override
		harness.configAPI.SetConfig(context.Background(), harness.workspaceID,
			"agent-config.learned_rules_disabled_by_default", false)

		// Create learning event
		ruleID, _ := harness.qmmAPI.CreateLearningEvent(context.Background(),
			harness.workspaceID, harness.agentID, map[string]interface{}{"type": "dislike"})

		// Verify rule created ENABLED (agent override takes effect)
		rule, _ := harness.qmmAPI.GetRule(context.Background(), ruleID)
		if !rule.IsEnabled {
			t.Errorf("expected agent-level override to create enabled rule, got enabled=%v", rule.IsEnabled)
		}
	})
}

// TestE2E_ExistingRulesUntouched verifies that existing rules are never modified
func TestE2E_ExistingRulesUntouched(t *testing.T) {
	t.Run("toggling flag doesn't modify existing rules", func(t *testing.T) {
		harness := setupTestHarness(t, 978573, "agent-test-7")
		defer harness.cleanup()

		// Create rule with flag OFF (enabled)
		harness.configAPI.SetConfig(context.Background(), harness.workspaceID,
			"agent-config.public.learned_rules_disabled_by_default", false)
		ruleID, _ := harness.qmmAPI.CreateLearningEvent(context.Background(),
			harness.workspaceID, harness.agentID, map[string]interface{}{"type": "dislike"})

		rule, _ := harness.qmmAPI.GetRule(context.Background(), ruleID)
		originalState := rule.IsEnabled

		// Toggle flag to ON
		harness.configAPI.SetConfig(context.Background(), harness.workspaceID,
			"agent-config.public.learned_rules_disabled_by_default", true)

		// Verify rule state unchanged
		rule, _ = harness.qmmAPI.GetRule(context.Background(), ruleID)
		if rule.IsEnabled != originalState {
			t.Errorf("expected rule state to not change when flag toggled, was %v now %v",
				originalState, rule.IsEnabled)
		}
	})
}

// Helper functions

func setupTestHarness(t *testing.T, wsID int64, agentID string) *TestHarness {
	// Initialize mock APIs
	return &TestHarness{
		workspaceID: wsID,
		agentID:     agentID,
		// In real tests, these would be initialized with actual API clients or mocks
	}
}

func (h *TestHarness) cleanup() {
	// Cleanup test data
}
