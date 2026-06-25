package api

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/bito/quality-measurement-manager/learning"
)

// APIHandler wraps the bulk status handler for HTTP
type APIHandler struct {
	bulkHandler *learning.BulkStatusHandler
}

func NewAPIHandler(bulkHandler *learning.BulkStatusHandler) *APIHandler {
	return &APIHandler{
		bulkHandler: bulkHandler,
	}
}

// PatchBulkStatus handles PATCH /qmm/api/v1/learning-rules/bulk-status
// Internal endpoint for bulk enabling/disabling learned negative rules
//
// Request body:
// {
//   "workspace_id": 978573,
//   "action": "enable|disable",
//   "scope": "all|selected",
//   "rule_ids": [...] // required for scope=selected
// }
//
// Response (200):
// {
//   "status": {"code": "1000", "message": "Rules updated successfully"},
//   "data": {"affected_count": N, "action": "...", "workspace_id": ...}
// }
func (h *APIHandler) PatchBulkStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]string{"error": "Method Not Allowed"})
		return
	}

	var req learning.BulkStatusRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("error decoding bulk status request: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "Invalid request body",
		})
		return
	}

	resp, err := h.bulkHandler.HandleBulkStatusUpdate(r.Context(), &req)
	if err != nil {
		statusCode := http.StatusInternalServerError
		if resp.Status.Code == "400" {
			statusCode = http.StatusBadRequest
		}
		w.WriteHeader(statusCode)
		json.NewEncoder(w).Encode(resp)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
}

// Helper to register the route
func RegisterBulkStatusRoute(mux *http.ServeMux, handler *APIHandler) {
	mux.HandleFunc("/qmm/api/v1/learning-rules/bulk-status", handler.PatchBulkStatus)
	log.Println("Registered: PATCH /qmm/api/v1/learning-rules/bulk-status")
}
