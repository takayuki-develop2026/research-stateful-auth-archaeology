package httpx

type CreateRunReq struct {
	ProjectID       string `json:"project_id"`
	PolicyVersion   string `json:"policy_version"`
	PipelineVersion string `json:"pipeline_version"`
	Mode            *int   `json:"mode"`
}

type CreateRunResp struct {
	RunID   string `json:"run_id"`
	TraceID string `json:"trace_id"`
	Status  string `json:"status"`
	Note    string `json:"note,omitempty"`
}
