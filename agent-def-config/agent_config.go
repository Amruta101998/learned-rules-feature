package config

type AgentConfig struct {
	// Workspace-level configuration
	WorkspaceConfig map[string]interface{} `json:"workspace_config"`
	// Agent-level configuration
	AgentConfig map[string]interface{} `json:"agent_config"`
}

type LearnedRulesConfig struct {
	// Tri-state: nil (unset), true, or false
	DisabledByDefault *bool `json:"learned_rules_disabled_by_default"`
}

// GetLearnedRulesConfig retrieves the learned rules configuration
func (ac *AgentConfig) GetLearnedRulesConfig() *LearnedRulesConfig {
	return &LearnedRulesConfig{
		DisabledByDefault: getTriStateValue(ac.AgentConfig, "learned_rules_disabled_by_default"),
	}
}

// getTriStateValue retrieves a tri-state boolean value from config
// Returns nil if not set, or the boolean value if set
func getTriStateValue(config map[string]interface{}, key string) *bool {
	if val, exists := config[key]; exists {
		if boolVal, ok := val.(bool); ok {
			return &boolVal
		}
	}
	return nil
}

// ResolveLearnedRulesDisabledByDefault resolves the flag with precedence:
// agent value > workspace value > default (false)
func ResolveLearnedRulesDisabledByDefault(agentVal, workspaceVal *bool) bool {
	if agentVal != nil {
		return *agentVal
	}
	if workspaceVal != nil {
		return *workspaceVal
	}
	return false // default
}
