package worker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	run "example.com/pisag_go/run"
)

type StubEmbeddingAdapter struct{}

func (a *StubEmbeddingAdapter) ExecuteEmbedding(ctx context.Context, in run.EmbeddingExecutionInput) (run.EmbeddingExecutionOutput, error) {
	_ = ctx

	if in.Task.ID <= 0 {
		return run.EmbeddingExecutionOutput{}, fmt.Errorf("stub embedding adapter: task id is required")
	}

	engineKind := run.EngineKindOpenCLIP
	if primary, ok := in.Selection.PrimaryChoice(run.EngineCapabilityEmbedding); ok {
		engineKind = primary.Kind
	}

	summary := fmt.Sprintf("embedding completed: engine=%s", engineKind)
	outputHash := sha256HexEmbedding(summary)

	return run.EmbeddingExecutionOutput{
		PayloadEvidenceAssetID:    stubEmbeddingEvidenceIDFromTask(in.Task.ID, 3001),
		ConfidenceEvidenceAssetID: nil,
		GeneratedOutputs: []run.MultimodalGeneratedOutput{
			{
				EvidenceID: stubEmbeddingEvidenceIDFromTask(in.Task.ID, 3002),
				OutputRole: run.MultimodalOutputRoleEmbeddingVector,
				Seq:        1,
			},
			{
				EvidenceID: stubEmbeddingEvidenceIDFromTask(in.Task.ID, 3003),
				OutputRole: run.MultimodalOutputRoleEmbeddingCandidates,
				Seq:        2,
			},
		},
		OutputHash:      outputHash,
		SummaryText:     summary,
		ConfidenceScore: floatPtrEmbedding(0.88),
		ReasonCode:      "embedding_ok",
		ReviewRequired:  false,
		EngineKind:      engineKind,
		EngineVersion:   "v1",
		Metadata: map[string]any{
			"adapter":    "stub_embedding",
			"task_type":  string(in.Task.TaskType),
			"project_id": in.Task.ProjectID,
			"selection":  embeddingSelectionKinds(in.Selection),
			"embedding_vector_ref": fmt.Sprintf("stub://embedding/%d", in.Task.ID),
			"embedding_dim":        768,
			"top_candidates": []map[string]any{
				{"rank": 1, "candidate_id": "item_101", "score": 0.94},
				{"rank": 2, "candidate_id": "item_202", "score": 0.89},
				{"rank": 3, "candidate_id": "item_303", "score": 0.83},
			},
		},
	}, nil
}

func embeddingSelectionKinds(sel run.EngineSelection) []string {
	out := make([]string, 0, len(sel.Embedding))
	for _, ch := range sel.Embedding {
		out = append(out, string(ch.Kind))
	}
	return out
}

func stubEmbeddingEvidenceIDFromTask(taskID int64, offset int64) int64 {
	return taskID*100000 + offset
}

func sha256HexEmbedding(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func floatPtrEmbedding(v float64) *float64 {
	return &v
}