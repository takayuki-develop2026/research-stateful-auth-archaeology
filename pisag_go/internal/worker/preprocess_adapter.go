package worker

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	run "example.com/pisag_go/run"
)

type PreprocessTaskInputLookup interface {
	ListByTaskID(ctx context.Context, projectID string, taskID int64) ([]run.MultimodalTaskInput, error)
}

type PreprocessEvidenceSourceLookup interface {
	GetEvidenceSourceURI(ctx context.Context, projectID string, evidenceID int64) (string, error)
}

type GeneratedEvidenceRegistrar interface {
	RegisterGeneratedEvidence(
		ctx context.Context,
		projectID string,
		traceID string,
		runID string,
		kind string,
		contentType string,
		sourceURI string,
		sha256hex string,
		sizeBytes int64,
	) (int64, error)
}

type pythonPreprocessRequest struct {
	ContentB64     string         `json:"content_b64"`
	Filename       string         `json:"filename,omitempty"`
	ContentSHA256  string         `json:"content_sha256,omitempty"`
	Operations     []string       `json:"operations"`
	Options        map[string]any `json:"options,omitempty"`
}

type pythonPreprocessResponse struct {
	MediaType          string         `json:"media_type"`
	Ext                string         `json:"ext"`
	ProcessedContentB64 string        `json:"processed_content_b64"`
	ProcessedSHA256    string         `json:"processed_sha256"`
	Bytes              int            `json:"bytes"`
	Metadata           map[string]any `json:"metadata"`
}

type PythonPreprocessAdapter struct {
	BaseURL            string
	HTTPClient         *http.Client
	TaskInputs         PreprocessTaskInputLookup
	EvidenceSources    PreprocessEvidenceSourceLookup
	EvidenceRegistrar  GeneratedEvidenceRegistrar
	EvidenceStore      EvidenceStore
	BaseDir            string
}

func (a *PythonPreprocessAdapter) ExecutePreprocess(ctx context.Context, in run.PreprocessExecutionInput) (run.PreprocessExecutionOutput, error) {
	if in.Task.ID <= 0 {
		return run.PreprocessExecutionOutput{}, fmt.Errorf("python preprocess adapter: task id is required")
	}
	if a.TaskInputs == nil {
		return run.PreprocessExecutionOutput{}, fmt.Errorf("python preprocess adapter: task input lookup is nil")
	}
	if a.EvidenceSources == nil {
		return run.PreprocessExecutionOutput{}, fmt.Errorf("python preprocess adapter: evidence source lookup is nil")
	}
	if a.EvidenceRegistrar == nil {
		return run.PreprocessExecutionOutput{}, fmt.Errorf("python preprocess adapter: evidence registrar is nil")
	}
	if a.EvidenceStore == nil {
		baseDir := strings.TrimSpace(a.BaseDir)
		if baseDir == "" {
			baseDir = "./var/evidence"
		}
		a.EvidenceStore = NewFSEvidenceStore(baseDir)
	}

	baseURL := strings.TrimSpace(a.BaseURL)
	if baseURL == "" {
		baseURL = strings.TrimSpace(os.Getenv("AK_PYTHON_PREPROCESS_BASE_URL"))
	}
	if baseURL == "" {
		baseURL = strings.TrimSpace(os.Getenv("AK_PADDLEOCR_BASE_URL"))
	}
	if baseURL == "" {
		return run.PreprocessExecutionOutput{}, fmt.Errorf("python preprocess adapter: base url is required")
	}

	client := a.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 120 * time.Second}
	}

	sourceEvidenceID, sourcePath, err := a.resolveSource(ctx, in)
	if err != nil {
		return run.PreprocessExecutionOutput{}, fmt.Errorf("python preprocess adapter resolve source: %w", err)
	}

	raw, err := os.ReadFile(sourcePath)
	if err != nil {
		return run.PreprocessExecutionOutput{}, fmt.Errorf("python preprocess adapter read source: %w", err)
	}

	ops := preprocessSelectionKinds(in.Selection)
	if len(ops) == 0 {
		ops = []string{"opencv_basic"}
	}

	reqBody := pythonPreprocessRequest{
		ContentB64:    base64.StdEncoding.EncodeToString(raw),
		Filename:      filepath.Base(sourcePath),
		ContentSHA256: sha256HexBytes(raw),
		Operations:    ops,
		Options:       map[string]any{},
	}
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return run.PreprocessExecutionOutput{}, fmt.Errorf("python preprocess adapter marshal request: %w", err)
	}

	url := strings.TrimRight(baseURL, "/") + "/v1/preprocess"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return run.PreprocessExecutionOutput{}, fmt.Errorf("python preprocess adapter new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Project-Id", in.Task.ProjectID)
	req.Header.Set("X-Trace-Id", in.Task.TraceID)

	res, err := client.Do(req)
	if err != nil {
		return run.PreprocessExecutionOutput{}, fmt.Errorf("python preprocess adapter do request: %w", err)
	}
	defer res.Body.Close()

	respBody, err := io.ReadAll(res.Body)
	if err != nil {
		return run.PreprocessExecutionOutput{}, fmt.Errorf("python preprocess adapter read response: %w", err)
	}
	if res.StatusCode >= 400 {
		return run.PreprocessExecutionOutput{}, fmt.Errorf("python preprocess adapter status=%d body=%s", res.StatusCode, string(respBody))
	}

	var parsed pythonPreprocessResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return run.PreprocessExecutionOutput{}, fmt.Errorf("python preprocess adapter parse response: %w", err)
	}

	outBytes, err := base64.StdEncoding.DecodeString(parsed.ProcessedContentB64)
	if err != nil {
		return run.PreprocessExecutionOutput{}, fmt.Errorf("python preprocess adapter decode processed content: %w", err)
	}

	ext := strings.TrimSpace(parsed.Ext)
	if ext == "" {
		ext = "bin"
	}
	kind := "preprocess_output"
	if strings.TrimSpace(parsed.MediaType) == "application/pdf" {
		kind = "preprocess_output_pdf"
		ext = "pdf"
	} else {
		kind = "preprocess_output_image"
	}

	relPath, shaHex, size, err := a.EvidenceStore.SaveBlob(
		ctx,
		in.Task.RunID,
		"preprocess_output",
		ext,
		bytes.NewReader(outBytes),
		int64(len(outBytes))+1,
	)
	if err != nil {
		return run.PreprocessExecutionOutput{}, fmt.Errorf("python preprocess adapter save blob: %w", err)
	}

	absPath := relPathToAbs(a.BaseDir, relPath)
	evidenceID, err := a.EvidenceRegistrar.RegisterGeneratedEvidence(
		ctx,
		in.Task.ProjectID,
		in.Task.TraceID,
		in.Task.RunID,
		kind,
		strings.TrimSpace(parsed.MediaType),
		absPath,
		shaHex,
		int64(size),
	)
	if err != nil {
		return run.PreprocessExecutionOutput{}, fmt.Errorf("python preprocess adapter register generated evidence: %w", err)
	}

	summary := fmt.Sprintf("preprocess completed: ops=%v", ops)
	engineKind := run.EngineKindOpenCVBasic
	return run.PreprocessExecutionOutput{
		PayloadEvidenceAssetID:    evidenceID,
		ConfidenceEvidenceAssetID: nil,
		GeneratedOutputs: []run.MultimodalGeneratedOutput{
			{
				EvidenceID: evidenceID,
				OutputRole: run.MultimodalOutputRolePreprocessImage,
				Seq:        1,
			},
		},
		OutputHash:      sha256HexString(summary + "|" + shaHex),
		SummaryText:     summary,
		ConfidenceScore: floatPtr(0.95),
		ReasonCode:      "preprocess_ok",
		ReviewRequired:  false,
		EngineKind:      engineKind,
		EngineVersion:   "python-v1",
		Metadata: map[string]any{
			"adapter":            "python_preprocess",
			"source_evidence_id": sourceEvidenceID,
			"source_path":        sourcePath,
			"output_path":        absPath,
			"operations":         parsed.Metadata["operations"],
			"selection":          ops,
			"python_metadata":    parsed.Metadata,
		},
	}, nil
}

func (a *PythonPreprocessAdapter) resolveSource(ctx context.Context, in run.PreprocessExecutionInput) (int64, string, error) {
	if in.SourceEvidenceID != nil && *in.SourceEvidenceID > 0 {
		p, err := a.EvidenceSources.GetEvidenceSourceURI(ctx, in.Task.ProjectID, *in.SourceEvidenceID)
		if err != nil {
			return 0, "", err
		}
		return *in.SourceEvidenceID, p, nil
	}

	inputs, err := a.TaskInputs.ListByTaskID(ctx, in.Task.ProjectID, in.Task.ID)
	if err != nil {
		return 0, "", fmt.Errorf("list task inputs: %w", err)
	}
	if len(inputs) == 0 {
		return 0, "", fmt.Errorf("no task inputs found")
	}

	selected := inputs[0]
	for _, item := range inputs {
		if item.InputRole == run.MultimodalInputRolePrimary {
			selected = item
			break
		}
	}

	p, err := a.EvidenceSources.GetEvidenceSourceURI(ctx, in.Task.ProjectID, selected.EvidenceID)
	if err != nil {
		return 0, "", fmt.Errorf("get evidence source uri: %w", err)
	}
	return selected.EvidenceID, p, nil
}

func preprocessSelectionKinds(sel run.EngineSelection) []string {
	out := make([]string, 0, len(sel.Preprocess))
	for _, ch := range sel.Preprocess {
		out = append(out, string(ch.Kind))
	}
	return out
}

func relPathToAbs(baseDir string, rel string) string {
	baseDir = strings.TrimSpace(baseDir)
	if baseDir == "" {
		baseDir = "./var/evidence"
	}
	p := filepath.Join(baseDir, rel)
	abs, err := filepath.Abs(p)
	if err != nil {
		return p
	}
	return abs
}

func sha256HexString(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func sha256HexBytes(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func floatPtr(v float64) *float64 {
	return &v
}