package run

type RuntimeCreateRunRequest struct {
	ProjectID        string                 `json:"project_id"`
	TraceID          string                 `json:"trace_id"`
	TaskType         MultimodalTaskType     `json:"task_type"`
	PipelineVersion  string                 `json:"pipeline_version"`
	PolicyVersionStr string                 `json:"policy_version_str"`
	Preset           RuntimePreset          `json:"preset"`
	EngineSelection  RuntimeEngineSelection `json:"engine_selection"`
	Inputs           []RuntimeInputRef      `json:"inputs"`
	Metadata         map[string]any         `json:"metadata,omitempty"`
}

type RuntimeEngineSelection struct {
	Preprocess []string `json:"preprocess"`
	OCR        []string `json:"ocr"`
	DocParse   []string `json:"docparse"`
	Embedding  []string `json:"embedding"`
	Vision     []string `json:"vision"`
	LLM        []string `json:"llm"`
}

type RuntimeInputRef struct {
	InputRole  MultimodalInputRole `json:"input_role"`
	EvidenceID int64               `json:"evidence_id"`
	Seq        int                 `json:"seq"`
	SHA256     string              `json:"sha256"`
	Kind       string              `json:"kind"`
	Bytes      int64               `json:"bytes"`
}

type RegisterRuntimeJSONEvidenceInput struct {
	ProjectID   string
	TraceID     string
	Kind        string
	BodyJSON    string
	SHA256      string
	Description string
}

type RegisterRuntimeJSONEvidenceOutput struct {
	EvidenceAssetID int64
}

func (r RuntimeCreateRunRequest) ToEngineSelection() EngineSelection {
	return EngineSelection{
		Preset:     r.Preset,
		Preprocess: stringKindsToChoices(r.EngineSelection.Preprocess),
		OCR:        stringKindsToChoices(r.EngineSelection.OCR),
		DocParse:   stringKindsToChoices(r.EngineSelection.DocParse),
		Embedding:  stringKindsToChoices(r.EngineSelection.Embedding),
		Vision:     stringKindsToChoices(r.EngineSelection.Vision),
		LLM:        stringKindsToChoices(r.EngineSelection.LLM),
	}
}

func (r RuntimeCreateRunRequest) ToTaskInputRefs() []MultimodalTaskInputRef {
	out := make([]MultimodalTaskInputRef, 0, len(r.Inputs))
	for _, in := range r.Inputs {
		out = append(out, MultimodalTaskInputRef{
			InputRole:  in.InputRole,
			EvidenceID: in.EvidenceID,
			Seq:        in.Seq,
			SHA256:     in.SHA256,
			Kind:       in.Kind,
			Bytes:      in.Bytes,
		})
	}
	return out
}

func stringKindsToChoices(in []string) []EngineChoice {
	out := make([]EngineChoice, 0, len(in))
	seen := map[string]struct{}{}

	for _, v := range in {
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, EngineChoice{
			Kind: EngineKind(v),
		})
	}

	return out
}