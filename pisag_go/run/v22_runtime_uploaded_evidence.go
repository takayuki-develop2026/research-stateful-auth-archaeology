package run

type RegisterRuntimeUploadedEvidenceInput struct {
	ProjectID        string
	TraceID          string
	TaskType         string
	InputRole        string
	OriginalFilename string
	ContentType      string
	SHA256           string
	SizeBytes        int64
	SourceURI        string
}

type RegisterRuntimeUploadedEvidenceOutput struct {
	EvidenceAssetID int64
	Kind            string
	Bytes           int64
	SHA256          string
	Filename        string
}

type GetRuntimeUploadedEvidenceSummaryInput struct {
	ProjectID  string
	EvidenceID int64
}

type GetRuntimeUploadedEvidenceSummaryOutput struct {
	EvidenceAssetID int64
	Kind            string
	Bytes           int64
	SHA256          string
	Filename        string
}