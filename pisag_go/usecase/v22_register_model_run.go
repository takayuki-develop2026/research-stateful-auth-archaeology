package usecase

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"example.com/pisag_go/postgres"
	run "example.com/pisag_go/run"
)

type RegisterV22ModelRunUseCase struct {
	Tasks     run.MultimodalTaskRepository
	ModelRuns *postgres.ModelRunRepo
}

type RegisterV22ModelRunInput struct {
	ProjectID      string
	TaskID         int64
	Capability     string
	EngineKind     run.EngineKind
	EngineVersion  string
	Status         string
	FinishedAt     *time.Time
	LatencyMS      *int64
	TokenUsage     map[string]any
	CostEstimate   *float64
	Metadata       map[string]any
}

type RegisterV22ModelRunOutput struct {
	Task       run.MultimodalTask
	ModelRunID int64
}

func (uc *RegisterV22ModelRunUseCase) Handle(ctx context.Context, in RegisterV22ModelRunInput) (RegisterV22ModelRunOutput, error) {
	if uc.Tasks == nil {
		return RegisterV22ModelRunOutput{}, fmt.Errorf("register v22 model run: tasks repository is nil")
	}
	if uc.ModelRuns == nil {
		return RegisterV22ModelRunOutput{}, fmt.Errorf("register v22 model run: model runs repo is nil")
	}
	if strings.TrimSpace(in.ProjectID) == "" {
		return RegisterV22ModelRunOutput{}, fmt.Errorf("register v22 model run: project_id is required")
	}
	if in.TaskID <= 0 {
		return RegisterV22ModelRunOutput{}, fmt.Errorf("register v22 model run: task_id is required")
	}
	if strings.TrimSpace(in.Capability) == "" {
		return RegisterV22ModelRunOutput{}, fmt.Errorf("register v22 model run: capability is required")
	}
	if in.EngineKind == "" {
		return RegisterV22ModelRunOutput{}, fmt.Errorf("register v22 model run: engine_kind is required")
	}
	if strings.TrimSpace(in.EngineVersion) == "" {
		return RegisterV22ModelRunOutput{}, fmt.Errorf("register v22 model run: engine_version is required")
	}

	task, err := uc.Tasks.FindByID(ctx, in.TaskID)
	if err != nil {
		return RegisterV22ModelRunOutput{}, fmt.Errorf("register v22 model run load task: %w", err)
	}
	if task.ProjectID != strings.TrimSpace(in.ProjectID) {
		return RegisterV22ModelRunOutput{}, fmt.Errorf("register v22 model run: task project mismatch")
	}

	startedAt := time.Now().UTC()
	if !task.StartedAtUTC.IsZero() {
		startedAt = task.StartedAtUTC.UTC()
	}

	finishedAt := in.FinishedAt
	if finishedAt == nil {
		now := time.Now().UTC()
		finishedAt = &now
	} else {
		t := finishedAt.UTC()
		finishedAt = &t
	}

	status := strings.TrimSpace(in.Status)
	if status == "" {
		status = "succeeded"
	}

	tokenUsageJSON, err := json.Marshal(defaultMap(in.TokenUsage))
	if err != nil {
		return RegisterV22ModelRunOutput{}, fmt.Errorf("register v22 model run marshal token usage: %w", err)
	}

	metadataJSON, err := json.Marshal(defaultMap(in.Metadata))
	if err != nil {
		return RegisterV22ModelRunOutput{}, fmt.Errorf("register v22 model run marshal metadata: %w", err)
	}

	modelRunID, err := uc.ModelRuns.Create(ctx, postgres.CreateModelRunInput{
		TaskID:         task.ID,
		ProjectID:      task.ProjectID,
		Capability:     strings.TrimSpace(in.Capability),
		EngineKind:     string(in.EngineKind),
		EngineVersion:  strings.TrimSpace(in.EngineVersion),
		Provider:       providerFromEngineKind(in.EngineKind),
		TaskKind:       nil,
		Status:         status,
		InputHash:      task.InputHash,
		StartedAt:      startedAt,
		FinishedAt:     finishedAt,
		LatencyMS:      in.LatencyMS,
		TokenUsageJSON: tokenUsageJSON,
		CostEstimate:   in.CostEstimate,
		MetadataJSON:   metadataJSON,
	})
	if err != nil {
		return RegisterV22ModelRunOutput{}, fmt.Errorf("register v22 model run create: %w", err)
	}

	return RegisterV22ModelRunOutput{
		Task:       task,
		ModelRunID: modelRunID,
	}, nil
}

func defaultMap(in map[string]any) map[string]any {
	if in == nil {
		return map[string]any{}
	}
	return in
}

func providerFromEngineKind(kind run.EngineKind) string {
	switch kind {
	case run.EngineKindGPT5:
		return "openai"
	case run.EngineKindGeminiFlash:
		return "google"
	case run.EngineKindClaudeHaiku45:
		return "anthropic"
	case run.EngineKindQwenVL,
		run.EngineKindGemma3,
		run.EngineKindMistralSmall,
		run.EngineKindOpenCLIP,
		run.EngineKindPaddleOCR,
		run.EngineKindPPStructureV3:
		return "opensource"
	case run.EngineKindOpenCVBasic,
		run.EngineKindDeblurBasic,
		run.EngineKindDeskewBasic,
		run.EngineKindDenoiseBasic:
		return "local"
	default:
		return "unknown"
	}
}