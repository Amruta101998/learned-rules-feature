package learning

import (
	"context"
	"database/sql"
	"log"
)

// BulkStatusRequest represents the request to PATCH /qmm/api/v1/learning-rules/bulk-status
type BulkStatusRequest struct {
	WorkspaceID int64   `json:"workspace_id"`
	Action      string  `json:"action"` // "enable" or "disable"
	Scope       string  `json:"scope"`  // "all" or "selected"
	RuleIDs     []int64 `json:"rule_ids"`
}

// BulkStatusResponse represents the response
type BulkStatusResponse struct {
	Status struct {
		Code    string `json:"code"` // "1000" for success
		Message string `json:"message"`
	} `json:"status"`
	Data struct {
		AffectedCount int64  `json:"affected_count"`
		Action        string `json:"action"`
		WorkspaceID   int64  `json:"workspace_id"`
	} `json:"data"`
}

type BulkStatusHandler struct {
	repo BulkRulesRepository
}

type BulkRulesRepository interface {
	// BulkUpdateNegativeRuleStatus updates only negative rules (v0 all + v1 negative)
	// Returns count of rows actually changed
	BulkUpdateNegativeRuleStatus(ctx context.Context, wsID int64, ruleIDs []int64, scope string, isEnabled bool) (int64, error)
	ValidateWorkspaceAccess(ctx context.Context, wsID int64) (bool, error)
}

func NewBulkStatusHandler(repo BulkRulesRepository) *BulkStatusHandler {
	return &BulkStatusHandler{
		repo: repo,
	}
}

// HandleBulkStatusUpdate processes bulk enable/disable requests
// Internal endpoint, negatives-only (filters by version=0 OR rule_type='negative')
func (bh *BulkStatusHandler) HandleBulkStatusUpdate(ctx context.Context, req *BulkStatusRequest) (*BulkStatusResponse, error) {
	resp := &BulkStatusResponse{}

	// Validation
	if err := bh.validateRequest(req); err != nil {
		resp.Status.Code = "400"
		resp.Status.Message = err.Error()
		return resp, err
	}

	// Verify workspace access
	valid, err := bh.repo.ValidateWorkspaceAccess(ctx, req.WorkspaceID)
	if err != nil || !valid {
		resp.Status.Code = "400"
		resp.Status.Message = "Invalid workspace_id"
		return resp, err
	}

	log.Printf("BulkStatusHandler: processing %s action, scope=%s, workspace=%d, rule_count=%d",
		req.Action, req.Scope, req.WorkspaceID, len(req.RuleIDs))

	// Determine enabled state based on action
	isEnabled := req.Action == "enable"

	// Update rules
	affectedCount, err := bh.repo.BulkUpdateNegativeRuleStatus(ctx, req.WorkspaceID, req.RuleIDs, req.Scope, isEnabled)
	if err != nil {
		log.Printf("error in bulk update: %v", err)
		resp.Status.Code = "500"
		resp.Status.Message = "Failed to update rules"
		return resp, err
	}

	log.Printf("BulkStatusHandler: %s completed - workspace=%d, action=%s, affected=%d",
		req.Scope, req.WorkspaceID, req.Action, affectedCount)

	resp.Status.Code = "1000"
	resp.Status.Message = "Rules updated successfully"
	resp.Data.AffectedCount = affectedCount
	resp.Data.Action = req.Action
	resp.Data.WorkspaceID = req.WorkspaceID

	return resp, nil
}

// validateRequest validates the bulk status request
func (bh *BulkStatusHandler) validateRequest(req *BulkStatusRequest) error {
	// Validate workspace_id
	if req.WorkspaceID <= 0 {
		return sql.ErrNoRows // Simplified - should return proper error
	}

	// Validate action
	if req.Action != "enable" && req.Action != "disable" {
		return sql.ErrNoRows
	}

	// Validate scope
	if req.Scope != "all" && req.Scope != "selected" {
		return sql.ErrNoRows
	}

	// Validate rule_ids based on scope
	if req.Scope == "selected" && len(req.RuleIDs) == 0 {
		return sql.ErrNoRows
	}

	if req.Scope == "all" && len(req.RuleIDs) > 0 {
		return sql.ErrNoRows
	}

	return nil
}

// BulkUpdateNegativeRuleSQL represents the SQL logic for bulk updates
// Target: is_deleted=0 AND (version=0 OR rule_type='negative')
// Set: is_enabled=?, is_disabled_by_user=(1 if action=disable else 0)
func GenerateBulkUpdateSQL(wsID int64, ruleIDs []int64, scope string, isEnabled bool) string {
	isEnabledInt := 0
	if isEnabled {
		isEnabledInt = 1
	}

	isDisabledByUserInt := 0
	if !isEnabled {
		isDisabledByUserInt = 1
	}

	query := `
		UPDATE cra_rule_metadata
		SET is_enabled = ?, is_disabled_by_user = ?
		WHERE ws_id = ?
		  AND is_deleted = 0
		  AND rule_id IN (
		    SELECT id FROM cra_learned_rules
		    WHERE (version = 0 OR rule_type = 'negative')
		  )`

	if scope == "selected" && len(ruleIDs) > 0 {
		// Add rule_id filter for selected scope
		query += ` AND rule_id IN (?, ?, ...)`
	}

	log.Printf("BulkUpdate SQL: isEnabled=%d, isDisabledByUser=%d, wsId=%d",
		isEnabledInt, isDisabledByUserInt, wsID)

	return query
}
