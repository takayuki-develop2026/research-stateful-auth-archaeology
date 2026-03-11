package worker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	run "example.com/pisag_go/run"
)

type StubPreprocessAdapter struct{}

func (a *StubPreprocessAdapter) ExecutePreprocess(ctx context.Context, in run.PreprocessExecutionInput) (run.PreprocessExecutionOutput, error) {
	_ = ctx

	if in.Task.ID <= 0 {
		return run.PreprocessExecutionOutput{}, fmt.Errorf("stub preprocess adapter: task id is required")
	}

	ops := []string{"opencv_basic"}
	if len(in.Selection.Preprocess) > 0 {
		ops = make([]string, 0, len(in.Selection.Preprocess))
		for _, ch := range in.Selection.Preprocess {
			ops = append(ops, string(ch.Kind))
		}
	}

	summary := fmt.Sprintf("preprocess completed: ops=%v", ops)
	outputHash := sha256Hex(summary)

	return run.PreprocessExecutionOutput{
		PayloadEvidenceAssetID:    stubEvidenceIDFromTask(in.Task.ID, 1001),
		ConfidenceEvidenceAssetID: nil,
		GeneratedOutputs: []run.MultimodalGeneratedOutput{
			{
				EvidenceID: stubEvidenceIDFromTask(in.Task.ID, 1002),
				OutputRole: run.MultimodalOutputRolePreprocessImage,
				Seq:        1,
			},
		},
		OutputHash:      outputHash,
		SummaryText:     summary,
		ConfidenceScore: floatPtr(0.95),
		ReasonCode:      "preprocess_ok",
		ReviewRequired:  false,
		EngineKind:      run.EngineKindOpenCVBasic,
		EngineVersion:   "v1",
		Metadata: map[string]any{
			"adapter":     "stub_preprocess",
			"operations":  ops,
			"task_type":   string(in.Task.TaskType),
			"project_id":  in.Task.ProjectID,
			"selection":   preprocessSelectionKinds(in.Selection),
		},
	}, nil
}

func preprocessSelectionKinds(sel run.EngineSelection) []string {
	out := make([]string, 0, len(sel.Preprocess))
	for _, ch := range sel.Preprocess {
		out = append(out, string(ch.Kind))
	}
	return out
}

func stubEvidenceIDFromTask(taskID int64, offset int64) int64 {
	return taskID*100000 + offset
}

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func floatPtr(v float64) *float64 {
	return &v
}