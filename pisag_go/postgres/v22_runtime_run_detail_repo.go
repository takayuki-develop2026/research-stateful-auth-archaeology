package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	run "example.com/pisag_go/run"
)

type RuntimeRunDetailRepo struct {
	db *pgxpool.Pool
}

func NewRuntimeRunDetailRepo(db *pgxpool.Pool) *RuntimeRunDetailRepo {
	return &RuntimeRunDetailRepo{db: db}
}

func (r *RuntimeRunDetailRepo) GetRuntimeRunDetail(ctx context.Context, projectID string, taskID int64) (run.RuntimeRunDetail, error) {
	if r == nil || r.db == nil {
		return run.RuntimeRunDetail{}, fmt.Errorf("get runtime run detail: repo is nil")
	}
	if projectID == "" {
		return run.RuntimeRunDetail{}, fmt.Errorf("get runtime run detail: project_id is required")
	}
	if taskID <= 0 {
		return run.RuntimeRunDetail{}, fmt.Errorf("get runtime run detail: task_id is required")
	}

	task, err := r.getTask(ctx, projectID, taskID)
	if err != nil {
		return run.RuntimeRunDetail{}, fmt.Errorf("get runtime run detail task: %w", err)
	}

	modelRuns, err := r.getModelRuns(ctx, projectID, taskID)
	if err != nil {
		return run.RuntimeRunDetail{}, fmt.Errorf("get runtime run detail model runs: %w", err)
	}

	results, err := r.getResults(ctx, projectID, taskID)
	if err != nil {
		return run.RuntimeRunDetail{}, fmt.Errorf("get runtime run detail results: %w", err)
	}

	var normalized *run.RuntimeNormalizedSummary
	if len(results) > 0 {
		norm, err := r.getNormalized(ctx, projectID, results[0].ID)
		if err == nil {
			normalized = &norm
		}
	}

	var reviewItem *run.RuntimeReviewQueueSummary
	if normalized != nil {
		review, err := r.getReviewQueueItem(ctx, projectID, normalized.ID)
		if err == nil {
			reviewItem = &review
		}
	}

	downstreams, err := r.getDownstreamHandoffs(ctx, projectID, normalized)
	if err != nil {
		return run.RuntimeRunDetail{}, fmt.Errorf("get runtime run detail downstream handoffs: %w", err)
	}

	results = enrichResultsWithOCRText(modelRuns, results)

	evidenceRefs, err := r.getEvidenceRefs(ctx, projectID, task, results, normalized)
	if err != nil {
		return run.RuntimeRunDetail{}, fmt.Errorf("get runtime run detail evidence refs: %w", err)
	}

	detail := run.RuntimeRunDetail{
		Run:                buildRuntimeRunSummaryFromTask(task),
		Task:               task,
		ModelRuns:          modelRuns,
		Results:            results,
		NormalizedResult:   normalized,
		ReviewQueueItem:    reviewItem,
		DownstreamHandoffs: downstreams,
		EvidenceRefs:       evidenceRefs,
	}

	return detail, nil
}

func (r *RuntimeRunDetailRepo) getTask(ctx context.Context, projectID string, taskID int64) (run.RuntimeTaskSummary, error) {
	var t run.RuntimeTaskSummary

	var modelRunID *int64
	var softErrorEvidenceAssetID *int64
	var startedAt *time.Time
	var finishedAt *time.Time
	var createdAt time.Time
	var updatedAt time.Time

	err := r.db.QueryRow(ctx, `
		SELECT
			id,
			project_id,
			trace_id,
			run_id,
			task_key,
			task_type,
			pipeline_version,
			policy_version_str,
			input_hash,
			status,
			router_plan_evidence_asset_id,
			options_evidence_asset_id,
			model_run_id,
			soft_error_evidence_asset_id,
			started_at_utc,
			finished_at_utc,
			created_at_utc,
			updated_at_utc
		FROM public.multimodal_tasks
		WHERE project_id = $1
		  AND id = $2
	`, projectID, taskID).Scan(
		&t.ID,
		&t.ProjectID,
		&t.TraceID,
		&t.RunID,
		&t.TaskKey,
		&t.TaskType,
		&t.PipelineVersion,
		&t.PolicyVersionStr,
		&t.InputHash,
		&t.Status,
		&t.RouterPlanEvidenceAssetID,
		&t.OptionsEvidenceAssetID,
		&modelRunID,
		&softErrorEvidenceAssetID,
		&startedAt,
		&finishedAt,
		&createdAt,
		&updatedAt,
	)
	if err != nil {
		return run.RuntimeTaskSummary{}, err
	}

	t.ModelRunID = modelRunID
	t.SoftErrorEvidenceAssetID = softErrorEvidenceAssetID
	t.StartedAtUTC = formatTimePtr(startedAt)
	t.FinishedAtUTC = formatTimePtr(finishedAt)
	t.CreatedAtUTC = createdAt.UTC().Format(time.RFC3339)
	t.UpdatedAtUTC = updatedAt.UTC().Format(time.RFC3339)
	t.EngineSelection = map[string][]string{}
	t.Metadata = map[string]any{}

	return t, nil
}

func (r *RuntimeRunDetailRepo) getModelRuns(ctx context.Context, projectID string, taskID int64) ([]run.RuntimeModelRunSummary, error) {
	rows, err := r.db.Query(ctx, `
		SELECT
			id,
			task_id,
			project_id,
			capability,
			engine_kind,
			engine_version,
			provider,
			task_kind,
			status,
			input_hash,
			started_at,
			finished_at,
			latency_ms,
			token_usage_json::text,
			cost_estimate,
			metadata_json::text
		FROM public.model_runs
		WHERE project_id = $1
		  AND task_id = $2
		ORDER BY started_at ASC, id ASC
	`, projectID, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]run.RuntimeModelRunSummary, 0)
	for rows.Next() {
		var item run.RuntimeModelRunSummary
		var taskKind *string
		var finishedAt *time.Time
		var latencyMS *int64
		var tokenUsageRaw string
		var metadataRaw string
		var startedAt time.Time
		var costEstimate *float64

		if err := rows.Scan(
			&item.ID,
			&item.TaskID,
			&item.ProjectID,
			&item.Capability,
			&item.EngineKind,
			&item.EngineVersion,
			&item.Provider,
			&taskKind,
			&item.Status,
			&item.InputHash,
			&startedAt,
			&finishedAt,
			&latencyMS,
			&tokenUsageRaw,
			&costEstimate,
			&metadataRaw,
		); err != nil {
			return nil, err
		}

		item.TaskKind = taskKind
		item.StartedAt = startedAt.UTC().Format(time.RFC3339)
		item.FinishedAt = formatTimePtr(finishedAt)
		item.LatencyMS = latencyMS
		item.CostEstimate = costEstimate
		item.TokenUsage = parseJSONMap(tokenUsageRaw)
		item.Metadata = parseJSONMap(metadataRaw)

		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return out, nil
}

func (r *RuntimeRunDetailRepo) getResults(ctx context.Context, projectID string, taskID int64) ([]run.RuntimeResultSummary, error) {
	rows, err := r.db.Query(ctx, `
		SELECT
			id,
			task_id,
			project_id,
			trace_id,
			run_id,
			result_key,
			result_type,
			output_hash,
			payload_evidence_asset_id,
			confidence_evidence_asset_id,
			created_at_utc,
			updated_at_utc
		FROM public.multimodal_results
		WHERE project_id = $1
		  AND task_id = $2
		ORDER BY created_at_utc DESC, id DESC
	`, projectID, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]run.RuntimeResultSummary, 0)
	for rows.Next() {
		var item run.RuntimeResultSummary
		var confidenceEvidenceAssetID *int64
		var createdAt time.Time
		var updatedAt time.Time

		if err := rows.Scan(
			&item.ID,
			&item.TaskID,
			&item.ProjectID,
			&item.TraceID,
			&item.RunID,
			&item.ResultKey,
			&item.ResultType,
			&item.OutputHash,
			&item.PayloadEvidenceAssetID,
			&confidenceEvidenceAssetID,
			&createdAt,
			&updatedAt,
		); err != nil {
			return nil, err
		}

		item.ConfidenceEvidenceAssetID = confidenceEvidenceAssetID
		item.CreatedAtUTC = createdAt.UTC().Format(time.RFC3339)
		item.UpdatedAtUTC = updatedAt.UTC().Format(time.RFC3339)
		item.Metadata = map[string]any{}

		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return out, nil
}

func (r *RuntimeRunDetailRepo) getNormalized(ctx context.Context, projectID string, resultID int64) (run.RuntimeNormalizedSummary, error) {
	var item run.RuntimeNormalizedSummary
	var confidenceScore *float64
	var reviewPayload *int64
	var downstreamPayload *int64
	var createdAt time.Time
	var updatedAt time.Time

	err := r.db.QueryRow(ctx, `
		SELECT
			id,
			project_id,
			trace_id,
			run_id,
			task_id,
			result_id,
			normalized_kind,
			normalized_status,
			summary_text,
			confidence_score,
			reason_code,
			review_payload_evidence_asset_id,
			downstream_payload_evidence_asset_id,
			created_at_utc,
			updated_at_utc
		FROM public.normalized_multimodal_results
		WHERE project_id = $1
		  AND result_id = $2
		ORDER BY id DESC
		LIMIT 1
	`, projectID, resultID).Scan(
		&item.ID,
		&item.ProjectID,
		&item.TraceID,
		&item.RunID,
		&item.TaskID,
		&item.ResultID,
		&item.NormalizedKind,
		&item.NormalizedStatus,
		&item.SummaryText,
		&confidenceScore,
		&item.ReasonCode,
		&reviewPayload,
		&downstreamPayload,
		&createdAt,
		&updatedAt,
	)
	if err != nil {
		return run.RuntimeNormalizedSummary{}, err
	}

	item.ConfidenceScore = confidenceScore
	item.ReviewPayloadEvidenceAssetID = reviewPayload
	item.DownstreamPayloadEvidenceAssetID = downstreamPayload
	item.CreatedAtUTC = createdAt.UTC().Format(time.RFC3339)
	item.UpdatedAtUTC = updatedAt.UTC().Format(time.RFC3339)
	item.Metadata = map[string]any{}

	return item, nil
}

func (r *RuntimeRunDetailRepo) getReviewQueueItem(ctx context.Context, projectID string, normalizedResultID int64) (run.RuntimeReviewQueueSummary, error) {
	var item run.RuntimeReviewQueueSummary
	var createdAt time.Time
	var updatedAt time.Time

	err := r.db.QueryRow(ctx, `
		SELECT
			id,
			project_id,
			normalized_result_id,
			priority,
			status,
			reason_code,
			created_at_utc,
			updated_at_utc
		FROM public.multimodal_review_queue
		WHERE project_id = $1
		  AND normalized_result_id = $2
		ORDER BY id DESC
		LIMIT 1
	`, projectID, normalizedResultID).Scan(
		&item.ID,
		&item.ProjectID,
		&item.NormalizedResultID,
		&item.Priority,
		&item.Status,
		&item.ReasonCode,
		&createdAt,
		&updatedAt,
	)
	if err != nil {
		return run.RuntimeReviewQueueSummary{}, err
	}

	item.CreatedAtUTC = createdAt.UTC().Format(time.RFC3339)
	item.UpdatedAtUTC = updatedAt.UTC().Format(time.RFC3339)
	item.Metadata = map[string]any{}

	return item, nil
}

func (r *RuntimeRunDetailRepo) getDownstreamHandoffs(ctx context.Context, projectID string, normalized *run.RuntimeNormalizedSummary) ([]run.RuntimeDownstreamSummary, error) {
	if normalized == nil {
		return []run.RuntimeDownstreamSummary{}, nil
	}

	rows, err := r.db.Query(ctx, `
		SELECT
			id,
			project_id,
			normalized_result_id,
			destination_kind,
			handoff_status,
			reason_code,
			created_at_utc,
			updated_at_utc
		FROM public.multimodal_downstream_handoffs
		WHERE project_id = $1
		  AND normalized_result_id = $2
		ORDER BY created_at_utc ASC, id ASC
	`, projectID, normalized.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]run.RuntimeDownstreamSummary, 0)
	for rows.Next() {
		var item run.RuntimeDownstreamSummary
		var createdAt time.Time
		var updatedAt time.Time

		if err := rows.Scan(
			&item.ID,
			&item.ProjectID,
			&item.NormalizedResultID,
			&item.DestinationKind,
			&item.Status,
			&item.ReasonCode,
			&createdAt,
			&updatedAt,
		); err != nil {
			return nil, err
		}

		item.CreatedAtUTC = createdAt.UTC().Format(time.RFC3339)
		item.UpdatedAtUTC = updatedAt.UTC().Format(time.RFC3339)
		item.Metadata = map[string]any{}

		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return out, nil
}

func (r *RuntimeRunDetailRepo) getEvidenceRefs(
	ctx context.Context,
	projectID string,
	task run.RuntimeTaskSummary,
	results []run.RuntimeResultSummary,
	normalized *run.RuntimeNormalizedSummary,
) ([]run.RuntimeEvidenceRef, error) {
	ids := make([]int64, 0, 16)

	if task.RouterPlanEvidenceAssetID > 0 {
		ids = append(ids, task.RouterPlanEvidenceAssetID)
	}
	if task.OptionsEvidenceAssetID > 0 {
		ids = append(ids, task.OptionsEvidenceAssetID)
	}
	if task.SoftErrorEvidenceAssetID != nil && *task.SoftErrorEvidenceAssetID > 0 {
		ids = append(ids, *task.SoftErrorEvidenceAssetID)
	}

	resultIDs := make([]int64, 0, len(results))
	for _, res := range results {
		resultIDs = append(resultIDs, res.ID)

		if res.PayloadEvidenceAssetID > 0 {
			ids = append(ids, res.PayloadEvidenceAssetID)
		}
		if res.ConfidenceEvidenceAssetID != nil && *res.ConfidenceEvidenceAssetID > 0 {
			ids = append(ids, *res.ConfidenceEvidenceAssetID)
		}
	}

	if len(resultIDs) > 0 {
		outputEvidenceIDs, err := r.getResultOutputEvidenceIDs(ctx, projectID, resultIDs)
		if err != nil {
			return nil, err
		}
		ids = append(ids, outputEvidenceIDs...)
	}

	if normalized != nil {
		if normalized.ReviewPayloadEvidenceAssetID != nil && *normalized.ReviewPayloadEvidenceAssetID > 0 {
			ids = append(ids, *normalized.ReviewPayloadEvidenceAssetID)
		}
		if normalized.DownstreamPayloadEvidenceAssetID != nil && *normalized.DownstreamPayloadEvidenceAssetID > 0 {
			ids = append(ids, *normalized.DownstreamPayloadEvidenceAssetID)
		}
	}

	uniq := dedupeInt64(ids)
	if len(uniq) == 0 {
		return []run.RuntimeEvidenceRef{}, nil
	}

	rows, err := r.db.Query(ctx, `
		SELECT
			id,
			kind,
			content_sha256,
			content_length,
			created_at
		FROM public.evidence_assets
		WHERE project_id = $1
		  AND id = ANY($2)
		ORDER BY id ASC
	`, projectID, uniq)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]run.RuntimeEvidenceRef, 0)
	for rows.Next() {
		var item run.RuntimeEvidenceRef
		var createdAt time.Time

		if err := rows.Scan(
			&item.ID,
			&item.Kind,
			&item.SHA256,
			&item.Bytes,
			&createdAt,
		); err != nil {
			return nil, err
		}

		item.CreatedAt = createdAt.UTC().Format(time.RFC3339)
		item.Metadata = map[string]any{}

		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return out, nil
}

func (r *RuntimeRunDetailRepo) getResultOutputEvidenceIDs(ctx context.Context, projectID string, resultIDs []int64) ([]int64, error) {
	if len(resultIDs) == 0 {
		return []int64{}, nil
	}

	rows, err := r.db.Query(ctx, `
		SELECT evidence_id
		FROM public.multimodal_result_outputs
		WHERE project_id = $1
		  AND result_id = ANY($2)
		ORDER BY id ASC
	`, projectID, resultIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]int64, 0)
	for rows.Next() {
		var evidenceID int64
		if err := rows.Scan(&evidenceID); err != nil {
			return nil, err
		}
		out = append(out, evidenceID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return out, nil
}

func enrichResultsWithOCRText(modelRuns []run.RuntimeModelRunSummary, results []run.RuntimeResultSummary) []run.RuntimeResultSummary {
	if len(results) == 0 || len(modelRuns) == 0 {
		return results
	}

	ocrText := ""
	ocrPreview := ""
	var conf any = nil

	for _, mr := range modelRuns {
		meta := mr.Metadata
		if len(meta) == 0 {
			continue
		}

		serviceMeta, _ := meta["service_meta"].(map[string]any)
		if serviceMeta == nil {
			continue
		}

		if ocrText == "" {
			if v, ok := serviceMeta["ocr_text"].(string); ok && strings.TrimSpace(v) != "" {
				ocrText = strings.TrimSpace(v)
			}
		}
		if ocrPreview == "" {
			if v, ok := serviceMeta["ocr_text_preview"].(string); ok && strings.TrimSpace(v) != "" {
				ocrPreview = strings.TrimSpace(v)
			}
		}
		if conf == nil {
			if v, ok := serviceMeta["avg_confidence"]; ok {
				conf = v
			}
			if v, ok := serviceMeta["avg_confidence_normalized"]; ok {
				conf = v
			}
		}
	}

	if ocrText == "" && ocrPreview == "" {
		return results
	}

	for i := range results {
		if results[i].Metadata == nil {
			results[i].Metadata = map[string]any{}
		}
		results[i].Metadata["ocr_text"] = ocrText
		results[i].Metadata["ocr_text_preview"] = ocrPreview
		if conf != nil {
			results[i].Metadata["confidence_from_model_run"] = conf
		}
	}

	return results
}

func buildRuntimeRunSummaryFromTask(task run.RuntimeTaskSummary) run.RuntimeRunSummary {
	return run.RuntimeRunSummary{
		ID:              task.ID,
		ProjectID:       task.ProjectID,
		TraceID:         task.TraceID,
		RunID:           task.RunID,
		PipelineVersion: task.PipelineVersion,
		Status:          task.Status,
		StartedAt:       task.StartedAtUTC,
		FinishedAt:      task.FinishedAtUTC,
		ErrorCode:       nil,
		ErrorMessage:    nil,
		Metadata:        map[string]any{},
	}
}

func parseJSONMap(raw string) map[string]any {
	if raw == "" {
		return map[string]any{}
	}
	var v map[string]any
	if err := json.Unmarshal([]byte(raw), &v); err != nil || v == nil {
		return map[string]any{}
	}
	return v
}

func formatTimePtr(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.UTC().Format(time.RFC3339)
	return &s
}

func dedupeInt64(in []int64) []int64 {
	if len(in) == 0 {
		return in
	}
	seen := map[int64]struct{}{}
	out := make([]int64, 0, len(in))
	for _, v := range in {
		if v <= 0 {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}