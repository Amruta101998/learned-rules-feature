# Operational Runbook: Learned Rules Disable-by-Default

Quick reference for on-call engineers troubleshooting BITO-13843 feature issues.

## Quick Diagnostics

### Is the feature working?

```bash
# Step 1: Check flag is set
curl https://preprod.bito.ai/config/get \
  -H "Authorization: $TOKEN" \
  -d '{"workspace_id":978573,"config_key":"agent-config.public.learned_rules_disabled_by_default"}'

# Should return: "true" or "false" (not null/missing)

# Step 2: Create test learning event and check rule state
# Trigger emoji reaction in a review, then:
mysql> SELECT id, is_enabled, is_disabled_by_user FROM cra_rule_metadata 
        WHERE ws_id=978573 ORDER BY id DESC LIMIT 1;

# Step 3: Check logs for flag resolution
kubectl logs -f deployment/agent-exec-mgmt -n production | \
  grep "resolved learned_rules_disabled_by_default" | head -5
```

---

## Common Issues & Solutions

### Issue 1: Flag Not Resolving (Returns NULL)

**Symptom**: Feature flag shows as "null" or missing in config

**Root Cause**: Flag never set or configuration service error

**Fix**:
```bash
# Check if workspace exists
curl https://preprod.bito.ai/config/get \
  -d '{"workspace_id":978573,"config_key":"agent-config.public.learned_rules_disabled_by_default"}'

# If missing, set it
curl https://preprod.bito.ai/config/set \
  -d '{
    "workspace_id": 978573,
    "config_key": "agent-config.public.learned_rules_disabled_by_default",
    "config_value": "false",
    "module": "agent-config",
    "data_type": "BOOLEAN"
  }'

# Verify it's set
curl https://preprod.bito.ai/config/get \
  -d '{"workspace_id":978573,"config_key":"agent-config.public.learned_rules_disabled_by_default"}'
```

**Prevention**: Add config monitoring to verify all required flags are set on deployment.

---

### Issue 2: Rules Still Created Enabled (When Flag Should Be ON)

**Symptom**: Flag set to true, but new rules still have is_enabled=1

**Root Cause**: Flag not propagating to learning services

**Diagnosis**:
```bash
# Check QMM logs for flag value
kubectl logs deployment/quality-measurement-manager -n production | \
  grep "SaveRule: learned_rules_disabled_by_default" | tail -5

# Check adaptive-learning logs
kubectl logs deployment/adaptive-learning -n production | \
  grep "disableNewNegatives" | tail -5

# Check agent-exec-mgmt resolution
kubectl logs deployment/agent-exec-mgmt -n production | \
  grep "resolved" | tail -5
```

**Fix**:
```bash
# Step 1: Verify flag in config service
curl https://preprod.bito.ai/config/get \
  -d '{"workspace_id":978573,"config_key":"agent-config.public.learned_rules_disabled_by_default"}'

# Step 2: Restart learning services to pick up flag
kubectl rollout restart deployment/quality-measurement-manager -n production
kubectl rollout restart deployment/adaptive-learning -n production

# Step 3: Trigger new learning event and verify
# Create new emoji reaction, check rule state

# Step 4: If still not working, check for agent-level override
mysql> SELECT JSON_EXTRACT(config_value, '$.learned_rules_disabled_by_default') 
        FROM agent_mgmt.agent_config_instances 
        WHERE workspace_id = 978573;
# If agent override is set to false, it overrides workspace setting
```

---

### Issue 3: Bulk Endpoint Returns 400 Error

**Symptom**: `PATCH /bulk-status` returns 400 validation error

**Common Causes**:
```bash
# Cause 1: Missing workspace_id
curl -X PATCH https://preprod.bito.ai/qmm/api/v1/learning-rules/bulk-status \
  -d '{"action":"disable","scope":"all"}'
# Fix: Add workspace_id

# Cause 2: Invalid action
curl -X PATCH https://preprod.bito.ai/qmm/api/v1/learning-rules/bulk-status \
  -d '{"workspace_id":978573,"action":"invalid","scope":"all"}'
# Fix: Use "enable" or "disable"

# Cause 3: Mismatched scope and rule_ids
curl -X PATCH https://preprod.bito.ai/qmm/api/v1/learning-rules/bulk-status \
  -d '{"workspace_id":978573,"action":"disable","scope":"all","rule_ids":[1,2,3]}'
# Fix: scope=all should have empty rule_ids

# Cause 4: scope=selected but empty rule_ids
curl -X PATCH https://preprod.bito.ai/qmm/api/v1/learning-rules/bulk-status \
  -d '{"workspace_id":978573,"action":"disable","scope":"selected","rule_ids":[]}'
# Fix: Provide rule_ids for scope=selected
```

**Correct Request Format**:
```bash
# All negative rules
curl -X PATCH https://preprod.bito.ai/qmm/api/v1/learning-rules/bulk-status \
  -d '{"workspace_id":978573,"action":"disable","scope":"all"}'

# Selected rules
curl -X PATCH https://preprod.bito.ai/qmm/api/v1/learning-rules/bulk-status \
  -d '{"workspace_id":978573,"action":"enable","scope":"selected","rule_ids":[489,490]}'
```

---

### Issue 4: Disabled Rules Still Applying in Reviews

**Symptom**: Rules with is_enabled=0 produce suggestions in reviews

**Root Cause**: CRA not filtering by is_enabled

**Fix**:
```bash
# Step 1: Verify rule is actually disabled
mysql> SELECT id, is_enabled FROM cra_rule_metadata WHERE id=489;
# Should show: is_enabled=0

# Step 2: Check CRA logs
kubectl logs deployment/automation-platform -n production | \
  grep "ApplyLearningToFeedbacks" | tail -10

# Step 3: Restart CRA to ensure latest code
kubectl rollout restart deployment/automation-platform -n production

# Step 4: Manually verify rule not applied
# Create new review that would match the rule
# Confirm no suggestion appears
```

**Database Verification**:
```sql
-- Check which rules are being queried by CRA
SELECT id, is_enabled, is_disabled_by_user 
FROM cra_rule_metadata 
WHERE ws_id=978573 AND is_enabled=1 
LIMIT 10;

-- Should NOT include rules with is_enabled=0
```

---

### Issue 5: Auto-Enable Running When Flag is ON

**Symptom**: Disabled rule (flag ON) auto-enables after intensity threshold

**Root Cause**: Auto-enable gating not working in v0

**Fix**:
```bash
# Step 1: Check flag value
curl https://preprod.bito.ai/config/get \
  -d '{"workspace_id":978573,"config_key":"agent-config.public.learned_rules_disabled_by_default"}'
# Should be: true

# Step 2: Check QMM logs for auto-enable gating
kubectl logs deployment/quality-measurement-manager -n production | \
  grep "Auto-enable gated" | tail -5

# Step 3: If no gating logs, restart QMM
kubectl rollout restart deployment/quality-measurement-manager -n production

# Step 4: Trigger multiple learning events to increase intensity
# Watch logs for "Auto-enable gated: flag is ON"
```

**Code Review**:
```go
// Verify this code is present in quality-measurement-manager/learning_service.go
if disabledByDefault {
  log.Printf("Auto-enable gated: flag is ON for ws=%d - skipping EnableRuleOnRuleIntensity")
  return nil
}
```

---

### Issue 6: Type Flip Misbehavior (Positive→Negative)

**Symptom**: After type flip, rule state doesn't match expected behavior

**Common Cases**:
```bash
# Case 1: Positive→Negative with flag ON should disable
# But rule stays enabled (IsDisabledByUser check failed)

# Check the rule's is_disabled_by_user flag
mysql> SELECT id, rule_type, is_enabled, is_disabled_by_user 
        FROM cra_rule_metadata WHERE id=RULE_ID;

# If is_disabled_by_user=1, rule won't be disabled (user override)
# This is correct behavior - user manually disabled takes precedence

# Case 2: Negative→Positive should always enable
# Check adaptive-learning logs
kubectl logs deployment/adaptive-learning -n production | \
  grep "Rule.*flipped.*positive" | tail -10
```

**Fix**:
```bash
# If user-disabled rule incorrectly blocking feature:
# Re-enable it and let the flip logic run again

mysql> UPDATE cra_rule_metadata 
        SET is_enabled=0, is_disabled_by_user=0 
        WHERE id=RULE_ID;

# Then trigger cluster update to re-run flip logic
# Monitor adaptive-learning logs for flip handling
```

---

### Issue 7: Bulk Endpoint Slow (10,000+ Rules)

**Symptom**: `PATCH /bulk-status` takes > 10 seconds

**Root Cause**: Large workspace with many rules, need optimization

**Temporary Workaround**:
```bash
# Use scope=selected with batches instead of scope=all

# Get all negative rule IDs
mysql> SELECT CONCAT('[', GROUP_CONCAT(id), ']') 
        FROM cra_learned_rules 
        WHERE ws_id=978573 AND (version=0 OR rule_type='negative');

# Disable in batches of 1000
for i in {0..9}; do
  START=$((i * 1000))
  END=$(((i + 1) * 1000))
  BATCH_IDS=$(mysql -se "SELECT CONCAT('[', GROUP_CONCAT(id), ']') 
              FROM cra_learned_rules 
              WHERE ws_id=978573 AND (version=0 OR rule_type='negative') 
              LIMIT $START, 1000;")
  
  curl -X PATCH https://preprod.bito.ai/qmm/api/v1/learning-rules/bulk-status \
    -d '{"workspace_id":978573,"action":"disable","scope":"selected","rule_ids":'$BATCH_IDS'}'
  
  sleep 1  # Rate limiting
done
```

**Long-term Fix**:
- Optimize bulk endpoint query (add indexes)
- Consider pagination for scope=all
- Implement async processing for very large operations

---

### Issue 8: Configuration Service Down

**Symptom**: Flag resolution fails, learning events not processed

**Impact**: All new learning events will use default behavior (enabled)

**Immediate Fix**:
```bash
# Step 1: Check config service status
kubectl get pods -n production | grep config

# Step 2: If unhealthy, restart
kubectl rollout restart deployment/config-service -n production

# Step 3: Monitor for recovery
kubectl rollout status deployment/config-service -n production

# Step 4: Check learning service logs
# Should resume logging "resolved learned_rules_disabled_by_default"
```

**Workaround** (temporary):
- Set flag value in agent-exec-mgmt config directly
- Requires restart but allows services to work offline

---

## Monitoring Dashboard

### Create Grafana Dashboard with:

```
Metrics:
- learned_rules_flag_resolution_rate (should be > 99.9%)
- learned_rules_disabled_count (track disabled rules)
- learned_rules_enabled_count
- bulk_endpoint_latency_p95 (should be < 2s)
- learning_event_latency_p95 (should be < 500ms)
- auto_enable_gated_count (should be > 0 if flag ON)

Logs:
- [agent-exec-mgmt] resolved learned_rules_disabled_by_default
- [qmm] SaveRule: learned_rules_disabled_by_default
- [adaptive-learning] disableNewNegatives flag
- [automation-platform] ApplyLearningToFeedbacks

Alerts:
- Flag resolution errors > 1%
- Bulk endpoint latency > 2s
- Learning event latency > 500ms
- No flag resolution logs in 5 minutes
```

---

## Escalation Path

| Issue | Severity | First Response | Escalate To |
| --- | --- | --- | --- |
| Flag not set | P3 | Set flag via curl | Product Manager |
| Rules not disabled | P2 | Check logs, restart services | Engineering Lead |
| Bulk endpoint 400 | P3 | Validate request format | API Owner |
| Disabled rules applying | P1 | Restart CRA, check code | CRA Team Lead |
| Auto-enable not gated | P1 | Restart QMM, verify code | QMM Team Lead |
| Performance degradation | P2 | Check metrics, batch operations | DevOps + Service Owner |
| Configuration service down | P1 | Restart, switch to offline mode | Platform Team |

---

## Quick Commands Reference

```bash
# Check flag value
curl https://preprod.bito.ai/config/get -d '{"workspace_id":978573,"config_key":"agent-config.public.learned_rules_disabled_by_default"}' | jq .

# Set flag ON
curl https://preprod.bito.ai/config/set -d '{"workspace_id":978573,"config_key":"agent-config.public.learned_rules_disabled_by_default","config_value":"true","module":"agent-config","data_type":"BOOLEAN"}'

# Bulk disable all
curl -X PATCH https://preprod.bito.ai/qmm/api/v1/learning-rules/bulk-status -d '{"workspace_id":978573,"action":"disable","scope":"all"}'

# Check QMM logs
kubectl logs -f deployment/quality-measurement-manager -n production | grep -i "learned_rules"

# Restart learning services
kubectl rollout restart deployment/quality-measurement-manager deployment/adaptive-learning -n production

# Database check
mysql -e "SELECT id, is_enabled, is_disabled_by_user FROM cra_rule_metadata WHERE ws_id=978573 ORDER BY id DESC LIMIT 5;"
```

---

## Contact & Resources

- **#bito-learned-rules** Slack channel
- **Owner**: [Engineering Lead Name]
- **On-Call**: [Rotation Schedule]
- **Documentation**: [Links to guides]
- **Jira Ticket**: BITO-13843
