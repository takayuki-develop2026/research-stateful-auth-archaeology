package usecase

import (
	"context"
	"fmt"
	"strings"

	run "example.com/pisag_go/run"
)

type RegisterRuntimeUploadedEvidencePort interface {
	RegisterUploadedEvidence(ctx context.Context, in run.RegisterRuntimeUploadedEvidenceInput) (run.RegisterRuntimeUploadedEvidenceOutput, error)
	GetUploadedEvidenceSummary(ctx context.Context, in run.GetRuntimeUploadedEvidenceSummaryInput) (run.GetRuntimeUploadedEvidenceSummaryOutput, error)
}

type RegisterRuntimeUploadedEvidenceUseCase struct {
	Evidence RegisterRuntimeUploadedEvidencePort
}

func (uc *RegisterRuntimeUploadedEvidenceUseCase) Handle(
	ctx context.Context,
	in run.RegisterRuntimeUploadedEvidenceInput,
) (run.RegisterRuntimeUploadedEvidenceOutput, error) {
	if uc.Evidence == nil {
		return run.RegisterRuntimeUploadedEvidenceOutput{}, fmt.Errorf("register runtime uploaded evidence: evidence port is nil")
	}

	projectID := strings.TrimSpace(in.ProjectID)
	if projectID == "" {
		return run.RegisterRuntimeUploadedEvidenceOutput{}, fmt.Errorf("register runtime uploaded evidence: project_id is required")
	}
	traceID := strings.TrimSpace(in.TraceID)
	if traceID == "" {
		return run.RegisterRuntimeUploadedEvidenceOutput{}, fmt.Errorf("register runtime uploaded evidence: trace_id is required")
	}
	taskType := strings.TrimSpace(in.TaskType)
	if taskType == "" {
		return run.RegisterRuntimeUploadedEvidenceOutput{}, fmt.Errorf("register runtime uploaded evidence: task_type is required")
	}
	inputRole := strings.TrimSpace(in.InputRole)
	if inputRole == "" {
		inputRole = "primary"
	}
	filename := strings.TrimSpace(in.OriginalFilename)
	if filename == "" {
		return run.RegisterRuntimeUploadedEvidenceOutput{}, fmt.Errorf("register runtime uploaded evidence: original_filename is required")
	}
	contentType := strings.TrimSpace(in.ContentType)
	if contentType == "" {
		return run.RegisterRuntimeUploadedEvidenceOutput{}, fmt.Errorf("register runtime uploaded evidence: content_type is required")
	}
	sha := strings.TrimSpace(in.SHA256)
	if sha == "" {
		return run.RegisterRuntimeUploadedEvidenceOutput{}, fmt.Errorf("register runtime uploaded evidence: sha256 is required")
	}
	if in.SizeBytes <= 0 {
		return run.RegisterRuntimeUploadedEvidenceOutput{}, fmt.Errorf("register runtime uploaded evidence: size_bytes must be > 0")
	}

	in.ProjectID = projectID
	in.TraceID = traceID
	in.TaskType = taskType
	in.InputRole = inputRole
	in.OriginalFilename = filename
	in.ContentType = contentType
	in.SHA256 = sha
	in.SourceURI = strings.TrimSpace(in.SourceURI)

	out, err := uc.Evidence.RegisterUploadedEvidence(ctx, in)
	if err != nil {
		return run.RegisterRuntimeUploadedEvidenceOutput{}, fmt.Errorf("register runtime uploaded evidence: %w", err)
	}
	return out, nil
}

type GetRuntimeUploadedEvidenceSummaryUseCase struct {
	Evidence RegisterRuntimeUploadedEvidencePort
}

func (uc *GetRuntimeUploadedEvidenceSummaryUseCase) Handle(
	ctx context.Context,
	in run.GetRuntimeUploadedEvidenceSummaryInput,
) (run.GetRuntimeUploadedEvidenceSummaryOutput, error) {
	if uc.Evidence == nil {
		return run.GetRuntimeUploadedEvidenceSummaryOutput{}, fmt.Errorf("get runtime uploaded evidence summary: evidence port is nil")
	}
	projectID := strings.TrimSpace(in.ProjectID)
	if projectID == "" {
		return run.GetRuntimeUploadedEvidenceSummaryOutput{}, fmt.Errorf("get runtime uploaded evidence summary: project_id is required")
	}
	if in.EvidenceID <= 0 {
		return run.GetRuntimeUploadedEvidenceSummaryOutput{}, fmt.Errorf("get runtime uploaded evidence summary: evidence_id must be > 0")
	}

	in.ProjectID = projectID

	out, err := uc.Evidence.GetUploadedEvidenceSummary(ctx, in)
	if err != nil {
		return run.GetRuntimeUploadedEvidenceSummaryOutput{}, fmt.Errorf("get runtime uploaded evidence summary: %w", err)
	}
	return out, nil
}