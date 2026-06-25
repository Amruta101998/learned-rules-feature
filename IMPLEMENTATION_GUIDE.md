# Learning Rules Disable-by-Default Feature

**BITO-13843: Give workspaces control over learned rule activation and application**

This implementation guide describes the changes needed to implement the `learned_rules_disabled_by_default` flag feature across all services.

## Feature Summary

A new workspace/agent-level flag `learned_rules_disabled_by_default` that controls whether newly learned **negative** rules are created in a disabled state. When enabled, learned negative rules won't be applied in reviews until someone explicitly enables them from the Learned Rules dashboard.

### Key Behaviors

- **Tri-state flag**: `true`, `false`, or unset (unset defaults to `false`)
- **Precedence**: agent value > workspace value > default (`false`)
- **Applies to**: Newly learned negative rules only
- **Positive rules**: Always enabled, unaffected by the flag
- **Existing rules**: Never modified by the flag
- **Opt-in**: Defaults to today's behavior (all rules enabled)

## Implementation Changes by Service

### 1. agent-def-config

**File**: `agent-def-config/agent_config.go`

Define the tri-state configuration flag at workspace and agent levels:

```go
type LearnedRulesConfig struct {
    DisabledByDefault *bool // nil = unset, true/false = explicit value
}

// Resolve with precedence: agent > workspace > default (false)
func ResolveLearnedRulesDisabledByDefault(agentVal, workspaceVal *bool) bool
```

**Configuration levels**:
- **Workspace**: `agent-config.public.learned_rules_disabled_by_default` (BOOLEAN, default false)
- **Agent**: JSON field `learned_rules_disabled_by_default` in `agent_config_instances.config_value`

### 2. agent-exec-mgmt

**File**: `agent-exec-mgmt/learning_processor.go`

Resolves the flag at event time and passes it into learning event payloads:

```go
func (lp *LearningProcessor) ProcessLearningEvent(ctx context.Context, wsID int64, agentID string, ruleData interface{}) (*LearningEvent, error)
```

The resolved boolean value is included in:
- `QMMLearningEvent` (v0 learning)
- `AdaptiveLearningEvent` (v1 learning)

**Log output**:
```
LearningProcessor: resolved learned_rules_disabled_by_default=true for ws=978573, agent=agent-123
```

### 3. quality-measurement-manager (v0 — QMM Learning)

**File**: `quality-measurement-manager/learning_service.go`

#### SaveRule — Create rule with resolved disabled state

```go
func (qs *QMMLearningService) SaveRule(ctx context.Context, ruleData *RuleData) (int64, error)
```

- Reads the flag from config
- Creates new negative rules with `is_enabled = !flag`
- v0 only produces negative rules
- Appends rule status to PR comment (disabled rules show "review in dashboard before enabling")

#### IncrementRuleIntensityAndCheckAutoEnable — Gate auto-enable

```go
func (qs *QMMLearningService) IncrementRuleIntensityAndCheckAutoEnable(ctx context.Context, ruleID int64, increment int) error
```

- Always increments rule intensity
- **When flag ON**: Skips `EnableRuleOnRuleIntensity` (auto-enable gated)
- **When flag OFF**: Runs existing auto-enable logic
- **Edge case**: Intensity increments while disabled; turning flag OFF resumes auto-enable on next recurrence (no backfill needed)

**Protection logic**: User-disabled rules (identified by `is_disabled_by_user=1`) are never auto-enabled regardless of flag state.

### 4. adaptive-learning (v1 — Adaptive Learning)

**File**: `adaptive-learning/embedding_service.go`

#### InsertLearnedRuleAndMetadata — Create rule with flag-aware enabled state

```go
func (es *EmbeddingService) InsertLearnedRuleAndMetadata(ctx context.Context, clusterEvent *ClusterUpdateEvent, ruleData *AdaptiveRuleData) (int64, error)
```

- **Negative rules**: `is_enabled = !flag` (disabled when flag ON)
- **Positive rules**: Always `is_enabled = true` (unaffected)
- Sets `is_disabled_by_user = 0` for system-disabled rules

#### SyncLearnedRuleWithCluster — Handle rule type flips

```go
func (es *EmbeddingService) SyncLearnedRuleWithCluster(ctx context.Context, clusterEvent *ClusterUpdateEvent) error
```

When a rule's type changes:

| Flip | Action |
| --- | --- |
| positive → negative | Set `is_enabled = !flag` (unless user-disabled) |
| negative → positive | Set `is_enabled = true` (always enable) |
| Same type | No change |

**Preserve user-disabled**: Never override `is_disabled_by_user=1` on type flip.

### 5. automation-platform (CRA)

**File**: `automation-platform/learning_filter.go`

#### ApplyLearningToFeedbacks — Apply only enabled rules

```go
func (lf *LearningFilter) ApplyLearningToFeedbacks(ctx context.Context, review *Review) error
```

- Queries both v0 and v1 learned rules
- Both queries filter `is_enabled=1` (disabled rules never apply)
- **Simplified design**: No new application flag needed; the creation-time disabled state is sufficient

## Database Schema

### Migration: Add `is_disabled_by_user` Column

**File**: `migrations/001_add_is_disabled_by_user.sql`

The `cra_rule_metadata` table already has `is_enabled`. This migration adds `is_disabled_by_user`:

```sql
ALTER TABLE cra_rule_metadata ADD COLUMN is_disabled_by_user TINYINT(1) NOT NULL DEFAULT 0;
```

**Semantics**:
| State | is_enabled | is_disabled_by_user | Meaning |
| --- | --- | --- | --- |
| Enabled | 1 | 0 | Active, applies in reviews |
| System-disabled | 0 | 0 | Disabled by the flag, needs review |
| User-disabled | 0 | 1 | User explicitly turned it off |

**Query to verify**:
```sql
SELECT
  id, rule_id, is_enabled, is_disabled_by_user,
  CASE
    WHEN is_enabled=1 THEN 'Enabled'
    WHEN is_enabled=0 AND is_disabled_by_user=0 THEN 'System-disabled'
    WHEN is_enabled=0 AND is_disabled_by_user=1 THEN 'User-disabled'
  END AS status
FROM cra_rule_metadata WHERE ws_id = 978573;
```

## Internal Bulk Status Endpoint

**Endpoint**: `PATCH /qmm/api/v1/learning-rules/bulk-status`

**Files**: 
- `quality-measurement-manager/bulk_status_handler.go`
- `quality-measurement-manager/api_handler.go`

### Request

```json
{
  "workspace_id": 978573,
  "action": "enable|disable",
  "scope": "all|selected",
  "rule_ids": [489, 490]  // Required for scope=selected
}
```

### Response

```json
{
  "status": {
    "code": "1000",
    "message": "Rules updated successfully"
  },
  "data": {
    "affected_count": 2,
    "action": "enable",
    "workspace_id": 978573
  }
}
```

### Behavior

- **Negatives-only**: Filters on `(version=0 OR rule_type='negative')`, never modifies positive rules
- **scope=all**: Updates all negative rules in the workspace
- **scope=selected**: Updates only specified rule IDs (positive IDs are silently skipped)
- **On disable**: Sets `is_disabled_by_user=1` (protects from auto-enable)
- **On enable**: Sets `is_disabled_by_user=0` (re-arms auto-enable if applicable)
- **Idempotent**: Repeated calls with same state return `affected_count=0`
- **Validation** (400): Missing `workspace_id`, invalid `action`/`scope`, mismatched `rule_ids` and `scope`

**Example curls**:

```bash
# Disable selected rules
curl -X PATCH https://preprod.bito.ai/qmm/api/v1/learning-rules/bulk-status \
  -H 'Content-Type: application/json' \
  -d '{
    "workspace_id": 978573,
    "action": "disable",
    "scope": "selected",
    "rule_ids": [489, 490]
  }'

# Enable all negative rules in workspace
curl -X PATCH https://preprod.bito.ai/qmm/api/v1/learning-rules/bulk-status \
  -H 'Content-Type: application/json' \
  -d '{
    "workspace_id": 978573,
    "action": "enable",
    "scope": "all"
  }'
```

## Setting the Flag

### Workspace Level

```bash
curl --location 'https://preprod.bito.ai/config/set' \
--header 'Authorization: <TOKEN>' \
--header 'Content-Type: application/json' \
--data '{
  "workspace_id": 978573,
  "config_key": "agent-config.public.learned_rules_disabled_by_default",
  "config_value": "true",
  "module": "agent-config",
  "data_type": "BOOLEAN"
}'
```

### Agent Level

```sql
UPDATE agent_mgmt.agent_config_instances
SET config_value = JSON_SET(
  config_value,
  '$.learned_rules_disabled_by_default',
  true
)
WHERE workspace_id = 978573
  AND id = '<agent-instance-id>';
```

**Query to read**:
```sql
SELECT
  id,
  JSON_EXTRACT(config_value, '$.learned_rules_disabled_by_default') AS learned_rules_disabled_by_default
FROM agent_mgmt.agent_config_instances
WHERE workspace_id = 978573 AND id = '<agent-instance-id>';
```

## Unit Tests

### QMM Learning Service

**File**: `quality-measurement-manager/learning_service_test.go`

- `TestSaveRule_FlagOff_RuleEnabled` — Flag OFF creates enabled rule
- `TestSaveRule_FlagOn_RuleDisabled` — Flag ON creates disabled rule
- `TestAutoEnableGating_FlagOn_SkipsAutoEnable` — Auto-enable skipped when flag ON
- `TestAutoEnableGating_FlagOff_AllowsAutoEnable` — Auto-enable runs when flag OFF
- `TestBulkUpdateRuleStatus_DisableSetting` — Bulk disable sets `is_disabled_by_user=1`

### Adaptive Learning Service

**File**: `adaptive-learning/embedding_service_test.go`

- `TestInsertNegativeRule_FlagOn_RuleDisabled` — Negative rule disabled when flag ON
- `TestInsertNegativeRule_FlagOff_RuleEnabled` — Negative rule enabled when flag OFF
- `TestInsertPositiveRule_AlwaysEnabled` — Positive rules always enabled
- `TestTypeFlapPositiveToNegative_FlagOn_Disabled` — Positive→negative flip respects flag
- `TestTypeFlipNegativeToPositive_AlwaysEnabled` — Negative→positive flip always enables
- `TestTypeFlipPreservesUserDisabled` — User-disabled flag preserved on flip

## Testing Checklist

See [BITO-13843 comment 177776](https://bito.atlassian.net/browse/BITO-13843?focusedCommentId=177776) for full test plan.

### Core Scenarios

- [ ] Flag OFF (default): Behavior identical to today
- [ ] Flag ON, v0 learning: New negative rules appear disabled
- [ ] Flag ON, v1 learning: New negative rules appear disabled
- [ ] v0 intensity auto-enable: Gated when flag ON
- [ ] v1 type flip (positive→negative): Respects flag
- [ ] v1 type flip (negative→positive): Always enabled
- [ ] Existing rules untouched: Flag toggle doesn't affect pre-existing rules
- [ ] Per-rule toggle still works: Manual enable/disable still functional
- [ ] Bulk endpoint: Correctly updates selected/all negatives
- [ ] Not applied in review: Disabled rules don't produce suggestions

### Configuration

- [ ] Workspace level flag sets correctly
- [ ] Agent level flag overrides workspace
- [ ] Default (unset) treated as false
- [ ] Flag value propagates through all services

## Rollout Plan

1. **Deploy agent-def-config** — Define flags (no-op until set)
2. **Deploy agent-exec-mgmt** — Resolve and pass flags
3. **Deploy quality-measurement-manager** — v0 implementation + bulk endpoint
4. **Deploy adaptive-learning** — v1 implementation
5. **Deploy automation-platform** — Confirm learning queries filter `is_enabled=1`
6. **Set flag on Mindtickle workspace** (978573) as opt-in customer
7. **Backfill** (if needed): Use bulk endpoint to disable existing rules for review

## Backward Compatibility

- **Default behavior preserved**: Flag unset defaults to false (today's behavior)
- **Existing rules unchanged**: Flag never modifies pre-existing rules
- **No UI/UX changes**: Learned Rules section UI unchanged; same toggle applies
- **Per-rule endpoint unchanged**: `PATCH /learning-rules/:rule_id/status` still works

## References

- **BITO-13843**: https://bito.atlassian.net/browse/BITO-13843
- **Implementation plan**: https://bito.atlassian.net/browse/BITO-13843?focusedCommentId=171274
- **Detailed behavior & edge cases**: https://bito.atlassian.net/browse/BITO-13843?focusedCommentId=171282
- **Bulk endpoint contract**: https://bito.atlassian.net/browse/BITO-13843?focusedCommentId=177515
- **Test plan**: https://bito.atlassian.net/browse/BITO-13843?focusedCommentId=177776
