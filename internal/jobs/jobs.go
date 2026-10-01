// Package jobs owns the background-jobs feature: the distributed task
// contract (task types and versioned payloads), the enqueue/process service,
// and the jobs HTTP surface. Both the API enqueue path and the worker
// handlers depend on the contract, so it lives in one place.
package jobs

import (
	"encoding/json"
	"fmt"
)

// Task types, as registered by the worker and requested by producers.
const (
	TypeEnrichBatch    = "user.enrich_batch"
	TypeReportGenerate = "user.report_generate"
	TypeWebhookDeliver = "webhook.deliver"
	TypeExportGenerate = "user.export_generate"
)

// ExportGenerate is the payload for TypeExportGenerate: one user's GDPR
// data bundle, stored as a file owned by them.
type ExportGenerate struct {
	PayloadVersion int    `json:"payload_version"`
	JobID          string `json:"job_id"`
	UserID         string `json:"user_id"`
}

// WebhookDeliver is the payload for TypeWebhookDeliver: one recorded
// delivery to (re-)post through the queue path.
type WebhookDeliver struct {
	PayloadVersion int    `json:"payload_version"`
	EndpointID     string `json:"endpoint_id"`
	DeliveryID     string `json:"delivery_id"`
}

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
