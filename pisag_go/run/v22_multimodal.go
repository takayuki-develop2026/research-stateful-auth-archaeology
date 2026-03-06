package run

import "time"

type MultimodalTaskType string

const (
	MultimodalTaskTypeFulltextExtract MultimodalTaskType = "fulltext_extract"
	MultimodalTaskTypeOCR             MultimodalTaskType = "ocr"
	MultimodalTaskTypeVision          MultimodalTaskType = "vision"
	MultimodalTaskTypeAudioTranscribe MultimodalTaskType = "audio_transcribe"
	MultimodalTaskTypeAudioClassify   MultimodalTaskType = "audio_classify"
)

type MultimodalTaskStatus string

const (
	MultimodalTaskStatusQueued         MultimodalTaskStatus = "queued"
	MultimodalTaskStatusRunning        MultimodalTaskStatus = "running"
	MultimodalTaskStatusSucceeded      MultimodalTaskStatus = "succeeded"
	MultimodalTaskStatusReviewRequired MultimodalTaskStatus = "review_required"
	MultimodalTaskStatusSkippedBudget  MultimodalTaskStatus = "skipped_budget"
	MultimodalTaskStatusFailedSoft     MultimodalTaskStatus = "failed_soft"
	MultimodalTaskStatusBlockedPolicy  MultimodalTaskStatus = "blocked_policy"
)

type MultimodalInputRole string

const (
	MultimodalInputRolePrimary  MultimodalInputRole = "primary"
	MultimodalInputRolePage     MultimodalInputRole = "page"
	MultimodalInputRoleSegment  MultimodalInputRole = "segment"
	MultimodalInputRoleContext  MultimodalInputRole = "context"
	MultimodalInputRoleFallback MultimodalInputRole = "fallback"
)

type MultimodalResultType string

const (
	MultimodalResultTypeExtractedText  MultimodalResultType = "extracted_text"
	MultimodalResultTypeOCRText        MultimodalResultType = "ocr_text"
	MultimodalResultTypeVisionLabels   MultimodalResultType = "vision_labels"
	MultimodalResultTypeVisionEntities MultimodalResultType = "vision_entities"
	MultimodalResultTypeTranscript     MultimodalResultType = "transcript"
	MultimodalResultTypeAudioLabels    MultimodalResultType = "audio_labels"
	MultimodalResultTypeEmbedding      MultimodalResultType = "embedding"
)

type MultimodalOutputRole string

const (
	MultimodalOutputRoleThumbnail      MultimodalOutputRole = "thumbnail"
	MultimodalOutputRolePageImage      MultimodalOutputRole = "page_image"
	MultimodalOutputRoleOCRInputImage  MultimodalOutputRole = "ocr_input_image"
	MultimodalOutputRoleSpectrogram    MultimodalOutputRole = "spectrogram"
	MultimodalOutputRoleAnnotatedImage MultimodalOutputRole = "annotated_image"
	MultimodalOutputRoleModelOutput    MultimodalOutputRole = "model_output"
	MultimodalOutputRoleTranscript     MultimodalOutputRole = "transcript"
)

type PIIRedactionAction string

const (
	PIIRedactionActionMask  PIIRedactionAction = "mask"
	PIIRedactionActionDeny  PIIRedactionAction = "deny"
	PIIRedactionActionAllow PIIRedactionAction = "allow"
)

type PIIRedactionAppliedByType string

const (
	PIIRedactionAppliedBySystem PIIRedactionAppliedByType = "system"
	PIIRedactionAppliedByHuman  PIIRedactionAppliedByType = "human"
)

type MultimodalTask struct {
	ID        int64
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

	StartedAtUTC             *time.Time
	FinishedAtUTC            *time.Time
	SoftErrorEvidenceAssetID *int64

	CreatedAtUTC time.Time
	UpdatedAtUTC time.Time
}

type MultimodalTaskInput struct {
	ID         int64
	ProjectID  string
	TaskID     int64
	EvidenceID int64
	InputRole  MultimodalInputRole
	Seq        int

	CreatedAtUTC time.Time
}

type MultimodalResult struct {
	ID        int64
	ProjectID string
	TraceID   string
	RunID     string
	TaskID    int64

	ResultKey  string
	ResultType MultimodalResultType
	OutputHash string

	PayloadEvidenceAssetID    int64
	ConfidenceEvidenceAssetID *int64

	CreatedAtUTC time.Time
	UpdatedAtUTC time.Time
}

type MultimodalResultOutput struct {
	ID         int64
	ProjectID  string
	ResultID   int64
	EvidenceID int64
	OutputRole MultimodalOutputRole
	Seq        int

	CreatedAtUTC time.Time
}

type PIIRedaction struct {
	ID         int64
	ProjectID  string
	TraceID    string
	EvidenceID int64

	PolicyDecisionID int64
	RuleKey          string
	Action           PIIRedactionAction
	AppliedByType    PIIRedactionAppliedByType
	AppliedByID      string

	DetailEvidenceAssetID int64
	CreatedAtUTC          time.Time
}
