package usecase

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	run "example.com/pisag_go/run"
)

type RouteV22ModelTaskUseCase struct {
	Catalog run.EngineCatalog
}

type RouteV22ModelTaskInput struct {
	ProjectID       string
	TaskType        run.MultimodalTaskType
	Preset          run.RuntimePreset
	Selection       *run.EngineSelection
	PipelineVersion string
	PolicyVersion   string
}

type RouteV22ModelTaskOutput struct {
	Selection              run.EngineSelection
	RoutePlan              V22RoutePlan
	OptionsCanonicalJSON   string
	RoutePlanCanonicalJSON string
}

type V22RoutePlan struct {
	TaskType        run.MultimodalTaskType `json:"task_type"`
	Preset          run.RuntimePreset      `json:"preset"`
	PipelineVersion string                 `json:"pipeline_version"`
	PolicyVersion   string                 `json:"policy_version"`
	Capabilities    []V22CapabilityRoute   `json:"capabilities"`
}

type V22CapabilityRoute struct {
	Capability run.EngineCapability `json:"capability"`
	Primary    *string              `json:"primary,omitempty"`
	Secondary  []string             `json:"secondary,omitempty"`
}

func (uc *RouteV22ModelTaskUseCase) Handle(ctx context.Context, in RouteV22ModelTaskInput) (RouteV22ModelTaskOutput, error) {
	_ = ctx

	if strings.TrimSpace(in.ProjectID) == "" {
		return RouteV22ModelTaskOutput{}, fmt.Errorf("route v22 model task: project_id is required")
	}
	if in.TaskType == "" {
		return RouteV22ModelTaskOutput{}, fmt.Errorf("route v22 model task: task_type is required")
	}
	if strings.TrimSpace(in.PipelineVersion) == "" {
		return RouteV22ModelTaskOutput{}, fmt.Errorf("route v22 model task: pipeline_version is required")
	}
	if strings.TrimSpace(in.PolicyVersion) == "" {
		return RouteV22ModelTaskOutput{}, fmt.Errorf("route v22 model task: policy_version is required")
	}

	catalog := uc.Catalog
	if len(catalog.Definitions) == 0 {
		catalog = run.DefaultV221EngineCatalog()
	}

	selection := resolveSelection(in.Preset, in.Selection)
	if err := selection.Validate(catalog); err != nil {
		return RouteV22ModelTaskOutput{}, fmt.Errorf("route v22 model task validate selection: %w", err)
	}

	plan := buildRoutePlan(in.TaskType, selection, in.PipelineVersion, in.PolicyVersion)

	optionsJSON, err := selection.CanonicalJSON()
	if err != nil {
		return RouteV22ModelTaskOutput{}, fmt.Errorf("route v22 model task options canonical json: %w", err)
	}

	planJSONBytes, err := json.Marshal(plan)
	if err != nil {
		return RouteV22ModelTaskOutput{}, fmt.Errorf("route v22 model task route plan canonical json: %w", err)
	}

	return RouteV22ModelTaskOutput{
		Selection:              selection,
		RoutePlan:              plan,
		OptionsCanonicalJSON:   string(planSafeCompactJSON(optionsJSON)),
		RoutePlanCanonicalJSON: string(planSafeCompactJSON(string(planJSONBytes))),
	}, nil
}

func resolveSelection(preset run.RuntimePreset, provided *run.EngineSelection) run.EngineSelection {
	if provided == nil {
		return run.DefaultSelectionForPreset(preset)
	}

	selection := *provided

	if selection.Preset == "" {
		selection.Preset = preset
	}

	if selection.Preset != run.RuntimePresetCustom {
		defaultSel := run.DefaultSelectionForPreset(selection.Preset)

		if len(selection.Preprocess) == 0 {
			selection.Preprocess = defaultSel.Preprocess
		}
		if len(selection.OCR) == 0 {
			selection.OCR = defaultSel.OCR
		}
		if len(selection.DocParse) == 0 {
			selection.DocParse = defaultSel.DocParse
		}
		if len(selection.Embedding) == 0 {
			selection.Embedding = defaultSel.Embedding
		}
		if len(selection.Vision) == 0 {
			selection.Vision = defaultSel.Vision
		}
		if len(selection.LLM) == 0 {
			selection.LLM = defaultSel.LLM
		}
	}

	return selection
}

func buildRoutePlan(
	taskType run.MultimodalTaskType,
	selection run.EngineSelection,
	pipelineVersion string,
	policyVersion string,
) V22RoutePlan {
	out := V22RoutePlan{
		TaskType:        taskType,
		Preset:          selection.Preset,
		PipelineVersion: pipelineVersion,
		PolicyVersion:   policyVersion,
		Capabilities:    make([]V22CapabilityRoute, 0, 6),
	}

	addCapability := func(cap run.EngineCapability, choices []run.EngineChoice) {
		if len(choices) == 0 {
			return
		}

		primary := string(choices[0].Kind)

		secondaries := make([]string, 0)
		for _, ch := range choices[1:] {
			secondaries = append(secondaries, string(ch.Kind))
		}

		out.Capabilities = append(out.Capabilities, V22CapabilityRoute{
			Capability: cap,
			Primary:    &primary,
			Secondary:  secondaries,
		})
	}

	switch taskType {
	case run.MultimodalTaskTypePreprocess:
		addCapability(run.EngineCapabilityPreprocess, selection.Preprocess)

	case run.MultimodalTaskTypeOCR:
		addCapability(run.EngineCapabilityPreprocess, selection.Preprocess)
		addCapability(run.EngineCapabilityOCR, selection.OCR)

	case run.MultimodalTaskTypeDocParse:
		addCapability(run.EngineCapabilityPreprocess, selection.Preprocess)
		addCapability(run.EngineCapabilityDocParse, selection.DocParse)

	case run.MultimodalTaskTypeEmbedding:
		addCapability(run.EngineCapabilityPreprocess, selection.Preprocess)
		addCapability(run.EngineCapabilityEmbedding, selection.Embedding)

	case run.MultimodalTaskTypeVision:
		addCapability(run.EngineCapabilityPreprocess, selection.Preprocess)
		addCapability(run.EngineCapabilityVision, selection.Vision)

	case run.MultimodalTaskTypeLLM:
		addCapability(run.EngineCapabilityLLM, selection.LLM)

	case run.MultimodalTaskTypeFulltextExtract:
		addCapability(run.EngineCapabilityPreprocess, selection.Preprocess)
		addCapability(run.EngineCapabilityOCR, selection.OCR)
		addCapability(run.EngineCapabilityDocParse, selection.DocParse)
		addCapability(run.EngineCapabilityLLM, selection.LLM)

	default:
		// 未対応 task_type は安全に最小構成で route を組まず、空 capabilities を返す
	}

	return out
}

func planSafeCompactJSON(raw string) []byte {
	var v any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return []byte(raw)
	}
	b, err := json.Marshal(v)
	if err != nil {
		return []byte(raw)
	}
	return b
}