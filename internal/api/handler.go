package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/nexora/nexora_segmentation/internal/models"
	"github.com/nexora/nexora_segmentation/internal/service"
)

// POST /segments/evaluate
// Body: models.SegmentPayload (like the JSON you shared)
// Query (optional): ?segment_id=abc-123 (will override payload's SegmentID)
func EvaluateHandler(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	fmt.Println(r.Body)
	fmt.Println("(((((((((((((((((((((r.Body)))))))))))))))))))))")
	var req models.SegmentPayload
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if sid := r.URL.Query().Get("segment_id"); sid != "" {
		req.SegmentID = sid
	}

	members, err := service.Evaluate(req)
	if err != nil {
		http.Error(w, "evaluation failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	resp := map[string]any{
		"segment_id": req.SegmentID,
		"count":      len(members),
		"matches":    members, // list of {nexora_id, client_id}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// Simple health
func HealthCheckHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("OK"))
}
