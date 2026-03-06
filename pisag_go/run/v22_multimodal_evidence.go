package run

type RegisterMultimodalTaskInput struct {
	ProjectID string
	TraceID   string
	RunID     string

	TaskKey          string
	TaskType         MultimodalTaskType
	PipelineVersion  string
	PolicyVersionStr string
	InputHash        string

	Status                    MultimodalTaskStatus
	RouterPlanEvidenceAssetID int64
	OptionsEvidenceAssetID    int64
	ModelRunID                *int64
	SoftErrorEvidenceAssetID  *int64
}

type AttachMultimodalTaskInputInput struct {
	ProjectID  string
	TaskID     int64
	EvidenceID int64
	InputRole  MultimodalInputRole
	Seq        int
}

type RegisterMultimodalResultInput struct {
	ProjectID string
	TraceID   string
	RunID     string
	TaskID    int64

	ResultKey  string
	ResultType MultimodalResultType
	OutputHash string

	PayloadEvidenceAssetID    int64
	ConfidenceEvidenceAssetID *int64
}

type AttachMultimodalResultOutputInput struct {
	ProjectID  string
	ResultID   int64
	EvidenceID int64
	OutputRole MultimodalOutputRole
	Seq        int
}

type RegisterPIIRedactionInput struct {
	ProjectID  string
	TraceID    string
	EvidenceID int64

	PolicyDecisionID int64
	RuleKey          string
	Action           PIIRedactionAction
	AppliedByType    PIIRedactionAppliedByType
	AppliedByID      string

	DetailEvidenceAssetID int64
}
