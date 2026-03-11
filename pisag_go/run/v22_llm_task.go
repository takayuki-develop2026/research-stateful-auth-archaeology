package run

type LLMTaskKind string

const (
	LLMTaskKindGenerateCopy    LLMTaskKind = "generate_copy"
	LLMTaskKindPolicyCheck     LLMTaskKind = "policy_check"
	LLMTaskKindSummarize       LLMTaskKind = "summarize"
	LLMTaskKindAttributeExplain LLMTaskKind = "attribute_explain"
)

type LLMTaskInput struct {
	TaskID       int64                  `json:"task_id"`
	ProjectID    string                 `json:"project_id"`
	TaskKind     LLMTaskKind            `json:"task_kind"`
	Selection    EngineSelection        `json:"selection"`
	Context      map[string]any         `json:"context,omitempty"`
	InputHash    string                 `json:"input_hash"`
	PromptVersion string                `json:"prompt_version"`
}