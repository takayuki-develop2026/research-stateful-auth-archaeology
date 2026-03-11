package worker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	run "example.com/pisag_go/run"
)

type StubDocParseAdapter struct{}

func (a *StubDocParseAdapter) ExecuteDocParse(ctx context.Context, in run.DocParseExecutionInput) (run.DocParseExecutionOutput, error) {
	_ = ctx

	if in.Task.ID <= 0 {
		return run.DocParseExecutionOutput{}, fmt.Errorf("stub docparse adapter: task id is required")
	}

	engineKind := run.EngineKindPPStructureV3
	if primary, ok := in.Selection.PrimaryChoice(run.EngineCapabilityDocParse); ok {
		engineKind = primary.Kind
	}

	summary := fmt.Sprintf("docparse completed: engine=%s", engineKind)
	outputHash := sha256HexDoc(summary)

	return run.DocParseExecutionOutput{
		PayloadEvidenceAssetID:    stubDocEvidenceIDFromTask(in.Task.ID, 2001),
		ConfidenceEvidenceAssetID: nil,
		GeneratedOutputs: []run.MultimodalGeneratedOutput{
			{
				EvidenceID: stubDocEvidenceIDFromTask(in.Task.ID, 2002),
				OutputRole: run.MultimodalOutputRoleDocParseJSON,
				Seq:        1,
			},
			{
				EvidenceID: stubDocEvidenceIDFromTask(in.Task.ID, 2003),
				OutputRole: run.MultimodalOutputRoleDocParseMarkdown,
				Seq:        2,
			},
		},
		OutputHash:      outputHash,
		SummaryText:     summary,
		ConfidenceScore: floatPtrDoc(0.90),
		ReasonCode:      "docparse_ok",
		ReviewRequired:  false,
		EngineKind:      engineKind,
		EngineVersion:   "v1",
		Metadata: map[string]any{
			"adapter":    "stub_docparse",
			"task_type":  string(in.Task.TaskType),
			"project_id": in.Task.ProjectID,
			"selection":  docparseSelectionKinds(in.Selection),
			"blocks": []map[string]any{
				{"seq": 1, "block_type": "heading", "text": "Sample Heading"},
				{"seq": 2, "block_type": "paragraph", "text": "Sample paragraph"},
			},
			"reading_order": []int{1, 2},
			"tables":        []map[string]any{},
			"markdown_text": "# Sample Heading\n\nSample paragraph",
		},
	}, nil
}

func docparseSelectionKinds(sel run.EngineSelection) []string {
	out := make([]string, 0, len(sel.DocParse))
	for _, ch := range sel.DocParse {
		out = append(out, string(ch.Kind))
	}
	return out
}

func stubDocEvidenceIDFromTask(taskID int64, offset int64) int64 {
	return taskID*100000 + offset
}

func sha256HexDoc(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func floatPtrDoc(v float64) *float64 {
	return &v
}