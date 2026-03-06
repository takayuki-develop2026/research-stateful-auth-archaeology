package usecase

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	run "example.com/pisag_go/run"
)

type CreateMultimodalTaskUseCase struct {
	Tasks run.MultimodalTaskRepository
}

type CreateMultimodalTaskInput struct {
	ProjectID string
	TraceID   string
	RunID     string

	TaskType         run.MultimodalTaskType
	PipelineVersion  string
	PolicyVersionStr string
	InputHash        string

	RouterPlanEvidenceAssetID int64
	OptionsEvidenceAssetID    int64
	ModelRunID                *int64
	SoftErrorEvidenceAssetID  *int64
}

type CreateMultimodalTaskOutput struct {
	Task    run.MultimodalTask
	Created bool
}

func (uc *CreateMultimodalTaskUseCase) Handle(ctx context.Context, in CreateMultimodalTaskInput) (CreateMultimodalTaskOutput, error) {
	if uc.Tasks == nil {
		return CreateMultimodalTaskOutput{}, fmt.Errorf("create multimodal task: tasks repository is nil")
	}
	if strings.TrimSpace(in.ProjectID) == "" {
		return CreateMultimodalTaskOutput{}, fmt.Errorf("create multimodal task: project_id is required")
	}
	if strings.TrimSpace(in.TraceID) == "" {
		return CreateMultimodalTaskOutput{}, fmt.Errorf("create multimodal task: trace_id is required")
	}
	if strings.TrimSpace(in.RunID) == "" {
		return CreateMultimodalTaskOutput{}, fmt.Errorf("create multimodal task: run_id is required")
	}
	if in.TaskType == "" {
		return CreateMultimodalTaskOutput{}, fmt.Errorf("create multimodal task: task_type is required")
	}
	if strings.TrimSpace(in.PipelineVersion) == "" {
		return CreateMultimodalTaskOutput{}, fmt.Errorf("create multimodal task: pipeline_version is required")
	}
	if strings.TrimSpace(in.PolicyVersionStr) == "" {
		return CreateMultimodalTaskOutput{}, fmt.Errorf("create multimodal task: policy_version_str is required")
	}
	if strings.TrimSpace(in.InputHash) == "" {
		return CreateMultimodalTaskOutput{}, fmt.Errorf("create multimodal task: input_hash is required")
	}
	if in.RouterPlanEvidenceAssetID <= 0 {
		return CreateMultimodalTaskOutput{}, fmt.Errorf("create multimodal task: router_plan_evidence_asset_id is required")
	}
	if in.OptionsEvidenceAssetID <= 0 {
		return CreateMultimodalTaskOutput{}, fmt.Errorf("create multimodal task: options_evidence_asset_id is required")
	}

	taskKey := buildMultimodalTaskKey(run.BuildMultimodalTaskKeyInput{
		ProjectID:        in.ProjectID,
		RunID:            in.RunID,
		TaskType:         in.TaskType,
		PipelineVersion:  in.PipelineVersion,
		PolicyVersionStr: in.PolicyVersionStr,
		InputHash:        in.InputHash,
	})

	existing, err := uc.Tasks.FindByProjectAndTaskKey(ctx, in.ProjectID, taskKey)
	if err == nil {
		return CreateMultimodalTaskOutput{
			Task:    existing,
			Created: false,
		}, nil
	}

	task, err := uc.Tasks.Create(ctx, run.RegisterMultimodalTaskInput{
		ProjectID:                 in.ProjectID,
		TraceID:                   in.TraceID,
		RunID:                     in.RunID,
		TaskKey:                   taskKey,
		TaskType:                  in.TaskType,
		PipelineVersion:           in.PipelineVersion,
		PolicyVersionStr:          in.PolicyVersionStr,
		InputHash:                 in.InputHash,
		Status:                    run.MultimodalTaskStatusQueued,
		RouterPlanEvidenceAssetID: in.RouterPlanEvidenceAssetID,
		OptionsEvidenceAssetID:    in.OptionsEvidenceAssetID,
		ModelRunID:                in.ModelRunID,
		SoftErrorEvidenceAssetID:  in.SoftErrorEvidenceAssetID,
	})
	if err != nil {
		return CreateMultimodalTaskOutput{}, fmt.Errorf("create multimodal task create: %w", err)
	}

	return CreateMultimodalTaskOutput{
		Task:    task,
		Created: true,
	}, nil
}

func BuildMultimodalInputHash(in run.BuildMultimodalInputHashInput) (string, error) {
	if strings.TrimSpace(in.ProjectID) == "" {
		return "", fmt.Errorf("build multimodal input hash: project_id is required")
	}
	if in.TaskType == "" {
		return "", fmt.Errorf("build multimodal input hash: task_type is required")
	}
	if strings.TrimSpace(in.PipelineVersion) == "" {
		return "", fmt.Errorf("build multimodal input hash: pipeline_version is required")
	}
	if strings.TrimSpace(in.PolicyVersionStr) == "" {
		return "", fmt.Errorf("build multimodal input hash: policy_version_str is required")
	}
	if len(in.Inputs) == 0 {
		return "", fmt.Errorf("build multimodal input hash: inputs are required")
	}

	inputs := make([]run.MultimodalTaskInputRef, len(in.Inputs))
	copy(inputs, in.Inputs)

	sort.Slice(inputs, func(i, j int) bool {
		if inputs[i].Seq != inputs[j].Seq {
			return inputs[i].Seq < inputs[j].Seq
		}
		if inputs[i].InputRole != inputs[j].InputRole {
			return inputs[i].InputRole < inputs[j].InputRole
		}
		return inputs[i].EvidenceID < inputs[j].EvidenceID
	})

	var b strings.Builder
	b.WriteString("project_id=")
	b.WriteString(strings.TrimSpace(in.ProjectID))
	b.WriteString("|task_type=")
	b.WriteString(string(in.TaskType))
	b.WriteString("|pipeline_version=")
	b.WriteString(strings.TrimSpace(in.PipelineVersion))
	b.WriteString("|policy_version_str=")
	b.WriteString(strings.TrimSpace(in.PolicyVersionStr))
	b.WriteString("|options=")
	b.WriteString(strings.TrimSpace(in.OptionsCanonical))

	for _, ref := range inputs {
		b.WriteString("|input:")
		b.WriteString(fmt.Sprintf(
			"seq=%d,role=%s,evidence_id=%d,sha256=%s,kind=%s,bytes=%d",
			ref.Seq,
			ref.InputRole,
			ref.EvidenceID,
			strings.TrimSpace(ref.SHA256),
			strings.TrimSpace(ref.Kind),
			ref.Bytes,
		))
	}

	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:]), nil
}

func buildMultimodalTaskKey(in run.BuildMultimodalTaskKeyInput) string {
	s := strings.Join([]string{
		strings.TrimSpace(in.ProjectID),
		strings.TrimSpace(in.RunID),
		string(in.TaskType),
		strings.TrimSpace(in.PipelineVersion),
		strings.TrimSpace(in.PolicyVersionStr),
		strings.TrimSpace(in.InputHash),
	}, "|")
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}
