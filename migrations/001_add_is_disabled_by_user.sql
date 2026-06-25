-- Migration: Add is_disabled_by_user column and update schema for learned rules disable-by-default feature
-- BITO-13843: Give workspaces control over learned rule activation and application

-- This column distinguishes between:
-- - System default-disabled (is_enabled=0, is_disabled_by_user=0)
-- - User-disabled (is_enabled=0, is_disabled_by_user=1)
--
-- Used primarily to protect user-disabled rules from being silently re-enabled by
-- v0 intensity-based auto-enable logic when learned_rules_disabled_by_default flag is ON.

ALTER TABLE cra_rule_metadata ADD COLUMN is_disabled_by_user TINYINT(1) NOT NULL DEFAULT 0;

-- Backfill existing rows with is_disabled_by_user=0 (they were not explicitly disabled by user)
UPDATE cra_rule_metadata SET is_disabled_by_user = 0 WHERE is_disabled_by_user IS NULL;

-- Index for faster lookups when filtering by disabled rules
ALTER TABLE cra_rule_metadata ADD INDEX idx_ws_enabled_disabled (ws_id, is_enabled, is_disabled_by_user);

-- Schema remains otherwise unchanged:
-- - is_enabled: 1 = rule applies, 0 = rule doesn't apply (both system and user-disabled)
-- - is_disabled_by_user: 1 = user explicitly disabled, 0 = system default-disabled or enabled

-- Verification query to check rule states after migration:
-- SELECT
--   id, rule_id, is_enabled, is_disabled_by_user,
--   CASE
--     WHEN is_enabled=1 THEN 'Enabled'
--     WHEN is_enabled=0 AND is_disabled_by_user=0 THEN 'System-disabled'
--     WHEN is_enabled=0 AND is_disabled_by_user=1 THEN 'User-disabled'
--   END AS status
-- FROM cra_rule_metadata
-- WHERE ws_id = ?;
