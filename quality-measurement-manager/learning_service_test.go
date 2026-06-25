package learning

import (
	"context"
	"testing"
)

// Mock implementations for testing
type mockRulesRepo struct {
	rules map[int64]*RuleMetadata
}

func (m *mockRulesRepo) CreateRule(ctx context.Context, wsID int64, ruleData *RuleData, isEnabled bool) (int64, error) {
	ruleID := int64(len(m.rules) + 1)
	m.rules[ruleID] = &RuleMetadata{
		RuleID:           ruleID,
		RuleType:         "negative",
		IsEnabled:        isEnabled,
		IsDisabledByUser: false,
		WorkspaceID:      wsID,
		Version:          0,
	}
	return ruleID, nil
}

func (m *mockRulesRepo) UpdateRuleStatus(ctx context.Context, ruleID int64, isEnabled bool, isDisabledByUser bool) error {
	if rule, ok := m.rules[ruleID]; ok {
		rule.IsEnabled = isEnabled
		rule.IsDisabledByUser = isDisabledByUser
	}
	return nil
}

func (m *mockRulesRepo) GetRule(ctx context.Context, ruleID int64) (*RuleMetadata, error) {
	return m.rules[ruleID], nil
}

func (m *mockRulesRepo) GetRulesByWorkspace(ctx context.Context, wsID int64) ([]*RuleMetadata, error) {
	var rules []*RuleMetadata
	for _, rule := range m.rules {
		if rule.WorkspaceID == wsID {
			rules = append(rules, rule)
		}
	}
	return rules, nil
}

type mockConfigProvider struct {
	disabledByDefault bool
}

func (m *mockConfigProvider) GetLearnedRulesDisabledByDefault(ctx context.Context, wsID int64) (bool, error) {
	return m.disabledByDefault, nil
}

type mockRulesService struct {
	autoEnabledRules map[int64]bool
}

func (m *mockRulesService) EnableRuleOnRuleIntensity(ctx context.Context, ruleID int64, intensity int) error {
	m.autoEnabledRules[ruleID] = true
	return nil
}

func (m *mockRulesService) UpdateRuleIntensity(ctx context.Context, ruleID int64, increment int) error {
	return nil
}

func TestSaveRule_FlagOff_RuleEnabled(t *testing.T) {
	repo := &mockRulesRepo{rules: make(map[int64]*RuleMetadata)}
	configProvider := &mockConfigProvider{disabledByDefault: false}
	rulesService := &mockRulesService{autoEnabledRules: make(map[int64]bool)}

	service := NewQMMLearningService(repo, configProvider, rulesService)

	ruleData := &RuleData{
		Pattern:      "test pattern",
		ReviewText:   "review text",
		PRNumber:     123,
		WorkspaceID:  978573,
		AgentID:      "agent-123",
		Confidence:   0.95,
		ReactionType: "dislike",
	}

	ruleID, err := service.SaveRule(context.Background(), ruleData)
	if err != nil {
		t.Fatalf("SaveRule failed: %v", err)
	}

	rule := repo.rules[ruleID]
	if !rule.IsEnabled {
		t.Errorf("expected IsEnabled=true when flag is OFF, got %v", rule.IsEnabled)
	}
	if rule.IsDisabledByUser {
		t.Errorf("expected IsDisabledByUser=false, got %v", rule.IsDisabledByUser)
	}
}

func TestSaveRule_FlagOn_RuleDisabled(t *testing.T) {
	repo := &mockRulesRepo{rules: make(map[int64]*RuleMetadata)}
	configProvider := &mockConfigProvider{disabledByDefault: true}
	rulesService := &mockRulesService{autoEnabledRules: make(map[int64]bool)}

	service := NewQMMLearningService(repo, configProvider, rulesService)

	ruleData := &RuleData{
		Pattern:      "test pattern",
		ReviewText:   "review text",
		PRNumber:     123,
		WorkspaceID:  978573,
		AgentID:      "agent-123",
		Confidence:   0.95,
		ReactionType: "dislike",
	}

	ruleID, err := service.SaveRule(context.Background(), ruleData)
	if err != nil {
		t.Fatalf("SaveRule failed: %v", err)
	}

	rule := repo.rules[ruleID]
	if rule.IsEnabled {
		t.Errorf("expected IsEnabled=false when flag is ON, got %v", rule.IsEnabled)
	}
	if rule.IsDisabledByUser {
		t.Errorf("expected IsDisabledByUser=false (system-disabled), got %v", rule.IsDisabledByUser)
	}
}

func TestAutoEnableGating_FlagOn_SkipsAutoEnable(t *testing.T) {
	repo := &mockRulesRepo{rules: make(map[int64]*RuleMetadata)}
	configProvider := &mockConfigProvider{disabledByDefault: true}
	rulesService := &mockRulesService{autoEnabledRules: make(map[int64]bool)}

	service := NewQMMLearningService(repo, configProvider, rulesService)

	// Create a disabled rule
	repo.rules[1] = &RuleMetadata{
		RuleID:           1,
		RuleType:         "negative",
		IsEnabled:        false,
		IsDisabledByUser: false,
		WorkspaceID:      978573,
		Version:          0,
	}

	// Try to auto-enable via intensity increment
	err := service.IncrementRuleIntensityAndCheckAutoEnable(context.Background(), 1)
	if err != nil {
		t.Fatalf("IncrementRuleIntensityAndCheckAutoEnable failed: %v", err)
	}

	// Rule should NOT be auto-enabled because flag is ON
	if len(rulesService.autoEnabledRules) > 0 {
		t.Errorf("expected auto-enable to be skipped when flag is ON, but it was called")
	}
}

func TestAutoEnableGating_FlagOff_AllowsAutoEnable(t *testing.T) {
	repo := &mockRulesRepo{rules: make(map[int64]*RuleMetadata)}
	configProvider := &mockConfigProvider{disabledByDefault: false}
	rulesService := &mockRulesService{autoEnabledRules: make(map[int64]bool)}

	service := NewQMMLearningService(repo, configProvider, rulesService)

	// Create a disabled rule
	repo.rules[1] = &RuleMetadata{
		RuleID:           1,
		RuleType:         "negative",
		IsEnabled:        false,
		IsDisabledByUser: false,
		WorkspaceID:      978573,
		Version:          0,
	}

	// Try to auto-enable via intensity increment
	err := service.IncrementRuleIntensityAndCheckAutoEnable(context.Background(), 1)
	if err != nil {
		t.Fatalf("IncrementRuleIntensityAndCheckAutoEnable failed: %v", err)
	}

	// Rule SHOULD be auto-enabled because flag is OFF
	if _, ok := rulesService.autoEnabledRules[1]; !ok {
		t.Errorf("expected auto-enable to be called when flag is OFF")
	}
}

func TestBulkUpdateRuleStatus_DisableSetting(t *testing.T) {
	repo := &mockRulesRepo{rules: make(map[int64]*RuleMetadata)}
	configProvider := &mockConfigProvider{disabledByDefault: false}
	rulesService := &mockRulesService{autoEnabledRules: make(map[int64]bool)}

	service := NewQMMLearningService(repo, configProvider, rulesService)

	// Create some rules
	repo.rules[1] = &RuleMetadata{RuleID: 1, IsEnabled: true, IsDisabledByUser: false, WorkspaceID: 978573}
	repo.rules[2] = &RuleMetadata{RuleID: 2, IsEnabled: true, IsDisabledByUser: false, WorkspaceID: 978573}

	// Bulk disable
	count, err := service.BulkUpdateRuleStatus(context.Background(), 978573, []int64{1, 2}, "disable")
	if err != nil {
		t.Fatalf("BulkUpdateRuleStatus failed: %v", err)
	}

	if count != 2 {
		t.Errorf("expected count=2, got %d", count)
	}

	for _, id := range []int64{1, 2} {
		rule := repo.rules[id]
		if rule.IsEnabled {
			t.Errorf("rule %d should be disabled", id)
		}
		if !rule.IsDisabledByUser {
			t.Errorf("rule %d should have IsDisabledByUser=true", id)
		}
	}
}
