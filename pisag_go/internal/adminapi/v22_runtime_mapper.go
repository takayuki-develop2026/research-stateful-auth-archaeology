package adminapi

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	run "example.com/pisag_go/run"
)

func BuildSelectionFromRequest(req run.RuntimeCreateRunRequest) run.EngineSelection {
	return req.ToEngineSelection()
}

func BuildInputRefsFromRequest(req run.RuntimeCreateRunRequest) []run.MultimodalTaskInputRef {
	return req.ToTaskInputRefs()
}

func BuildOptionsSHA256(optionsCanonicalJSON string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(optionsCanonicalJSON)))
	return hex.EncodeToString(sum[:])
}

func BuildRoutePlanSHA256(routePlanCanonicalJSON string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(routePlanCanonicalJSON)))
	return hex.EncodeToString(sum[:])
}

func BuildCreateTaskInput(
	req run.RuntimeCreateRunRequest,
	traceID string,
	runID string,
	inputHash string,
	routerPlanEvidenceAssetID int64,
	optionsEvidenceAssetID int64,
) usecaseCreateTaskInput {
	return usecaseCreateTaskInput{
		ProjectID:                 req.ProjectID,
		TraceID:                   traceID,
		RunID:                     runID,
		TaskType:                  req.TaskType,
		PipelineVersion:           req.PipelineVersion,
		PolicyVersionStr:          req.PolicyVersionStr,
		InputHash:                 inputHash,
		RouterPlanEvidenceAssetID: routerPlanEvidenceAssetID,
		OptionsEvidenceAssetID:    optionsEvidenceAssetID,
	}
}

func BuildAttachTaskInputsInput(
	projectID string,
	taskID int64,
	inputs []run.MultimodalTaskInputRef,
) usecaseAttachTaskInputsInput {
	out := make([]run.AttachMultimodalTaskInputInput, 0, len(inputs))
	for _, in := range inputs {
		out = append(out, run.AttachMultimodalTaskInputInput{
			ProjectID:  projectID,
			TaskID:     taskID,
			EvidenceID: in.EvidenceID,
			InputRole:  in.InputRole,
			Seq:        in.Seq,
		})
	}

	return usecaseAttachTaskInputsInput{
		ProjectID: projectID,
		TaskID:    taskID,
		Inputs:    out,
	}
}

func BuildCreateRunResponse(
	task run.MultimodalTask,
	selection run.EngineSelection,
	routePlanJSON string,
	optionsJSON string,
) map[string]any {
	return map[string]any{
		"id": task.ID,
		"task": map[string]any{
			"id":                          task.ID,
			"project_id":                  task.ProjectID,
			"trace_id":                    task.TraceID,
			"run_id":                      task.RunID,
			"task_type":                   task.TaskType,
			"task_key":                    task.TaskKey,
			"pipeline_version":            task.PipelineVersion,
			"policy_version_str":          task.PolicyVersionStr,
			"input_hash":                  task.InputHash,
			"status":                      task.Status,
			"router_plan_evidence_asset_id": task.RouterPlanEvidenceAssetID,
			"options_evidence_asset_id":     task.OptionsEvidenceAssetID,
		},
		"engine_selection": map[string]any{
			"preset":     selection.Preset,
			"preprocess": stringifyChoices(selection.Preprocess),
			"ocr":        stringifyChoices(selection.OCR),
			"docparse":   stringifyChoices(selection.DocParse),
			"embedding":  stringifyChoices(selection.Embedding),
			"vision":     stringifyChoices(selection.Vision),
			"llm":        stringifyChoices(selection.LLM),
		},
		"route_plan_json":   routePlanJSON,
		"options_json":      optionsJSON,
	}
}

func stringifyChoices(in []run.EngineChoice) []string {
	out := make([]string, 0, len(in))
	for _, ch := range in {
		out = append(out, string(ch.Kind))
	}
	return out
}

func GenerateTraceIDOrDefault(v string) string {
	v = strings.TrimSpace(v)
	if v != "" {
		return v
	}
	// Week2.5: deterministic enough for local/dev use
	return fmt.Sprintf("airt-%s", shortRandomHex())
}

func GenerateRunID(projectID string, taskType run.MultimodalTaskType) string {
	return fmt.Sprintf("run-%s-%s-%s", strings.TrimSpace(projectID), string(taskType), shortRandomHex())
}

func shortRandomHex() string {
	raw := []byte(fmt.Sprintf("%d", len("v22.1")))
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])[:12]
}

func CompactJSON(raw string) string {
	var v any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return raw
	}
	b, err := json.Marshal(v)
	if err != nil {
		return raw
	}
	return string(b)
}

// usecase input aliases to keep mapper isolated from direct package import cycles
type usecaseCreateTaskInput struct {
	ProjectID                 string
	TraceID                   string
	RunID                     string
	TaskType                  run.MultimodalTaskType
	PipelineVersion           string
	PolicyVersionStr          string
	InputHash                 string
	RouterPlanEvidenceAssetID int64
	OptionsEvidenceAssetID    int64
}

type usecaseAttachTaskInputsInput struct {
	ProjectID string
	TaskID    int64
	Inputs    []run.AttachMultimodalTaskInputInput
}