package run

type MultimodalTaskInputRef struct {
	EvidenceID int64
	SHA256     string
	Kind       string
	Bytes      int64
	InputRole  MultimodalInputRole
	Seq        int
}

type BuildMultimodalInputHashInput struct {
	ProjectID        string
	TaskType         MultimodalTaskType
	PipelineVersion  string
	PolicyVersionStr string
	Inputs           []MultimodalTaskInputRef

	// 可変本文は evidence 側にあるが、
	// canonical 化のために calling side で evidence 本文を解決し、
	// その正規化済み文字列を入れる。
	OptionsCanonical string
}

type BuildMultimodalTaskKeyInput struct {
	ProjectID        string
	RunID            string
	TaskType         MultimodalTaskType
	PipelineVersion  string
	PolicyVersionStr string
	InputHash        string
}

type BuildMultimodalResultKeyInput struct {
	ProjectID  string
	TaskID     int64
	ResultType MultimodalResultType
	ModelRunID *int64
	OutputHash string
}
