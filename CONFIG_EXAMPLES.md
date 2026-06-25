# Configuration Examples

This guide shows how to configure the `learned_rules_disabled_by_default` flag in different scenarios.

## Scenario 1: Enable for Entire Workspace (Mindtickle)

**Goal**: Disable newly learned rules by default for the Mindtickle workspace so the team can review them before activation.

### Set at Workspace Level

```bash
curl --location 'https://preprod.bito.ai/config/set' \
--header 'Authorization: <YOUR_TOKEN>' \
--header 'Content-Type: application/json' \
--data '{
  "workspace_id": 978573,
  "config_key": "agent-config.public.learned_rules_disabled_by_default",
  "config_value": "true",
  "module": "agent-config",
  "data_type": "BOOLEAN"
}'
```

**Verification**:
```bash
# Verify the flag is set
curl --location 'https://preprod.bito.ai/config/get' \
--header 'Authorization: <YOUR_TOKEN>' \
--data '{
  "workspace_id": 978573,
  "config_key": "agent-config.public.learned_rules_disabled_by_default"
}'
```

**Result**: All agents in this workspace will create new negative rules disabled by default.

---

## Scenario 2: Override for Specific Agent

**Goal**: Enable the flag for most agents, but override it for one specific agent that is in beta testing.

### Set at Agent Level (Overrides Workspace)

```bash
# Get the agent config instance first
mysql> SELECT id, workspace_id FROM agent_mgmt.agent_config_instances 
        WHERE workspace_id = 978573 AND agent_id = 'agent-beta-123';
```

Then update:

```sql
UPDATE agent_mgmt.agent_config_instances
SET config_value = JSON_SET(
  config_value,
  '$.learned_rules_disabled_by_default',
  false
)
WHERE workspace_id = 978573
  AND id = 'agent-config-instance-789';
```

**Verification**:
```sql
SELECT 
  id,
  JSON_EXTRACT(config_value, '$.learned_rules_disabled_by_default') AS flag_value
FROM agent_mgmt.agent_config_instances
WHERE workspace_id = 978573 AND id = 'agent-config-instance-789';
```

**Result**: This agent ignores the workspace setting and creates rules enabled (flag OFF).

---

## Scenario 3: Gradual Rollout with Per-Agent Control

**Goal**: Enable the feature progressively for different agents within the same workspace.

### Step 1: Set Workspace Default to OFF (no change)

```bash
curl --location 'https://preprod.bito.ai/config/set' \
--header 'Authorization: <YOUR_TOKEN>' \
--header 'Content-Type: application/json' \
--data '{
  "workspace_id": 978573,
  "config_key": "agent-config.public.learned_rules_disabled_by_default",
  "config_value": "false",
  "module": "agent-config",
  "data_type": "BOOLEAN"
}'
```

### Step 2: Enable for Pilot Agents Only

```sql
-- Agent 1: Enable for pilot
UPDATE agent_mgmt.agent_config_instances
SET config_value = JSON_SET(config_value, '$.learned_rules_disabled_by_default', true)
WHERE workspace_id = 978573 AND agent_id = 'agent-pilot-1';

-- Agent 2: Enable for pilot
UPDATE agent_mgmt.agent_config_instances
SET config_value = JSON_SET(config_value, '$.learned_rules_disabled_by_default', true)
WHERE workspace_id = 978573 AND agent_id = 'agent-pilot-2';
```

### Step 3: Monitor & Expand

After 1 week, if successful:
```sql
-- Enable for production agents
UPDATE agent_mgmt.agent_config_instances
SET config_value = JSON_SET(config_value, '$.learned_rules_disabled_by_default', true)
WHERE workspace_id = 978573 AND agent_id IN ('agent-prod-1', 'agent-prod-2', 'agent-prod-3');
```

---

## Scenario 4: Backfill Existing Rules Using Bulk Endpoint

**Goal**: After enabling the flag, disable all existing negative rules so the team can review them.

### 1. Verify current rules

```sql
SELECT 
  id, rule_id, is_enabled, is_disabled_by_user,
  CASE
    WHEN is_enabled=1 THEN 'Enabled'
    WHEN is_enabled=0 AND is_disabled_by_user=0 THEN 'System-disabled'
    WHEN is_enabled=0 AND is_disabled_by_user=1 THEN 'User-disabled'
  END AS status
FROM cra_rule_metadata
WHERE ws_id = 978573
ORDER BY id DESC LIMIT 20;
```

### 2. Disable all negative rules

```bash
curl -X PATCH 'https://preprod.bito.ai/qmm/api/v1/learning-rules/bulk-status' \
--header 'Content-Type: application/json' \
--data '{
  "workspace_id": 978573,
  "action": "disable",
  "scope": "all"
}'
```

**Response**:
```json
{
  "status": {
    "code": "1000",
    "message": "Rules updated successfully"
  },
  "data": {
    "affected_count": 47,
    "action": "disable",
    "workspace_id": 978573
  }
}
```

### 3. Verify the backfill

```sql
SELECT COUNT(*) as disabled_count
FROM cra_rule_metadata
WHERE ws_id = 978573 AND is_enabled = 0 AND is_disabled_by_user = 1;
-- Should show: 47
```

### 4. Enable selected rules (after review)

```bash
curl -X PATCH 'https://preprod.bito.ai/qmm/api/v1/learning-rules/bulk-status' \
--header 'Content-Type: application/json' \
--data '{
  "workspace_id": 978573,
  "action": "enable",
  "scope": "selected",
  "rule_ids": [489, 490, 491, 492, 495]
}'
```

---

## Scenario 5: Testing on Staging

**Goal**: Test the feature on staging with workspace ID 2858.

### Setup

```bash
# Set flag for staging workspace
curl --location 'https://staging.bito.ai/config/set' \
--header 'Authorization: <STAGING_TOKEN>' \
--header 'Content-Type: application/json' \
--data '{
  "workspace_id": 2858,
  "config_key": "agent-config.public.learned_rules_disabled_by_default",
  "config_value": "true",
  "module": "agent-config",
  "data_type": "BOOLEAN"
}'
```

### Test Workflow

1. **Generate new learning event** (emoji reaction or cluster update)
2. **Verify rule created disabled**:
```sql
SELECT * FROM cra_rule_metadata WHERE ws_id = 2858 ORDER BY id DESC LIMIT 1;
-- Should show: is_enabled=0, is_disabled_by_user=0
```

3. **Test bulk endpoint**:
```bash
# Disable all negatives
curl -X PATCH 'https://staging.bito.ai/qmm/api/v1/learning-rules/bulk-status' \
--header 'Content-Type: application/json' \
--data '{"workspace_id": 2858, "action": "disable", "scope": "all"}'

# Enable specific ones
curl -X PATCH 'https://staging.bito.ai/qmm/api/v1/learning-rules/bulk-status' \
--header 'Content-Type: application/json' \
--data '{"workspace_id": 2858, "action": "enable", "scope": "selected", "rule_ids": [X, Y, Z]}'
```

4. **Verify rule not applied in review** (disabled rule shouldn't produce suggestions)

---

## Troubleshooting

### Flag not resolving

**Symptom**: New rules still created enabled even though flag set to true

**Check**:
1. Verify flag is set at the right level:
```bash
curl 'https://preprod.bito.ai/config/get' -d '{"workspace_id": 978573, "config_key": "agent-config.public.learned_rules_disabled_by_default"}'
```

2. Check agent-level override:
```sql
SELECT JSON_EXTRACT(config_value, '$.learned_rules_disabled_by_default') 
FROM agent_mgmt.agent_config_instances 
WHERE workspace_id = 978573;
```

3. Look for resolution logs:
```
grep "resolved learned_rules_disabled_by_default" /var/log/agent-exec-mgmt.log
```

### Auto-enable still happening

**Symptom**: Disabled rule auto-enabled after intensity increased

**Check**: 
- Flag should gate the auto-enable logic
- Review logs for `Auto-enable gated: flag is ON`

### Bulk endpoint returning 400

**Symptom**: `PATCH /bulk-status` returns validation error

**Common issues**:
- Missing `workspace_id` or it's <= 0
- `scope=all` but `rule_ids` provided (should be empty)
- `scope=selected` but `rule_ids` is empty

**Example of correct request**:
```json
{
  "workspace_id": 978573,
  "action": "enable",
  "scope": "selected",
  "rule_ids": [489, 490, 491]
}
```

---

## Monitoring

### Key Metrics to Track

After enabling the feature, monitor:

1. **Flag resolution rate**: How many learning events are processing the flag correctly
2. **System-disabled vs User-disabled ratio**: Track `is_disabled_by_user` distribution
3. **Review conversion**: How many disabled rules are enabled after review vs. deleted
4. **Application impact**: Verify disabled rules don't produce suggestions

### Queries

```sql
-- Flag resolution (should show new rules with is_disabled_by_user=0)
SELECT 
  DATE(created_at) as date,
  COUNT(*) as new_rules,
  SUM(CASE WHEN is_disabled_by_user=0 AND is_enabled=0 THEN 1 ELSE 0 END) as system_disabled,
  SUM(CASE WHEN is_enabled=1 THEN 1 ELSE 0 END) as enabled
FROM cra_rule_metadata
WHERE ws_id = 978573
GROUP BY DATE(created_at)
ORDER BY date DESC;

-- Bulk endpoint usage
SELECT 
  COUNT(*) as bulk_operations,
  SUM(affected_count) as total_rules_affected
FROM bulk_status_audit_log
WHERE workspace_id = 978573
AND created_at > DATE_SUB(NOW(), INTERVAL 7 DAY);
```

---

## Rollback

If issues arise, rollback is simple:

```bash
# Reset to OFF (today's behavior)
curl --location 'https://preprod.bito.ai/config/set' \
--header 'Authorization: <YOUR_TOKEN>' \
--header 'Content-Type: application/json' \
--data '{
  "workspace_id": 978573,
  "config_key": "agent-config.public.learned_rules_disabled_by_default",
  "config_value": "false",
  "module": "agent-config",
  "data_type": "BOOLEAN"
}'

# Clear agent-level overrides
UPDATE agent_mgmt.agent_config_instances
SET config_value = JSON_REMOVE(config_value, '$.learned_rules_disabled_by_default')
WHERE workspace_id = 978573;
```

**Result**: All new rules revert to enabled (today's behavior), existing rules unchanged.
