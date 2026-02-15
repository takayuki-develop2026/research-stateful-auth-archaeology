package httpx

/*
P0 contract (v3):
- HTTP should expose:
  - state: internal progress state (queued/running/done/review_required/failed/blocked/...)
  - status: public 2-value status (review_required/failed) or omitted when empty
  - result: pending/success/failed
- mode: optional integer (v3.2)
*/

type CreateRunReq struct {
	ProjectID       string `json:"project_id"`
	PolicyVersion   string `json:"policy_version"`
	PipelineVersion string `json:"pipeline_version"`
	Mode            *int   `json:"mode"`
}

type CreateRunResp struct {
	RunID   string `json:"run_id"`
	TraceID string `json:"trace_id"`

	// internal progress state (queued/running/done/review_required/failed/blocked/...)
	State string `json:"state"`

	// public 2-value status (review_required/failed) or omitted when empty
	Status string `json:"status,omitempty"`

	// pending/success/failed
	Result string `json:"result"`

	Note string `json:"note,omitempty"`
}