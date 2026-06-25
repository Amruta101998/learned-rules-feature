# BITO-13843: Learning Rules Disable-by-Default Feature

Complete implementation of the "Give workspaces control over learned rule activation and application" feature.

## Overview

This repository contains all code changes needed to implement the `learned_rules_disabled_by_default` flag across Bito's learned rule system. The feature allows workspaces to control whether newly learned negative rules are automatically applied or require manual review before activation.

## Repository Structure

```
learned-rules-feature/
├── README.md                              # This file
├── IMPLEMENTATION_GUIDE.md                # Detailed implementation guide
├── agent-def-config/
│   └── agent_config.go                    # Flag definition & resolution logic
├── agent-exec-mgmt/
│   └── learning_processor.go              # Flag resolution & event construction
├── quality-measurement-manager/
│   ├── learning_service.go                # v0 QMM learning implementation
│   ├── learning_service_test.go           # v0 unit tests
│   ├── bulk_status_handler.go             # Internal bulk enable/disable logic
│   └── api_handler.go                     # PATCH /bulk-status endpoint
├── adaptive-learning/
│   ├── embedding_service.go               # v1 adaptive learning implementation
│   └── embedding_service_test.go          # v1 unit tests
├── automation-platform/
│   └── learning_filter.go                 # CRA learning application filter
├── migrations/
│   └── 001_add_is_disabled_by_user.sql    # DB migration for schema
└── [This is a git repository for tracking changes]
```

## Quick Start

### What This Implements

✅ **Tri-state workspace/agent-level configuration flag**
- Defaults to false (today's behavior)
- Agent value takes precedence over workspace value
- Can be set via curl or direct DB update

✅ **v0 Learning (QMM) — Reactions & "avoid" comments**
- New negative rules created disabled when flag ON
- v0 auto-enable gated by flag (no silent re-activation)
- Intensity still increments; flag OFF resumes auto-enable

✅ **v1 Learning (Adaptive) — Cluster-based**
- New negative rules created disabled when flag ON
- Positive rules always enabled (unaffected)
- Type flips handled: positive→negative obeys flag; negative→positive always enables
- User-disabled state preserved across flips

✅ **CRA Application Filter**
- Both v0 and v1 queries filter `is_enabled=1`
- Disabled rules (by default or user) never apply to reviews

✅ **Internal Bulk Status Endpoint**
- `PATCH /qmm/api/v1/learning-rules/bulk-status`
- Negatives-only, supports `scope=all` or `scope=selected`
- Sets `is_disabled_by_user` flag to protect from auto-enable

✅ **Database Schema**
- New column: `cra_rule_metadata.is_disabled_by_user` (tri-state tracking)
- No data loss; existing rules unaffected

### Key Files to Review

1. **Feature Overview**: [IMPLEMENTATION_GUIDE.md](IMPLEMENTATION_GUIDE.md)
2. **v0 Logic**: [quality-measurement-manager/learning_service.go](quality-measurement-manager/learning_service.go)
3. **v1 Logic**: [adaptive-learning/embedding_service.go](adaptive-learning/embedding_service.go)
4. **Bulk Endpoint**: [quality-measurement-manager/bulk_status_handler.go](quality-measurement-manager/bulk_status_handler.go)
5. **Tests**: `*_test.go` files for unit test examples

### Setting the Flag

**Workspace Level**:
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

**Agent Level**:
```sql
UPDATE agent_mgmt.agent_config_instances
SET config_value = JSON_SET(config_value, '$.learned_rules_disabled_by_default', true)
WHERE workspace_id = 978573 AND id = '<agent-instance-id>';
```

### Using the Bulk Endpoint

```bash
# Disable all negative rules in workspace
curl -X PATCH https://preprod.bito.ai/qmm/api/v1/learning-rules/bulk-status \
  -H 'Content-Type: application/json' \
  -d '{"workspace_id": 978573, "action": "disable", "scope": "all"}'

# Enable selected rules
curl -X PATCH https://preprod.bito.ai/qmm/api/v1/learning-rules/bulk-status \
  -H 'Content-Type: application/json' \
  -d '{"workspace_id": 978573, "action": "enable", "scope": "selected", "rule_ids": [489, 490]}'
```

## How It Works

### Flag Resolution (Agent > Workspace > Default)

```
resolved = agent_value (if set)
         | workspace_value (if set, and agent not set)
         | false (default, if neither set)
```

### New Negative Rule Flow

1. **Learning event triggered** (emoji reaction, "avoid" comment, or cluster update)
2. **agent-exec-mgmt** resolves flag from agent + workspace config
3. **Pass into learning event** with resolved value
4. **v0/v1 service** checks flag and creates rule:
   - Flag OFF → `is_enabled=1` (today's behavior)
   - Flag ON → `is_enabled=0, is_disabled_by_user=0` (system-disabled)
5. **CRA applies rules** by filtering `is_enabled=1` (disabled never apply)
6. **Dashboard UI** shows disabled rules in Learned Rules section for review

### Edge Case: v0 Auto-Enable Gating

When a rule recurs and intensity crosses threshold:

| Flag | is_disabled_by_user | Result |
| --- | --- | --- |
| OFF | 0 | Auto-enable (existing behavior) |
| OFF | 1 | Stays disabled (user protection) |
| ON | 0 | **Stays disabled** (flag master switch) |
| ON | 1 | Stays disabled (user protection) |

**Benefit**: Intensity increments while disabled. When flag OFF later, rule resumes auto-enable on next recurrence (no backfill).

### Type Flip: Adaptive Learning

When a rule's sentiment shifts and type changes:

| From | To | Flag ON | Flag OFF |
| --- | --- | --- | --- |
| positive | negative | Disable (apply flag) | Enable (default) |
| negative | positive | **Always enable** (positives by default) | **Always enable** |
| Same type | — | No change | No change |

**Protection**: User-disabled rules (`is_disabled_by_user=1`) never auto-enabled by flips.

## Testing

### Unit Tests

All tests pass covering:
- Flag resolution and precedence
- v0: Rule creation, auto-enable gating
- v1: Rule creation, type flips, positive rules
- Bulk endpoint: Request validation, scope logic, isolation

Run with:
```bash
# v0 tests
go test ./quality-measurement-manager/...

# v1 tests
go test ./adaptive-learning/...
```

### Integration Test Scenarios

See [IMPLEMENTATION_GUIDE.md § Testing Checklist](IMPLEMENTATION_GUIDE.md#testing-checklist) for full coverage (15+ test cases):

1. **Default behavior** — Flag unset = today's behavior
2. **v0 & v1 creation** — New negatives created disabled when flag ON
3. **Application filter** — Disabled rules not applied in reviews
4. **Auto-enable gating** — Intensity increments but no auto-enable when flag ON
5. **Type flips** — Positive→negative flips respect flag; negative→positive always enable
6. **Bulk endpoint** — Correctly updates negatives-only, validates requests
7. **Backward compatibility** — Existing rules untouched, per-rule toggle still works

## Acceptance Criteria (from BITO-13843)

✅ **With the setting off, behavior is identical to today**
- Default flag OFF → new rules created enabled → applied as always
- Positive rules unaffected
- Existing rules unchanged

✅ **With it on, newly learned negative rules appear disabled**
- Flag ON → new rules created disabled (`is_enabled=0`)
- Shown in Learned Rules section
- Not applied in reviews until enabled

✅ **Existing rules retain their current state**
- Flag never modifies pre-existing rules
- Toggling flag doesn't affect prior learned rules

✅ **Positive rule behavior is unchanged in all cases**
- Positive rules always enabled
- Always apply in reviews (their application separately governed)
- No UI/UX changes

## Database Schema

### Migration

File: `migrations/001_add_is_disabled_by_user.sql`

Adds column to track manual disable vs. system default-disable:

```sql
ALTER TABLE cra_rule_metadata ADD COLUMN is_disabled_by_user TINYINT(1) NOT NULL DEFAULT 0;
```

### Rule States

| is_enabled | is_disabled_by_user | Status | Applies? |
| --- | --- | --- | --- |
| 1 | 0 | Enabled | ✅ Yes |
| 0 | 0 | System-disabled (by flag) | ❌ No |
| 0 | 1 | User-disabled (manual) | ❌ No |

## Rollout

1. Deploy to preprod → run full test suite
2. Deploy `agent-def-config` (no-op until flag set)
3. Deploy `agent-exec-mgmt` (resolves and passes flag)
4. Deploy `quality-measurement-manager` (v0 + bulk endpoint)
5. Deploy `adaptive-learning` (v1)
6. Deploy `automation-platform` (confirm is_enabled filter)
7. Enable flag for opt-in workspace (Mindtickle 978573)
8. Monitor logs for `learned_rules_disabled_by_default` flag resolution

## Logs to Watch

### agent-exec-mgmt
```
LearningProcessor: resolved learned_rules_disabled_by_default=true for ws=978573, agent=agent-123 (agent=true, ws=unset)
```

### quality-measurement-manager (v0)
```
QMM SaveRule: learned_rules_disabled_by_default=true, creating rule with isEnabled=false for ws=978573
Negative rule created as disabled (flag ON) for workspace 978573 - requires manual review before activation
Auto-enable gated: flag is ON for ws=978573 - skipping EnableRuleOnRuleIntensity for rule 489
```

### adaptive-learning (v1)
```
EmbeddingService: disableNewNegatives flag for workspace 978573 = true
New negative rule created with isEnabled=false for cluster cluster-123, workspace 978573
Rule type flip detected for cluster cluster-123: positive → negative
Rule 489 flipped positive→negative: setting isEnabled=false (disabledByDefault=true)
```

### Bulk endpoint
```
BulkStatusHandler: processing disable action, scope=selected, workspace=978573, rule_count=2
BulkStatusHandler: selected completed - workspace=978573, action=disable, affected=2
```

## Links

- **Jira Ticket**: https://bito.atlassian.net/browse/BITO-13843
- **Implementation Plan**: https://bito.atlassian.net/browse/BITO-13843?focusedCommentId=171274
- **Detailed Behavior**: https://bito.atlassian.net/browse/BITO-13843?focusedCommentId=171282
- **Bulk Endpoint Spec**: https://bito.atlassian.net/browse/BITO-13843?focusedCommentId=177515
- **Test Plan**: https://bito.atlassian.net/browse/BITO-13843?focusedCommentId=177776

## Support

For questions on implementation details, refer to:
1. The ticket's detailed comments (linked above)
2. [IMPLEMENTATION_GUIDE.md](IMPLEMENTATION_GUIDE.md) — full technical spec
3. Unit tests — live examples of expected behavior
4. Inline code comments — flagged key edge cases

---

**Status**: Complete implementation ready for review and deployment  
**Author**: Generated from BITO-13843  
**Date**: 2026-06-25
