# cURL Recipes for Learned Rules Feature

Quick reference for common operations using cURL commands.

## Authentication Token

Replace `<YOUR_TOKEN>` with your Bito API token:

```bash
export BITO_TOKEN="<YOUR_TOKEN>"
export BITO_ENV="preprod"  # or "staging", "prod"
export BASE_URL="https://${BITO_ENV}.bito.ai"
```

## Configuration Management

### Get Current Flag Value

```bash
curl --location "${BASE_URL}/config/get" \
--header "Authorization: ${BITO_TOKEN}" \
--header "Content-Type: application/json" \
--data '{
  "workspace_id": 978573,
  "config_key": "agent-config.public.learned_rules_disabled_by_default"
}'
```

### Set Flag to TRUE

```bash
curl --location "${BASE_URL}/config/set" \
--header "Authorization: ${BITO_TOKEN}" \
--header "Content-Type: application/json" \
--data '{
  "workspace_id": 978573,
  "config_key": "agent-config.public.learned_rules_disabled_by_default",
  "config_value": "true",
  "module": "agent-config",
  "data_type": "BOOLEAN"
}'
```

### Set Flag to FALSE

```bash
curl --location "${BASE_URL}/config/set" \
--header "Authorization: ${BITO_TOKEN}" \
--header "Content-Type: application/json" \
--data '{
  "workspace_id": 978573,
  "config_key": "agent-config.public.learned_rules_disabled_by_default",
  "config_value": "false",
  "module": "agent-config",
  "data_type": "BOOLEAN"
}'
```

### Unset Flag (Reset to Default)

```bash
curl --location "${BASE_URL}/config/delete" \
--header "Authorization: ${BITO_TOKEN}" \
--header "Content-Type: application/json" \
--data '{
  "workspace_id": 978573,
  "config_key": "agent-config.public.learned_rules_disabled_by_default",
  "module": "agent-config"
}'
```

---

## Bulk Status Endpoint

### Disable All Negative Rules

```bash
curl -X PATCH "${BASE_URL}/qmm/api/v1/learning-rules/bulk-status" \
--header "Content-Type: application/json" \
--data '{
  "workspace_id": 978573,
  "action": "disable",
  "scope": "all"
}'
```

### Enable All Negative Rules

```bash
curl -X PATCH "${BASE_URL}/qmm/api/v1/learning-rules/bulk-status" \
--header "Content-Type: application/json" \
--data '{
  "workspace_id": 978573,
  "action": "enable",
  "scope": "all"
}'
```

### Disable Selected Rules

```bash
curl -X PATCH "${BASE_URL}/qmm/api/v1/learning-rules/bulk-status" \
--header "Content-Type: application/json" \
--data '{
  "workspace_id": 978573,
  "action": "disable",
  "scope": "selected",
  "rule_ids": [489, 490, 491, 492, 493]
}'
```

### Enable Selected Rules

```bash
curl -X PATCH "${BASE_URL}/qmm/api/v1/learning-rules/bulk-status" \
--header "Content-Type: application/json" \
--data '{
  "workspace_id": 978573,
  "action": "enable",
  "scope": "selected",
  "rule_ids": [489, 490]
}'
```

---

## Batch Operations with jq

### Get Rule IDs to Disable (from DB, then via API)

```bash
# First, get IDs from database
RULE_IDS=$(mysql -h db.preprod.internal -u bito -p -e \
  "SELECT GROUP_CONCAT(id) FROM cra_learned_rules WHERE ws_id=978573 AND rule_type='negative' LIMIT 100;" | tail -1)

# Convert to JSON array
RULE_ARRAY=$(echo $RULE_IDS | jq -R 'split(",") | map(tonumber)')

# Use in bulk API call
curl -X PATCH "${BASE_URL}/qmm/api/v1/learning-rules/bulk-status" \
--header "Content-Type: application/json" \
--data @- <<EOF
{
  "workspace_id": 978573,
  "action": "disable",
  "scope": "selected",
  "rule_ids": $RULE_ARRAY
}
EOF
```

### Parse Bulk Endpoint Response

```bash
curl -X PATCH "${BASE_URL}/qmm/api/v1/learning-rules/bulk-status" \
--header "Content-Type: application/json" \
--data '{
  "workspace_id": 978573,
  "action": "disable",
  "scope": "all"
}' | jq '{
  status: .status.code,
  message: .status.message,
  affected: .data.affected_count,
  action: .data.action,
  workspace: .data.workspace_id
}'
```

---

## Testing & Validation

### Test Flag Resolution (Check Logs)

```bash
# SSH to agent-exec-mgmt pod and tail logs
kubectl logs -f deployment/agent-exec-mgmt -c agent-exec-mgmt | \
  grep "resolved learned_rules_disabled_by_default"
```

### Test QMM Rule Creation

```bash
# Monitor QMM service logs
kubectl logs -f deployment/quality-measurement-manager -c qmm | \
  grep -E "SaveRule|Negative rule created as disabled"
```

### Test Adaptive Learning

```bash
# Monitor adaptive-learning service logs
kubectl logs -f deployment/adaptive-learning -c adaptive-learning | \
  grep -E "disableNewNegatives|Rule type flip detected"
```

---

## Error Handling

### Invalid Request (Missing Workspace ID)

```bash
curl -X PATCH "${BASE_URL}/qmm/api/v1/learning-rules/bulk-status" \
--header "Content-Type: application/json" \
--data '{
  "action": "disable",
  "scope": "all"
}' 2>&1 | jq .
```

**Response** (400):
```json
{
  "status": {
    "code": "400",
    "message": "Missing or invalid workspace_id"
  }
}
```

### Mismatched Scope and Rule IDs

```bash
# Error: scope=all but rule_ids provided
curl -X PATCH "${BASE_URL}/qmm/api/v1/learning-rules/bulk-status" \
--header "Content-Type: application/json" \
--data '{
  "workspace_id": 978573,
  "action": "disable",
  "scope": "all",
  "rule_ids": [489, 490]
}'
```

**Response** (400):
```json
{
  "status": {
    "code": "400",
    "message": "scope=all should not include rule_ids"
  }
}
```

---

## One-Liners

### Quick Status Check

```bash
curl -s "${BASE_URL}/config/get" -H "Authorization: ${BITO_TOKEN}" -d '{"workspace_id":978573,"config_key":"agent-config.public.learned_rules_disabled_by_default"}' | jq .
```

### Disable All & Count

```bash
RESULT=$(curl -s -X PATCH "${BASE_URL}/qmm/api/v1/learning-rules/bulk-status" -d '{"workspace_id":978573,"action":"disable","scope":"all"}'); echo "Disabled $(echo $RESULT | jq .data.affected_count) rules"
```

### Enable & Verify

```bash
curl -s -X PATCH "${BASE_URL}/qmm/api/v1/learning-rules/bulk-status" -d '{"workspace_id":978573,"action":"enable","scope":"selected","rule_ids":[489]}' | jq '.status | {code, message}'
```

---

## Environment Variables for Automation

### Bash Script Template

```bash
#!/bin/bash
set -e

WORKSPACE_ID=${1:-978573}
ACTION=${2:-disable}
SCOPE=${3:-all}
BITO_ENV=${BITO_ENV:-preprod}
BASE_URL="https://${BITO_ENV}.bito.ai"

echo "🔧 Bulk Update Learned Rules"
echo "   Workspace: $WORKSPACE_ID"
echo "   Action: $ACTION"
echo "   Scope: $SCOPE"
echo ""

RESPONSE=$(curl -s -X PATCH "${BASE_URL}/qmm/api/v1/learning-rules/bulk-status" \
  --header "Content-Type: application/json" \
  --data "{
    \"workspace_id\": $WORKSPACE_ID,
    \"action\": \"$ACTION\",
    \"scope\": \"$SCOPE\"
  }")

CODE=$(echo $RESPONSE | jq -r '.status.code')
MESSAGE=$(echo $RESPONSE | jq -r '.status.message')
AFFECTED=$(echo $RESPONSE | jq -r '.data.affected_count')

if [ "$CODE" = "1000" ]; then
  echo "✅ $MESSAGE"
  echo "   Affected: $AFFECTED rules"
else
  echo "❌ Error: $MESSAGE"
  exit 1
fi
```

Usage:
```bash
./bulk_update.sh 978573 disable all
./bulk_update.sh 978573 enable selected
```

---

## Integration Testing

### Full Workflow Test

```bash
#!/bin/bash

WS_ID=2858  # staging workspace

echo "1. Set flag ON"
curl -s "${BASE_URL}/config/set" -H "Authorization: ${BITO_TOKEN}" \
  -d '{"workspace_id":'$WS_ID',"config_key":"agent-config.public.learned_rules_disabled_by_default","config_value":"true","module":"agent-config","data_type":"BOOLEAN"}' | jq .

echo -e "\n2. Trigger learning event (manual - emoji reaction on staging PR)"
read -p "Press enter after triggering learning event..."

echo -e "\n3. Verify rule created disabled"
mysql -h db.staging.internal -u bito -p -e \
  "SELECT id, is_enabled, is_disabled_by_user FROM cra_rule_metadata WHERE ws_id=$WS_ID ORDER BY id DESC LIMIT 1;"

echo -e "\n4. Test bulk disable all"
curl -s -X PATCH "${BASE_URL}/qmm/api/v1/learning-rules/bulk-status" \
  -d '{"workspace_id":'$WS_ID',"action":"disable","scope":"all"}' | jq '.data'

echo -e "\n5. Test bulk enable selected"
curl -s -X PATCH "${BASE_URL}/qmm/api/v1/learning-rules/bulk-status" \
  -d '{"workspace_id":'$WS_ID',"action":"enable","scope":"selected","rule_ids":[1,2,3]}' | jq '.data'

echo -e "\n✅ All tests completed!"
```

---

## Performance Considerations

### Bulk Operations on Large Workspaces

For workspaces with 10k+ rules, bulk operations may take time:

```bash
# Check affected_count in response - if 0, all rules already in target state
AFFECTED=$(curl -s -X PATCH "${BASE_URL}/qmm/api/v1/learning-rules/bulk-status" \
  -d '{"workspace_id":978573,"action":"disable","scope":"all"}' | jq .data.affected_count)

if [ "$AFFECTED" -eq 0 ]; then
  echo "All rules already disabled (idempotent operation)"
else
  echo "Updated $AFFECTED rules"
fi
```

### Rate Limiting

If hitting rate limits:

```bash
# Implement exponential backoff
for attempt in {1..5}; do
  RESPONSE=$(curl -s -X PATCH "${BASE_URL}/qmm/api/v1/learning-rules/bulk-status" \
    -d '{"workspace_id":978573,"action":"disable","scope":"all"}')
  
  STATUS=$(echo $RESPONSE | jq -r '.status.code')
  
  if [ "$STATUS" = "1000" ]; then
    echo "✅ Success"
    break
  elif [ "$STATUS" = "429" ]; then
    WAIT=$((2 ** attempt))
    echo "⏳ Rate limited, waiting ${WAIT}s..."
    sleep $WAIT
  else
    echo "❌ Error: $STATUS"
    break
  fi
done
```
