package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"example.com/pisag_go/internal/adminapi"
	"example.com/pisag_go/internal/worker"
	"example.com/pisag_go/postgres"
	run "example.com/pisag_go/run"
	"example.com/pisag_go/usecase"
)

func TestV22APIAndWorkerOCRFlow(t *testing.T) {
	db := openV22TestPool(t)
	defer db.Close()

	handler := buildV22HandlerForTest(db)
	mux := http.NewServeMux()
	adminapi.RegisterV22MultimodalRoutes(mux, handler)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	projectID := "akproj_0000000000000000000"
	runID := "run_v22_api_ocr_" + uuid.NewString()
	traceID := "trace_v22_api_ocr_" + uuid.NewString()

	inputHash, err := usecase.BuildMultimodalInputHash(run.BuildMultimodalInputHashInput{
		ProjectID:        projectID,
		TaskType:         run.MultimodalTaskTypeOCR,
		PipelineVersion:  "v22-api-ocr-1",
		PolicyVersionStr: "v22-api-ocr-policy-1",
		Inputs: []run.MultimodalTaskInputRef{
			{
				EvidenceID: 107,
				SHA256:     "api_ocr_sha",
				Kind:       "raw_pdf",
				Bytes:      1000,
				InputRole:  run.MultimodalInputRolePrimary,
				Seq:        0,
			},
		},
		OptionsCanonical: "ocr=true",
	})
	if err != nil {
		t.Fatalf("build input hash: %v", err)
	}

	createReq := map[string]any{
		"project_id":                    projectID,
		"trace_id":                      traceID,
		"run_id":                        runID,
		"task_type":                     string(run.MultimodalTaskTypeOCR),
		"pipeline_version":              "v22-api-ocr-1",
		"policy_version_str":            "v22-api-ocr-policy-1",
		"input_hash":                    inputHash,
		"router_plan_evidence_asset_id": int64(109),
		"options_evidence_asset_id":     int64(108),
		"inputs": []map[string]any{
			{
				"evidence_id": int64(107),
				"input_role":  string(run.MultimodalInputRolePrimary),
				"seq":         0,
			},
		},
	}

	taskID := createTaskViaAPI(t, srv.URL, createReq)

	execReq := map[string]any{
		"project_id":       projectID,
		"destination_kind": "provider_intelligence",
	}
	execResp := postJSONMap(t, srv.URL+"/v22/multimodal/tasks/"+strconv.FormatInt(taskID, 10)+"/execute/ocr", execReq)
	taskMap := execResp["task"].(map[string]any)
	if got := taskMap["Status"].(string); got != string(run.MultimodalTaskStatusSucceeded) {
		t.Fatalf("expected succeeded, got %s", got)
	}

	resultsResp := getJSONMap(t, srv.URL+"/v22/multimodal/results?project_id="+projectID+"&run_id="+runID)
	if int(resultsResp["count"].(float64)) != 1 {
		t.Fatalf("expected 1 result, got %v", resultsResp["count"])
	}

	reviewResp := getJSONMap(t, srv.URL+"/v22/multimodal/review-queue?project_id="+projectID+"&run_id="+runID)
	if int(reviewResp["count"].(float64)) != 0 {
		t.Fatalf("expected 0 review queue items, got %v", reviewResp["count"])
	}
}

func TestV22APIAndWorkerVisionReviewFlow(t *testing.T) {
	db := openV22TestPool(t)
	defer db.Close()

	handler := buildV22HandlerForTest(db)
	mux := http.NewServeMux()
	adminapi.RegisterV22MultimodalRoutes(mux, handler)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	projectID := "akproj_0000000000000000000"
	runID := "run_v22_api_vision_" + uuid.NewString()
	traceID := "trace_v22_api_vision_" + uuid.NewString()

	inputHash, err := usecase.BuildMultimodalInputHash(run.BuildMultimodalInputHashInput{
		ProjectID:        projectID,
		TaskType:         run.MultimodalTaskTypeVision,
		PipelineVersion:  "v22-api-vision-1",
		PolicyVersionStr: "vision-review",
		Inputs: []run.MultimodalTaskInputRef{
			{
				EvidenceID: 107,
				SHA256:     "api_vision_sha",
				Kind:       "raw_image",
				Bytes:      2000,
				InputRole:  run.MultimodalInputRolePrimary,
				Seq:        0,
			},
		},
		OptionsCanonical: "vision=true",
	})
	if err != nil {
		t.Fatalf("build input hash: %v", err)
	}

	createReq := map[string]any{
		"project_id":                    projectID,
		"trace_id":                      traceID,
		"run_id":                        runID,
		"task_type":                     string(run.MultimodalTaskTypeVision),
		"pipeline_version":              "v22-api-vision-1",
		"policy_version_str":            "vision-review",
		"input_hash":                    inputHash,
		"router_plan_evidence_asset_id": int64(109),
		"options_evidence_asset_id":     int64(108),
		"inputs": []map[string]any{
			{
				"evidence_id": int64(107),
				"input_role":  string(run.MultimodalInputRolePrimary),
				"seq":         0,
			},
		},
	}

	taskID := createTaskViaAPI(t, srv.URL, createReq)

	execReq := map[string]any{
		"project_id":       projectID,
		"destination_kind": "catalog_update",
	}
	execResp := postJSONMap(t, srv.URL+"/v22/multimodal/tasks/"+strconv.FormatInt(taskID, 10)+"/execute/vision", execReq)
	taskMap := execResp["task"].(map[string]any)
	if got := taskMap["Status"].(string); got != string(run.MultimodalTaskStatusReviewRequired) {
		t.Fatalf("expected review_required, got %s", got)
	}

	reviewResp := getJSONMap(t, srv.URL+"/v22/multimodal/review-queue?project_id="+projectID+"&run_id="+runID)
	if int(reviewResp["count"].(float64)) != 1 {
		t.Fatalf("expected 1 review queue item, got %v", reviewResp["count"])
	}
}

func buildV22HandlerForTest(db *pgxpool.Pool) *adminapi.V22MultimodalHandler {
	taskRepo := postgres.NewMultimodalTaskRepo(db)
	taskInputRepo := postgres.NewMultimodalTaskInputRepo(db)
	resultRepo := postgres.NewMultimodalResultRepo(db)
	resultOutputRepo := postgres.NewMultimodalResultOutputRepo(db)
	normalizedRepo := postgres.NewNormalizedMultimodalResultRepo(db)
	reviewQueueRepo := postgres.NewMultimodalReviewQueueRepo(db)
	downstreamRepo := postgres.NewMultimodalDownstreamHandoffRepo(db)

	markRunningUC := &usecase.MarkMultimodalTaskRunningUseCase{Tasks: taskRepo}
	markSucceededUC := &usecase.MarkMultimodalTaskSucceededUseCase{Tasks: taskRepo}
	markReviewRequiredUC := &usecase.MarkMultimodalTaskReviewRequiredUseCase{Tasks: taskRepo}
	markFailedSoftUC := &usecase.MarkMultimodalTaskFailedSoftUseCase{Tasks: taskRepo}
	markBlockedPolicyUC := &usecase.MarkMultimodalTaskBlockedPolicyUseCase{Tasks: taskRepo}
	markSkippedBudgetUC := &usecase.MarkMultimodalTaskSkippedBudgetUseCase{Tasks: taskRepo}

	budgetGateUC := &usecase.EvaluateV22BudgetGateUseCase{
		Tasks:              taskRepo,
		Gate:               &worker.ConventionBudgetGate{},
		MarkSkippedBudget:  markSkippedBudgetUC,
		MarkReviewRequired: markReviewRequiredUC,
	}

	policyGateUC := &usecase.EvaluateV22PolicyGateUseCase{
		Tasks:              taskRepo,
		Gate:               &worker.ConventionPolicyGate{},
		MarkBlockedPolicy:  markBlockedPolicyUC,
		MarkReviewRequired: markReviewRequiredUC,
	}

	normalizeUC := &usecase.NormalizeV22MultimodalResultUseCase{
		Results:    resultRepo,
		Tasks:      taskRepo,
		Normalized: normalizedRepo,
	}

	enqueueReviewUC := &usecase.EnqueueV22MultimodalReviewUseCase{
		ReviewQueue: reviewQueueRepo,
		Normalized:  normalizedRepo,
	}

	downstreamUC := &usecase.CreateV22DownstreamHandoffUseCase{
		Downstream: downstreamRepo,
		Normalized: normalizedRepo,
	}

	executeOCRUC := &usecase.ExecuteV22OCRTaskUseCase{
		Tasks:               taskRepo,
		BudgetGate:          budgetGateUC,
		PolicyGate:          policyGateUC,
		MarkRunning:         markRunningUC,
		MarkSucceeded:       markSucceededUC,
		MarkReviewRequired:  markReviewRequiredUC,
		MarkFailedSoft:      markFailedSoftUC,
		RegisterResult:      &usecase.RegisterMultimodalResultUseCase{Tasks: taskRepo, Results: resultRepo},
		AttachResultOutputs: &usecase.AttachMultimodalResultOutputsUseCase{Results: resultRepo, ResultOutputs: resultOutputRepo},
		NormalizeResult:     normalizeUC,
		EnqueueReview:       enqueueReviewUC,
		DownstreamHandoff:   downstreamUC,
		OCRPort:             &worker.StubOCRAdapter{},
	}

	executeVisionUC := &usecase.ExecuteV22VisionTaskUseCase{
		Tasks:               taskRepo,
		BudgetGate:          budgetGateUC,
		PolicyGate:          policyGateUC,
		MarkRunning:         markRunningUC,
		MarkSucceeded:       markSucceededUC,
		MarkReviewRequired:  markReviewRequiredUC,
		MarkFailedSoft:      markFailedSoftUC,
		RegisterResult:      &usecase.RegisterMultimodalResultUseCase{Tasks: taskRepo, Results: resultRepo},
		AttachResultOutputs: &usecase.AttachMultimodalResultOutputsUseCase{Results: resultRepo, ResultOutputs: resultOutputRepo},
		NormalizeResult:     normalizeUC,
		EnqueueReview:       enqueueReviewUC,
		DownstreamHandoff:   downstreamUC,
		VisionPort:          &worker.StubVisionAdapter{},
	}

	return &adminapi.V22MultimodalHandler{
		CreateTaskUC:    &usecase.CreateMultimodalTaskUseCase{Tasks: taskRepo},
		AttachInputsUC:  &usecase.AttachMultimodalTaskInputsUseCase{Tasks: taskRepo, TaskInputs: taskInputRepo},
		ExecuteOCRUC:    executeOCRUC,
		ExecuteVisionUC: executeVisionUC,
		TaskRepo:        taskRepo,
		ResultRepo:      resultRepo,
		ReviewQueueRepo: reviewQueueRepo,
		NormalizedRepo:  normalizedRepo,
		DownstreamRepo:  downstreamRepo,
	}
}

func createTaskViaAPI(t *testing.T, baseURL string, req map[string]any) int64 {
	t.Helper()

	resp := postJSONMap(t, baseURL+"/v22/multimodal/tasks", req)
	taskMap := resp["task"].(map[string]any)
	return int64(taskMap["ID"].(float64))
}

func postJSONMap(t *testing.T, url string, req map[string]any) map[string]any {
	t.Helper()

	b, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	httpReq, err := http.NewRequestWithContext(context.Background(), http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-Trace-Id", "trace_test_http")

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		t.Fatalf("http do: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		t.Fatalf("unexpected status=%d body=%s", resp.StatusCode, string(body))
	}

	var out map[string]any
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("unmarshal response: %v body=%s", err, string(body))
	}
	return out
}

func getJSONMap(t *testing.T, url string) map[string]any {
	t.Helper()

	httpReq, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	httpReq.Header.Set("X-Trace-Id", "trace_test_http_get")

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		t.Fatalf("http do: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		t.Fatalf("unexpected status=%d body=%s", resp.StatusCode, string(body))
	}

	var out map[string]any
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("unmarshal response: %v body=%s", err, string(body))
	}
	return out
}
