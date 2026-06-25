package learning

import (
	"context"
	"testing"
	"time"
)

// Mock implementations for adaptive learning tests
type mockAdaptiveRulesRepo struct {
	rules map[int64]*AdaptiveRuleMetadata
}

func (m *mockAdaptiveRulesRepo) InsertLearnedRuleAndMetadata(ctx context.Context, wsID int64, ruleData *AdaptiveRuleData, isEnabled bool) (int64, error) {
	ruleID := int64(len(m.rules) + 1)
	m.rules[ruleID] = &AdaptiveRuleMetadata{
		RuleID:          ruleID,
		ClusterID:       ruleData.ClusterID,
		RuleType:        ruleData.RuleType,
		IsEnabled:       isEnabled,
		IsDisabledByUser: false,
		WorkspaceID:     wsID,
		Version:         1,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}
	return ruleID, nil
}

func (m *mockAdaptiveRulesRepo) UpdateRuleType(ctx context.Context, ruleID int64, newType string) error {
	if rule, ok := m.rules[ruleID]; ok {
		rule.RuleType = newType
	}
	return nil
}

func (m *mockAdaptiveRulesRepo) UpdateRuleEnabledState(ctx context.Context, ruleID int64, isEnabled bool, isDisabledByUser bool) error {
	if rule, ok := m.rules[ruleID]; ok {
		rule.IsEnabled = isEnabled
		rule.IsDisabledByUser = isDisabledByUser
	}
	return nil
}

func (m *mockAdaptiveRulesRepo) GetRuleByClusterID(ctx context.Context, clusterID string) (*AdaptiveRuleMetadata, error) {
	for _, rule := range m.rules {
		if rule.ClusterID == clusterID {
			return rule, nil
		}
	}
	return nil, nil
}

type mockAdaptiveConfigProvider struct {
	disabledByDefault bool
}

func (m *mockAdaptiveConfigProvider) GetLearnedRulesDisabledByDefault(ctx context.Context, wsID int64) (bool, error) {
	return m.disabledByDefault, nil
}

func TestInsertNegativeRule_FlagOn_RuleDisabled(t *testing.T) {
	repo := &mockAdaptiveRulesRepo{rules: make(map[int64]*AdaptiveRuleMetadata)}
	configProvider := &mockAdaptiveConfigProvider{disabledByDefault: true}

	service := NewEmbeddingService(repo, configProvider)

	clusterEvent := &ClusterUpdateEvent{
		ClusterID:         "cluster-123",
		WorkspaceID:       978573,
		AgentID:           "agent-456",
		PreviousType:      "",
		CurrentType:       "negative",
		Sentiment:         -0.8,
		ConfidenceScore:   0.95,
		DisabledByDefault: true,
	}

	ruleData := &AdaptiveRuleData{
		ClusterID:       "cluster-123",
		Pattern:         "negative pattern",
		WorkspaceID:     978573,
		AgentID:         "agent-456",
		RuleType:        "negative",
		ConfidenceScore: 0.95,
	}

	ruleID, err := service.InsertLearnedRuleAndMetadata(context.Background(), clusterEvent, ruleData)
	if err != nil {
		t.Fatalf("InsertLearnedRuleAndMetadata failed: %v", err)
	}

	rule := repo.rules[ruleID]
	if rule.IsEnabled {
		t.Errorf("expected IsEnabled=false for negative rule when flag is ON, got %v", rule.IsEnabled)
	}
	if rule.IsDisabledByUser {
		t.Errorf("expected IsDisabledByUser=false (system-disabled), got %v", rule.IsDisabledByUser)
	}
}

func TestInsertNegativeRule_FlagOff_RuleEnabled(t *testing.T) {
	repo := &mockAdaptiveRulesRepo{rules: make(map[int64]*AdaptiveRuleMetadata)}
	configProvider := &mockAdaptiveConfigProvider{disabledByDefault: false}

	service := NewEmbeddingService(repo, configProvider)

	clusterEvent := &ClusterUpdateEvent{
		ClusterID:         "cluster-123",
		WorkspaceID:       978573,
		AgentID:           "agent-456",
		PreviousType:      "",
		CurrentType:       "negative",
		Sentiment:         -0.8,
		ConfidenceScore:   0.95,
		DisabledByDefault: false,
	}

	ruleData := &AdaptiveRuleData{
		ClusterID:       "cluster-123",
		Pattern:         "negative pattern",
		WorkspaceID:     978573,
		AgentID:         "agent-456",
		RuleType:        "negative",
		ConfidenceScore: 0.95,
	}

	ruleID, err := service.InsertLearnedRuleAndMetadata(context.Background(), clusterEvent, ruleData)
	if err != nil {
		t.Fatalf("InsertLearnedRuleAndMetadata failed: %v", err)
	}

	rule := repo.rules[ruleID]
	if !rule.IsEnabled {
		t.Errorf("expected IsEnabled=true for negative rule when flag is OFF, got %v", rule.IsEnabled)
	}
}

func TestInsertPositiveRule_AlwaysEnabled(t *testing.T) {
	repo := &mockAdaptiveRulesRepo{rules: make(map[int64]*AdaptiveRuleMetadata)}
	configProvider := &mockAdaptiveConfigProvider{disabledByDefault: true}

	service := NewEmbeddingService(repo, configProvider)

	clusterEvent := &ClusterUpdateEvent{
		ClusterID:         "cluster-positive",
		WorkspaceID:       978573,
		AgentID:           "agent-456",
		PreviousType:      "",
		CurrentType:       "positive",
		Sentiment:         0.8,
		ConfidenceScore:   0.95,
		DisabledByDefault: true,
	}

	ruleData := &AdaptiveRuleData{
		ClusterID:       "cluster-positive",
		Pattern:         "positive pattern",
		WorkspaceID:     978573,
		AgentID:         "agent-456",
		RuleType:        "positive",
		ConfidenceScore: 0.95,
	}

	ruleID, err := service.InsertLearnedRuleAndMetadata(context.Background(), clusterEvent, ruleData)
	if err != nil {
		t.Fatalf("InsertLearnedRuleAndMetadata failed: %v", err)
	}

	rule := repo.rules[ruleID]
	if !rule.IsEnabled {
		t.Errorf("expected positive rules to always be enabled, got %v", rule.IsEnabled)
	}
}

func TestTypeFlapPositiveToNegative_FlagOn_Disabled(t *testing.T) {
	repo := &mockAdaptiveRulesRepo{rules: make(map[int64]*AdaptiveRuleMetadata)}
	configProvider := &mockAdaptiveConfigProvider{disabledByDefault: true}

	service := NewEmbeddingService(repo, configProvider)

	// Seed a positive rule
	repo.rules[1] = &AdaptiveRuleMetadata{
		RuleID:           1,
		ClusterID:        "cluster-flip",
		RuleType:         "positive",
		IsEnabled:        true,
		IsDisabledByUser: false,
		WorkspaceID:      978573,
		Version:          1,
	}

	// Flip positive → negative
	clusterEvent := &ClusterUpdateEvent{
		ClusterID:         "cluster-flip",
		WorkspaceID:       978573,
		AgentID:           "agent-456",
		PreviousType:      "positive",
		CurrentType:       "negative",
		Sentiment:         -0.8,
		ConfidenceScore:   0.95,
		DisabledByDefault: true,
	}

	err := service.SyncLearnedRuleWithCluster(context.Background(), clusterEvent)
	if err != nil {
		t.Fatalf("SyncLearnedRuleWithCluster failed: %v", err)
	}

	rule := repo.rules[1]
	if rule.RuleType != "negative" {
		t.Errorf("expected type to flip to negative, got %v", rule.RuleType)
	}
	if rule.IsEnabled {
		t.Errorf("expected rule to be disabled after positive→negative flip with flag ON, got %v", rule.IsEnabled)
	}
}

func TestTypeFlipNegativeToPositive_AlwaysEnabled(t *testing.T) {
	repo := &mockAdaptiveRulesRepo{rules: make(map[int64]*AdaptiveRuleMetadata)}
	configProvider := &mockAdaptiveConfigProvider{disabledByDefault: true}

	service := NewEmbeddingService(repo, configProvider)

	// Seed a negative rule that was disabled
	repo.rules[1] = &AdaptiveRuleMetadata{
		RuleID:           1,
		ClusterID:        "cluster-flip",
		RuleType:         "negative",
		IsEnabled:        false,
		IsDisabledByUser: false,
		WorkspaceID:      978573,
		Version:          1,
	}

	// Flip negative → positive
	clusterEvent := &ClusterUpdateEvent{
		ClusterID:         "cluster-flip",
		WorkspaceID:       978573,
		AgentID:           "agent-456",
		PreviousType:      "negative",
		CurrentType:       "positive",
		Sentiment:         0.8,
		ConfidenceScore:   0.95,
		DisabledByDefault: true,
	}

	err := service.SyncLearnedRuleWithCluster(context.Background(), clusterEvent)
	if err != nil {
		t.Fatalf("SyncLearnedRuleWithCluster failed: %v", err)
	}

	rule := repo.rules[1]
	if rule.RuleType != "positive" {
		t.Errorf("expected type to flip to positive, got %v", rule.RuleType)
	}
	if !rule.IsEnabled {
		t.Errorf("expected positive rules to always be enabled after flip, got %v", rule.IsEnabled)
	}
}

func TestTypeFlipPreservesUserDisabled(t *testing.T) {
	repo := &mockAdaptiveRulesRepo{rules: make(map[int64]*AdaptiveRuleMetadata)}
	configProvider := &mockAdaptiveConfigProvider{disabledByDefault: true}

	service := NewEmbeddingService(repo, configProvider)

	// Seed a positive rule that was manually disabled by user
	repo.rules[1] = &AdaptiveRuleMetadata{
		RuleID:           1,
		ClusterID:        "cluster-flip",
		RuleType:         "positive",
		IsEnabled:        false,
		IsDisabledByUser: true, // User explicitly disabled it
		WorkspaceID:      978573,
		Version:          1,
	}

	// Flip positive → negative
	clusterEvent := &ClusterUpdateEvent{
		ClusterID:         "cluster-flip",
		WorkspaceID:       978573,
		AgentID:           "agent-456",
		PreviousType:      "positive",
		CurrentType:       "negative",
		Sentiment:         -0.8,
		ConfidenceScore:   0.95,
		DisabledByDefault: true,
	}

	err := service.SyncLearnedRuleWithCluster(context.Background(), clusterEvent)
	if err != nil {
		t.Fatalf("SyncLearnedRuleWithCluster failed: %v", err)
	}

	rule := repo.rules[1]
	if rule.IsEnabled {
		t.Errorf("expected rule to stay disabled (user-disabled), got %v", rule.IsEnabled)
	}
	if !rule.IsDisabledByUser {
		t.Errorf("expected IsDisabledByUser to be preserved, got %v", rule.IsDisabledByUser)
	}
}
