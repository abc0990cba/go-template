// Package jobs defines the distributed task types and their versioned
// payloads. Both the API enqueue path and the worker handlers depend on
// these definitions, so the contract lives in one place.
package jobs

import (
	"encoding/json"
	"fmt"
)

// Task types, as registered by the worker and requested by producers.
const (
	TypeEnrichBatch    = "user.enrich_batch"
	TypeReportGenerate = "user.report_generate"
)

// EnrichBatch is the payload for TypeEnrichBatch. Bump PayloadVersion when
// the shape changes incompatibly and handle old versions in the handler.
type EnrichBatch struct {
	PayloadVersion int    `json:"payload_version"`
	JobID          string `json:"job_id"`
	Count          int    `json:"count"`
}

// Validate clamps and checks the batch size.
func (p EnrichBatch) Validate() (int, error) {
	if p.Count <= 0 || p.Count > 100 {
		return 0, fmt.Errorf("count must be between 1 and 100, got %d", p.Count)
	}
	return p.Count, nil
}

// ReportGenerate is the payload for TypeReportGenerate.
type ReportGenerate struct {
	PayloadVersion int    `json:"payload_version"`
	JobID          string `json:"job_id"`
}

// Encode marshals a payload to the task body.
func Encode(payload any) ([]byte, error) {
	return json.Marshal(payload)
}
