# Deployment Guide: Learned Rules Disable-by-Default Feature

Complete guide for deploying BITO-13843 feature across environments.

## Table of Contents

1. [Pre-Deployment Checklist](#pre-deployment-checklist)
2. [Deployment Order](#deployment-order)
3. [Environment-Specific Steps](#environment-specific-steps)
4. [Monitoring & Validation](#monitoring--validation)
5. [Rollback Procedures](#rollback-procedures)
6. [Post-Deployment Verification](#post-deployment-verification)

---

## Pre-Deployment Checklist

### Code Review & Testing
- [ ] All pull requests approved and merged to main
- [ ] Unit tests pass: `go test ./...`
- [ ] Integration tests pass: `go test ./integration_tests/...`
- [ ] Load tests run without errors: `go test -run=TestLoad ./integration_tests/`
- [ ] Code review completed for all service changes
- [ ] Security review completed (no SQL injection, proper auth)

### Database
- [ ] Migration reviewed and tested on staging
- [ ] Backup taken before migration (both staging & production)
- [ ] Rollback procedure documented and tested
- [ ] Schema changes compatible with current code

### Configuration
- [ ] Default flag value (false) correct for all environments
- [ ] Workspace/agent configuration structure defined
- [ ] Feature flags added to agent-def-config repo
- [ ] Configuration rollback procedures documented

### Documentation
- [ ] README.md and IMPLEMENTATION_GUIDE.md complete
- [ ] CONFIG_EXAMPLES.md and CURL_RECIPES.md reviewed
- [ ] Runbooks prepared for on-call team
- [ ] Test plan documented (IMPLEMENTATION_GUIDE.md § Testing Checklist)

### Team Readiness
- [ ] All service teams notified of changes
- [ ] On-call team trained on monitoring & troubleshooting
- [ ] Customer (Mindtickle) notified of feature availability
- [ ] Support team briefed on common issues

---

## Deployment Order

**Critical**: Deploy in this specific order to avoid issues.

### Phase 1: Infrastructure (Day 1)

#### Step 1.1: Deploy Database Migration

**Service**: Database Team / DevOps

```bash
# Staging environment
./scripts/migrate-staging.sh migrations/001_add_is_disabled_by_user.sql

# Production environment (after staging validation)
./scripts/migrate-prod.sh migrations/001_add_is_disabled_by_user.sql
```

**Validation**:
```sql
-- Verify column added
DESCRIBE cra_rule_metadata;
-- Should show: is_disabled_by_user TINYINT(1)

-- Verify data integrity
SELECT COUNT(*) FROM cra_rule_metadata WHERE is_disabled_by_user IS NOT NULL;
```

**Rollback** (if needed):
```sql
ALTER TABLE cra_rule_metadata DROP COLUMN is_disabled_by_user;
```

---

#### Step 1.2: Deploy agent-def-config

**Repository**: agent-def-config  
**Service**: Config Management  
**Deployment**: Kubernetes rolling update

```bash
# Build and push image
docker build -t bito/agent-def-config:v2.5.0 .
docker push bito/agent-def-config:v2.5.0

# Deploy to staging
kubectl set image deployment/agent-def-config \
  agent-def-config=bito/agent-def-config:v2.5.0 \
  -n staging

# Verify deployment
kubectl rollout status deployment/agent-def-config -n staging

# Deploy to production
kubectl set image deployment/agent-def-config \
  agent-def-config=bito/agent-def-config:v2.5.0 \
  -n production

# Verify production deployment
kubectl rollout status deployment/agent-def-config -n production
```

**Validation**:
```bash
# Check logs for any errors
kubectl logs -f deployment/agent-def-config -n staging | grep -i error

# Verify config APIs are responsive
curl https://staging.bito.ai/config/health
```

---

### Phase 2: Core Services (Day 2)

#### Step 2.1: Deploy agent-exec-mgmt

**Repository**: agent-exec-mgmt  
**Service**: Learning Event Processor

```bash
# Build
docker build -t bito/agent-exec-mgmt:v3.2.0 .
docker push bito/agent-exec-mgmt:v3.2.0

# Deploy staging
kubectl set image deployment/agent-exec-mgmt \
  agent-exec-mgmt=bito/agent-exec-mgmt:v3.2.0 \
  -n staging

# Verify
kubectl rollout status deployment/agent-exec-mgmt -n staging

# Deploy production
kubectl set image deployment/agent-exec-mgmt \
  agent-exec-mgmt=bito/agent-exec-mgmt:v3.2.0 \
  -n production
```

**Validation** (check logs):
```bash
# Monitor for flag resolution logs
kubectl logs -f deployment/agent-exec-mgmt -n staging | \
  grep "resolved learned_rules_disabled_by_default"

# Should show: "resolved learned_rules_disabled_by_default=false" (default)
```

---

#### Step 2.2: Deploy quality-measurement-manager (v0)

**Repository**: quality-measurement-manager  
**Service**: QMM Learning

```bash
# Build
docker build -t bito/qmm:v1.8.0 .
docker push bito/qmm:v1.8.0

# Deploy staging
kubectl set image deployment/quality-measurement-manager \
  qmm=bito/qmm:v1.8.0 \
  -n staging

# Verify
kubectl rollout status deployment/quality-measurement-manager -n staging
kubectl port-forward service/qmm 8080:80 -n staging &

# Test bulk endpoint
curl -X PATCH http://localhost:8080/qmm/api/v1/learning-rules/bulk-status \
  -H 'Content-Type: application/json' \
  -d '{"workspace_id":2858,"action":"disable","scope":"all"}'

# Deploy production
kubectl set image deployment/quality-measurement-manager \
  qmm=bito/qmm:v1.8.0 \
  -n production

# Verify production
kubectl rollout status deployment/quality-measurement-manager -n production
```

**Validation** (check logs):
```bash
kubectl logs -f deployment/quality-measurement-manager -n staging | \
  grep -E "SaveRule|Negative rule created|Auto-enable gated"
```

---

#### Step 2.3: Deploy adaptive-learning (v1)

**Repository**: adaptive-learning  
**Service**: Adaptive Learning

```bash
# Build
docker build -t bito/adaptive-learning:v2.1.0 .
docker push bito/adaptive-learning:v2.1.0

# Deploy staging
kubectl set image deployment/adaptive-learning \
  adaptive-learning=bito/adaptive-learning:v2.1.0 \
  -n staging

# Verify
kubectl rollout status deployment/adaptive-learning -n staging

# Deploy production
kubectl set image deployment/adaptive-learning \
  adaptive-learning=bito/adaptive-learning:v2.1.0 \
  -n production
```

**Validation** (check logs):
```bash
kubectl logs -f deployment/adaptive-learning -n staging | \
  grep -E "disableNewNegatives|Rule type flip"
```

---

#### Step 2.4: Deploy automation-platform (CRA)

**Repository**: automation-platform  
**Service**: Code Review Automation

```bash
# Build
docker build -t bito/automation-platform:v4.5.0 .
docker push bito/automation-platform:v4.5.0

# Deploy staging
kubectl set image deployment/automation-platform \
  automation-platform=bito/automation-platform:v4.5.0 \
  -n staging

# Verify
kubectl rollout status deployment/automation-platform -n staging

# Deploy production
kubectl set image deployment/automation-platform \
  automation-platform=bito/automation-platform:v4.5.0 \
  -n production
```

**Validation**:
```bash
# Verify CRA applies only enabled rules
kubectl logs -f deployment/automation-platform -n staging | \
  grep -E "ApplyLearningToFeedbacks|is_enabled=1"
```

---

### Phase 3: Feature Activation (Day 3)

#### Step 3.1: Enable on Staging Workspace (Smoke Test)

**Workspace**: 2858 (staging test workspace)

```bash
# Set flag ON for staging workspace
curl --location 'https://staging.bito.ai/config/set' \
--header 'Authorization: ${STAGING_TOKEN}' \
--header 'Content-Type: application/json' \
--data '{
  "workspace_id": 2858,
  "config_key": "agent-config.public.learned_rules_disabled_by_default",
  "config_value": "true",
  "module": "agent-config",
  "data_type": "BOOLEAN"
}'

# Verify flag is set
curl --location 'https://staging.bito.ai/config/get' \
--header 'Authorization: ${STAGING_TOKEN}' \
--data '{"workspace_id":2858,"config_key":"agent-config.public.learned_rules_disabled_by_default"}'
```

**Run Full Test Suite** (reference: IMPLEMENTATION_GUIDE.md § Testing Checklist):
1. Create new learning event (emoji reaction or cluster)
2. Verify rule created disabled in Learned Rules section
3. Test bulk endpoint (disable all, enable selected)
4. Verify disabled rules don't apply in reviews
5. Toggle flag OFF and verify behavior reverts
6. Toggle flag ON and verify auto-enable is gated

---

#### Step 3.2: Enable for Mindtickle Workspace (Customer)

**Workspace**: 978573 (Mindtickle customer - prod)

```bash
# Set flag ON for Mindtickle workspace
curl --location 'https://preprod.bito.ai/config/set' \
--header 'Authorization: ${PROD_TOKEN}' \
--header 'Content-Type: application/json' \
--data '{
  "workspace_id": 978573,
  "config_key": "agent-config.public.learned_rules_disabled_by_default",
  "config_value": "true",
  "module": "agent-config",
  "data_type": "BOOLEAN"
}'

# Notify customer
# Subject: BITO-13843 Feature Enabled - Learned Rules Disable-by-Default
# Body: Mindtickle workspace 978573 now has the new "learned rules disable-by-default"
#       feature enabled. New negative rules will require review before activation.
#       Use the Learned Rules dashboard to enable reviewed rules.
```

---

## Environment-Specific Steps

### Staging Environment

```bash
# Full deployment checklist for staging
export ENV=staging
export NAMESPACE=staging

# 1. Run all tests
go test ./... -v

# 2. Deploy all services in order
./deploy.sh $ENV

# 3. Run integration tests
go test ./integration_tests/... -v

# 4. Run load tests
go test -run=TestLoad ./integration_tests/ -timeout=120s

# 5. Enable feature flag
curl "${BASE_URL}/config/set" -d '{"workspace_id":2858,"config_key":"agent-config.public.learned_rules_disabled_by_default","config_value":"true"}'

# 6. Run full test plan from IMPLEMENTATION_GUIDE.md
./scripts/run-staging-tests.sh
```

### Production Environment

```bash
# Production deployment checklist
export ENV=production
export NAMESPACE=production

# 1. Verify all services running in staging
kubectl get deployments -n staging | grep -E "agent-def-config|agent-exec-mgmt|qmm|adaptive-learning|automation-platform"

# 2. Approval required before production deployment
# - Get sign-off from engineering lead
# - Get sign-off from product manager
# - Create change ticket in incident management system

# 3. Deploy during maintenance window (lowest traffic hours)
./deploy.sh $ENV

# 4. Monitor error rates for 1 hour post-deployment
./scripts/monitor-errors.sh $ENV 60

# 5. Enable feature flag for customer workspace
# - Only after all services verified healthy
# - Send customer notification with documentation link

curl "${BASE_URL}/config/set" -d '{"workspace_id":978573,"config_key":"agent-config.public.learned_rules_disabled_by_default","config_value":"true"}'
```

---

## Monitoring & Validation

### Key Metrics to Monitor

```bash
# During and after deployment, watch these metrics

# 1. Service health (all should be green)
kubectl get pods -n production | grep -E "agent-def-config|agent-exec-mgmt|qmm|adaptive-learning|automation-platform"

# 2. Error rates (should be < 0.1%)
curl https://metrics.internal/query?query=error_rate{service=~"qmm|adaptive-learning|automation-platform"}

# 3. Flag resolution success (should be 100%)
curl https://metrics.internal/query?query=flag_resolution_success{feature="learned_rules_disabled_by_default"}

# 4. Learning event processing latency (should be < 500ms p95)
curl https://metrics.internal/query?query=learning_event_latency_p95

# 5. Bulk endpoint response time (should be < 2s for 10k rules)
curl https://metrics.internal/query?query=bulk_endpoint_latency_p95
```

### Log Patterns to Verify

```bash
# Monitoring dashboard queries

# Flag resolution
kubectl logs -f deployment/agent-exec-mgmt -n production | \
  grep "resolved learned_rules_disabled_by_default" | \
  tail -20

# v0 rule creation
kubectl logs -f deployment/quality-measurement-manager -n production | \
  grep "QMM SaveRule: learned_rules_disabled_by_default" | \
  tail -20

# v1 rule creation
kubectl logs -f deployment/adaptive-learning -n production | \
  grep "disableNewNegatives flag" | \
  tail -20

# Bulk operations
kubectl logs -f deployment/quality-measurement-manager -n production | \
  grep "BulkStatusHandler" | \
  tail -20

# Errors (should be empty or minimal)
kubectl logs -f deployment/quality-measurement-manager -n production | \
  grep ERROR | \
  tail -20
```

---

## Rollback Procedures

### Quick Rollback (< 5 minutes)

**If critical issue detected during deployment:**

```bash
# 1. Disable feature flag
curl --location 'https://preprod.bito.ai/config/set' \
--header 'Authorization: ${TOKEN}' \
--data '{
  "workspace_id": 978573,
  "config_key": "agent-config.public.learned_rules_disabled_by_default",
  "config_value": "false",
  "module": "agent-config",
  "data_type": "BOOLEAN"
}'

# 2. Restart affected pods (rolling restart)
kubectl rollout undo deployment/quality-measurement-manager -n production
kubectl rollout undo deployment/adaptive-learning -n production
kubectl rollout undo deployment/agent-exec-mgmt -n production

# 3. Verify services recovering
kubectl rollout status deployment/quality-measurement-manager -n production
```

### Full Rollback (revert to previous version)

**If issues persist after quick rollback:**

```bash
# 1. Revert all deployments
for service in agent-def-config agent-exec-mgmt quality-measurement-manager adaptive-learning automation-platform; do
  kubectl rollout undo deployment/$service -n production
done

# 2. Verify all services healthy
kubectl get pods -n production

# 3. Revert database migration
# Requires DBAs - careful procedure
./scripts/rollback-migration.sh

# 4. Notify customer of rollback
# Send email with status update
```

### Zero-Downtime Rollback (Preferred)

```bash
# Use canary deployment to minimize risk

# 1. Deploy new version to 10% of traffic
kubectl set image deployment/quality-measurement-manager \
  qmm=bito/qmm:v1.8.0 \
  --record -n production

# 2. Monitor error rates for 15 minutes
./scripts/monitor-canary-errors.sh quality-measurement-manager 15

# 3. If errors detected, immediately rollback
kubectl rollout undo deployment/quality-measurement-manager -n production

# 4. If metrics look good, proceed with full rollout
kubectl rollout resume deployment/quality-measurement-manager -n production
```

---

## Post-Deployment Verification

### Day 1 Verification

```bash
# All services healthy
kubectl get deployments -n production | \
  awk '{if(NR>1 && $3!=$4) print "⚠️  " $1 " not ready"; else if(NR>1) print "✓ " $1}'

# No error spikes in logs
for service in qmm adaptive-learning; do
  errors=$(kubectl logs deployment/$service -n production --since=1h | grep ERROR | wc -l)
  if [ "$errors" -lt 5 ]; then
    echo "✓ $service: $errors errors in last hour (acceptable)"
  else
    echo "⚠️  $service: $errors errors in last hour (investigate)"
  fi
done

# Feature flag set correctly
curl https://preprod.bito.ai/config/get -d '{"workspace_id":978573,"config_key":"agent-config.public.learned_rules_disabled_by_default"}' | jq .

# Bulk endpoint working
curl -X PATCH https://preprod.bito.ai/qmm/api/v1/learning-rules/bulk-status \
  -d '{"workspace_id":2858,"action":"disable","scope":"all"}' | jq '.status.code' # Should be "1000"
```

### Week 1 Verification

```bash
# Run full integration test suite
go test ./integration_tests/... -v

# Check customer adoption
# Query: How many new rules created per day in workspace 978573?
mysql -e "SELECT DATE(created_at) as date, COUNT(*) as new_rules FROM cra_learned_rules 
           WHERE ws_id=978573 GROUP BY DATE(created_at) ORDER BY date DESC LIMIT 7;"

# Check flag usage
# Query: Are new rules being created disabled?
mysql -e "SELECT 
  SUM(CASE WHEN is_enabled=1 THEN 1 ELSE 0 END) as enabled,
  SUM(CASE WHEN is_enabled=0 AND is_disabled_by_user=0 THEN 1 ELSE 0 END) as system_disabled,
  SUM(CASE WHEN is_enabled=0 AND is_disabled_by_user=1 THEN 1 ELSE 0 END) as user_disabled
FROM cra_rule_metadata WHERE ws_id=978573 AND created_at > DATE_SUB(NOW(), INTERVAL 7 DAY);"

# Monitor performance
# Query: Is bulk endpoint performance acceptable?
# Check avg response time is < 2s for scope=all operations
```

### Monitoring Alerts to Configure

```yaml
# prometheus.yml additions
groups:
  - name: learned-rules-feature
    rules:
      - alert: FlagResolutionFailure
        expr: rate(flag_resolution_error[5m]) > 0.01
        annotations:
          summary: "Flag resolution failures exceeding 1%"
          
      - alert: BulkEndpointLatency
        expr: bulk_endpoint_latency_p95 > 2
        annotations:
          summary: "Bulk endpoint p95 latency > 2s"
          
      - alert: RuleCreationLatency
        expr: learning_event_latency_p95 > 0.5
        annotations:
          summary: "Rule creation latency > 500ms"
```

---

## Support & Documentation

- **Runbook**: ./RUNBOOK.md (troubleshooting guide)
- **Configuration**: ./CONFIG_EXAMPLES.md
- **API Reference**: ./CURL_RECIPES.md
- **Test Plan**: ./IMPLEMENTATION_GUIDE.md § Testing Checklist
- **Contact**: #bito-learned-rules Slack channel

---

## Sign-Off

- [ ] **Engineering Lead**: _________________ Date: _______
- [ ] **Product Manager**: _________________ Date: _______
- [ ] **QA Lead**: _________________ Date: _______
- [ ] **DevOps Lead**: _________________ Date: _______
