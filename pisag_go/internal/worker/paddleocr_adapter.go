package worker

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	run "example.com/pisag_go/run"
)

type OCRTaskInputLookup interface {
	ListByTaskID(ctx context.Context, projectID string, taskID int64) ([]run.MultimodalTaskInput, error)
}

type OCREvidenceSourceLookup interface {
	GetEvidenceSourceURI(ctx context.Context, projectID string, evidenceID int64) (string, error)
}

type PaddleOCRAdapter struct {
	BaseURL    string
	HTTPClient *http.Client

	TaskInputs       OCRTaskInputLookup
	EvidenceSources  OCREvidenceSourceLookup

	// 開発段階では true にすると、OCR サービス未接続でも
	// 疎通確認用の擬似結果を返せる
	AllowStubFallback bool
}

type paddleOCRRequest struct {
	ContentB64    string         `json:"content_b64"`
	ContentSHA256 string         `json:"content_sha256"`
	SourceURL     *string        `json:"source_url"`
	Options       map[string]any `json:"options"`
}

type paddleOCRResponse struct {
	Text string         `json:"text"`
	Meta map[string]any `json:"meta"`
}

func (a *PaddleOCRAdapter) ExecuteOCR(ctx context.Context, in run.OCRExecutionInput) (run.OCRExecutionOutput, error) {
	if in.Task.ID <= 0 {
		return run.OCRExecutionOutput{}, fmt.Errorf("paddleocr adapter: task id is required")
	}

	baseURL := strings.TrimSpace(a.BaseURL)
	if baseURL == "" {
		baseURL = strings.TrimSpace(os.Getenv("AK_PADDLEOCR_BASE_URL"))
	}
	if baseURL == "" {
		if a.AllowStubFallback {
			return a.stubOutput(in), nil
		}
		return run.OCRExecutionOutput{}, fmt.Errorf("paddleocr adapter: base url is required")
	}

	client := a.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 60 * time.Second}
	}

	sourcePath, err := a.resolveOCRSourcePath(ctx, in)
	if err != nil {
		if a.AllowStubFallback {
			return a.stubOutput(in), nil
		}
		return run.OCRExecutionOutput{}, fmt.Errorf("paddleocr adapter resolve source: %w", err)
	}

	respBody, err := a.callPaddleOCR(ctx, client, baseURL, sourcePath, in)
	if err != nil {
		if a.AllowStubFallback {
			return a.stubOutput(in), nil
		}
		return run.OCRExecutionOutput{}, fmt.Errorf("paddleocr adapter call: %w", err)
	}

	var parsed paddleOCRResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		if a.AllowStubFallback {
			return a.stubOutput(in), nil
		}
		return run.OCRExecutionOutput{}, fmt.Errorf("paddleocr adapter parse response: %w", err)
	}

	fullText := strings.ToValidUTF8(strings.TrimSpace(parsed.Text), "")
	confidence := extractConfidenceFromMeta(parsed.Meta)
	reviewRequired := extractDecisionFromMeta(parsed.Meta) != "accept"
	if strings.Contains(in.Task.PolicyVersionStr, "ocr-review") {
		reviewRequired = true
	}

	outputHash := paddleSHA256Hex(string(respBody))
	payloadEvidenceID := in.Task.OptionsEvidenceAssetID
	confidenceEvidenceID := paddleInt64Ptr(in.Task.OptionsEvidenceAssetID)

	serviceMeta := paddleDefaultAnyMap(parsed.Meta)
	serviceMeta["ocr_text"] = fullText
	serviceMeta["ocr_text_preview"] = buildOCRPreview(fullText, 1000)
	serviceMeta["ocr_text_length"] = len(fullText)

	meta := map[string]any{
		"adapter":      "paddleocr",
		"project_id":   in.Task.ProjectID,
		"task_id":      in.Task.ID,
		"task_type":    string(in.Task.TaskType),
		"source_path":  sourcePath,
		"service_meta": serviceMeta,
	}

	generated := []run.MultimodalGeneratedOutput{
		{
			EvidenceID: in.Task.RouterPlanEvidenceAssetID,
			OutputRole: run.MultimodalOutputRoleAnnotatedImage,
			Seq:        1,
		},
	}

	summary := buildOCRSummary(fullText, 120)
	if summary == "" {
		summary = "paddleocr extracted text"
	}

	return run.OCRExecutionOutput{
		PayloadEvidenceAssetID:    payloadEvidenceID,
		ConfidenceEvidenceAssetID: confidenceEvidenceID,
		GeneratedOutputs:          generated,
		OutputHash:                outputHash,
		SummaryText:               summary,
		ConfidenceScore:           confidence,
		ReasonCode:                reasonCode(reviewRequired, "ocr_low_confidence", "ocr_ok"),
		ReviewRequired:            reviewRequired,
		EngineKind:                run.EngineKindPaddleOCR,
		EngineVersion:             "v1",
		Metadata:                  meta,
	}, nil
}

func (a *PaddleOCRAdapter) callPaddleOCR(
	ctx context.Context,
	client *http.Client,
	baseURL string,
	sourcePath string,
	in run.OCRExecutionInput,
) ([]byte, error) {
	fileBytes, err := os.ReadFile(sourcePath)
	if err != nil {
		return nil, fmt.Errorf("read source file: %w", err)
	}

	contentSHA := sha256.Sum256(fileBytes)
	contentSHAHex := hex.EncodeToString(contentSHA[:])
	contentB64 := base64.StdEncoding.EncodeToString(fileBytes)

	reqBody := paddleOCRRequest{
		ContentB64:    contentB64,
		ContentSHA256: contentSHAHex,
		SourceURL:     nil,
		Options: map[string]any{
			"engine": "paddleocr",
			"mode":   "force_ocr",
			"lang":   getenvOr("AK_PADDLEOCR_LANG", "jpn"),
			"dpi":    getenvIntOr("AK_PADDLEOCR_DPI", 200),
			"budget": map[string]any{
				"max_ms":       getenvIntOr("AK_PADDLEOCR_BUDGET_MAX_MS", 15000),
				"max_cost_usd": 0.0,
			},
		},
	}

	payload, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	url := strings.TrimRight(baseURL, "/") + "/v1/extract/pdf_ocr"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(string(payload)))
	if err != nil {
		return nil, fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Project-Id", in.Task.ProjectID)
	req.Header.Set("X-Trace-Id", in.Task.TraceID)

	res, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer res.Body.Close()

	respBody, err := ioReadAll(res)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if res.StatusCode >= 400 {
		return nil, fmt.Errorf("ocr service status=%d body=%s", res.StatusCode, string(respBody))
	}

	return respBody, nil
}

func (a *PaddleOCRAdapter) resolveOCRSourcePath(ctx context.Context, in run.OCRExecutionInput) (string, error) {
	raw := strings.TrimSpace(os.Getenv("AK_OCR_SOURCE_PATH"))
	if raw != "" {
		return raw, nil
	}

	if a.TaskInputs == nil {
		return "", fmt.Errorf("task input lookup is nil")
	}
	if a.EvidenceSources == nil {
		return "", fmt.Errorf("evidence source lookup is nil")
	}

	inputs, err := a.TaskInputs.ListByTaskID(ctx, in.Task.ProjectID, in.Task.ID)
	if err != nil {
		return "", fmt.Errorf("list task inputs: %w", err)
	}
	if len(inputs) == 0 {
		return "", fmt.Errorf("no task inputs found")
	}

	var selected *run.MultimodalTaskInput
	for i := range inputs {
		if inputs[i].InputRole == run.MultimodalInputRolePrimary {
			selected = &inputs[i]
			break
		}
	}
	if selected == nil {
		selected = &inputs[0]
	}

	sourcePath, err := a.EvidenceSources.GetEvidenceSourceURI(ctx, in.Task.ProjectID, selected.EvidenceID)
	if err != nil {
		return "", fmt.Errorf("get evidence source uri: %w", err)
	}
	sourcePath = strings.TrimSpace(sourcePath)
	if sourcePath == "" {
		return "", fmt.Errorf("empty source path")
	}

	if _, err := os.Stat(sourcePath); err != nil {
		return "", fmt.Errorf("source path not readable: %w", err)
	}

	return sourcePath, nil
}

func (a *PaddleOCRAdapter) stubOutput(in run.OCRExecutionInput) run.OCRExecutionOutput {
	conf := 0.93
	reviewRequired := strings.Contains(in.Task.PolicyVersionStr, "ocr-review")
	if reviewRequired {
		conf = 0.42
	}

	fullText := "stub ocr extracted text"
	meta := map[string]any{
		"adapter":    "paddleocr_stub_fallback",
		"project_id": in.Task.ProjectID,
		"task_id":    in.Task.ID,
		"task_type":  string(in.Task.TaskType),
		"service_meta": map[string]any{
			"ocr_text":         fullText,
			"ocr_text_preview": fullText,
			"ocr_text_length":  len(fullText),
		},
	}

	return run.OCRExecutionOutput{
		PayloadEvidenceAssetID:    in.Task.OptionsEvidenceAssetID,
		ConfidenceEvidenceAssetID: paddleInt64Ptr(in.Task.OptionsEvidenceAssetID),
		GeneratedOutputs: []run.MultimodalGeneratedOutput{
			{
				EvidenceID: in.Task.RouterPlanEvidenceAssetID,
				OutputRole: run.MultimodalOutputRoleAnnotatedImage,
				Seq:        1,
			},
		},
		OutputHash:      fmt.Sprintf("stub_ocr_output_%d", in.Task.ID),
		SummaryText:     buildOCRSummary(fullText, 120),
		ConfidenceScore: &conf,
		ReasonCode:      reasonCode(reviewRequired, "ocr_low_confidence", "ocr_ok"),
		ReviewRequired:  reviewRequired,
		EngineKind:      run.EngineKindPaddleOCR,
		EngineVersion:   "v1",
		Metadata:        meta,
	}
}

func extractConfidenceFromMeta(meta map[string]any) *float64 {
	if meta == nil {
		return nil
	}

	if v, ok := meta["avg_confidence_normalized"]; ok {
		if f, ok := toFloat(v); ok {
			x := f / 100.0
			return &x
		}
	}

	if v, ok := meta["avg_confidence"]; ok {
		if f, ok := toFloat(v); ok {
			if f > 1.0 {
				f = f / 100.0
			}
			return &f
		}
	}

	return nil
}

func extractDecisionFromMeta(meta map[string]any) string {
	if meta == nil {
		return ""
	}
	v, _ := meta["decision"].(string)
	return strings.TrimSpace(v)
}

func toFloat(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case float32:
		return float64(x), true
	case int:
		return float64(x), true
	case int64:
		return float64(x), true
	case json.Number:
		f, err := x.Float64()
		return f, err == nil
	default:
		return 0, false
	}
}

func ioReadAll(res *http.Response) ([]byte, error) {
	defer res.Body.Close()
	return io.ReadAll(res.Body)
}

func paddleSHA256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func paddleInt64Ptr(v int64) *int64 {
	return &v
}

func paddleDefaultAnyMap(in map[string]any) map[string]any {
	if in == nil {
		return map[string]any{}
	}
	return in
}

func buildOCRSummary(fullText string, max int) string {
	s := strings.TrimSpace(fullText)
	if s == "" {
		return ""
	}

	s = strings.ToValidUTF8(s, "")
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.Join(strings.Fields(s), " ")

	rs := []rune(s)
	if len(rs) <= max {
		return string(rs)
	}
	return string(rs[:max])
}

func buildOCRPreview(fullText string, max int) string {
	s := strings.TrimSpace(fullText)
	if s == "" {
		return ""
	}

	s = strings.ToValidUTF8(s, "")
	rs := []rune(s)
	if len(rs) <= max {
		return string(rs)
	}
	return string(rs[:max])
}

func getenvOr(k, fallback string) string {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		return fallback
	}
	return v
}

func getenvIntOr(k string, fallback int) int {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}