package postgres

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

type V13Repository struct{ db *sql.DB }

func NewV13Repository(db *sql.DB) *V13Repository { return &V13Repository{db: db} }

// ------------------------------
// Idempotency
// ------------------------------

type IdempotencyStartResult struct {
	IdempotencyID int64
	FoundExisting bool
}

func (r *V13Repository) IdempotencyStart(
	ctx context.Context,
	projectID string,
	scope string,
	idempotencyKey string,
	requestFingerprint string, // char(64) or ""
) (IdempotencyStartResult, error) {
	projectID = strings.TrimSpace(projectID)
	scope = strings.TrimSpace(scope)
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	requestFingerprint = strings.TrimSpace(requestFingerprint)

	if projectID == "" {
		return IdempotencyStartResult{}, errors.New("project_id is required")
	}
	if scope == "" {
		return IdempotencyStartResult{}, errors.New("scope is required")
	}
	if idempotencyKey == "" {
		return IdempotencyStartResult{}, errors.New("idempotency_key is required")
	}
	if requestFingerprint == "" {
		requestFingerprint = strings.Repeat("0", 64)
	}
	if len(requestFingerprint) != 64 {
		return IdempotencyStartResult{}, errors.New("request_fingerprint must be 64 chars (hex)")
	}

	const q = `
SELECT idempotency_id, found_existing
FROM public.idempotency_start_v13($1,$2,$3,$4::char(64));
`
	var out IdempotencyStartResult
	if err := r.db.QueryRowContext(ctx, q, projectID, scope, idempotencyKey, requestFingerprint).
		Scan(&out.IdempotencyID, &out.FoundExisting); err != nil {
		return IdempotencyStartResult{}, err
	}
	return out, nil
}

func (r *V13Repository) IdempotencyFinish(
	ctx context.Context,
	projectID string,
	idempotencyID int64,
	status string, // succeeded|review_required|failed
	resultSummary *string,
	resultEvidenceAssetID *int64,
	finishedAtUTC time.Time,
) error {
	projectID = strings.TrimSpace(projectID)
	status = strings.TrimSpace(status)

	if projectID == "" {
		return errors.New("project_id is required")
	}
	if idempotencyID <= 0 {
		return errors.New("idempotency_id is required")
	}
	if status == "" {
		return errors.New("status is required")
	}

	var summaryAny any = nil
	if resultSummary != nil && strings.TrimSpace(*resultSummary) != "" {
		s := strings.TrimSpace(*resultSummary)
		summaryAny = s
	}
	var evAny any = nil
	if resultEvidenceAssetID != nil && *resultEvidenceAssetID > 0 {
		evAny = *resultEvidenceAssetID
	}

	const q = `SELECT public.idempotency_finish_v13($1,$2,$3,$4,$5);`
	_, err := r.db.ExecContext(ctx, q,
		projectID,
		idempotencyID,
		status,
		summaryAny,
		evAny,
	)
	return err
}

// ------------------------------
// DLQ
// ------------------------------

func (r *V13Repository) DlqEnqueue(
	ctx context.Context,
	projectID string,
	runID *string, // uuid text or nil
	traceID string, // uuid text
	taskType string,
	source string, // queue|scheduler|webhook|manual
	correlationKey *string,
	payloadEvidenceAssetID int64,
	lastErrorEvidenceAssetID *int64,
) (int64, error) {
	projectID = strings.TrimSpace(projectID)
	traceID = strings.TrimSpace(traceID)
	taskType = strings.TrimSpace(taskType)
	source = strings.TrimSpace(source)

	if projectID == "" {
		return 0, errors.New("project_id is required")
	}
	if traceID == "" {
		return 0, errors.New("trace_id is required")
	}
	if taskType == "" {
		return 0, errors.New("task_type is required")
	}
	if source == "" {
		return 0, errors.New("source is required")
	}
	if payloadEvidenceAssetID <= 0 {
		return 0, errors.New("payload_evidence_asset_id is required")
	}

	var runAny any = nil
	if runID != nil && strings.TrimSpace(*runID) != "" {
		runAny = strings.TrimSpace(*runID)
	}

	var corrAny any = nil
	if correlationKey != nil && strings.TrimSpace(*correlationKey) != "" {
		s := strings.TrimSpace(*correlationKey)
		corrAny = s
	}

	var errEvAny any = nil
	if lastErrorEvidenceAssetID != nil && *lastErrorEvidenceAssetID > 0 {
		errEvAny = *lastErrorEvidenceAssetID
	}

	const q = `
SELECT public.dlq_enqueue_v13(
  $1,
  NULLIF($2,'')::uuid,
  ($3)::uuid,
  $4,
  $5,
  $6,
  $7::bigint,
  NULLIF($8::bigint,0)
) AS dlq_id;
`
	var dlqID int64
	// runAny can be nil; to keep placeholder stable, pass empty string when nil
	runStr := ""
	if runAny != nil {
		runStr = runAny.(string)
	}
	errEvVal := int64(0)
	if errEvAny != nil {
		errEvVal = errEvAny.(int64)
	}

	if err := r.db.QueryRowContext(ctx, q,
		projectID,
		runStr,
		traceID,
		taskType,
		source,
		corrAny,
		payloadEvidenceAssetID,
		errEvVal,
	).Scan(&dlqID); err != nil {
		return 0, err
	}
	return dlqID, nil
}

func (r *V13Repository) DlqMark(
	ctx context.Context,
	projectID string,
	dlqID int64,
	status string, // requeued|resolved|ignored
	resultErrorEvidenceAssetID *int64,
) error {
	projectID = strings.TrimSpace(projectID)
	status = strings.TrimSpace(status)

	if projectID == "" {
		return errors.New("project_id is required")
	}
	if dlqID <= 0 {
		return errors.New("dlq_id is required")
	}
	if status == "" {
		return errors.New("status is required")
	}

	var evAny any = nil
	if resultErrorEvidenceAssetID != nil && *resultErrorEvidenceAssetID > 0 {
		evAny = *resultErrorEvidenceAssetID
	}

	const q = `SELECT public.dlq_mark_v13($1,$2,$3,NULLIF($4::bigint,0));`
	evVal := int64(0)
	if evAny != nil {
		evVal = evAny.(int64)
	}
	_, err := r.db.ExecContext(ctx, q, projectID, dlqID, status, evVal)
	return err
}

// ------------------------------
// Compat Contracts
// ------------------------------

func (r *V13Repository) CompatContractInsert(
	ctx context.Context,
	projectID string,
	contractType string,
	contractVersion string,
	checksumSha256 string, // 64 hex
	artifactRef *string,
	diffSummary *string,
	detailEvidenceAssetID *int64,
) (int64, error) {
	projectID = strings.TrimSpace(projectID)
	contractType = strings.TrimSpace(contractType)
	contractVersion = strings.TrimSpace(contractVersion)
	checksumSha256 = strings.TrimSpace(checksumSha256)

	if projectID == "" {
		return 0, errors.New("project_id is required")
	}
	if contractType == "" {
		return 0, errors.New("contract_type is required")
	}
	if contractVersion == "" {
		return 0, errors.New("contract_version is required")
	}
	if len(checksumSha256) != 64 {
		return 0, errors.New("checksum_sha256 must be 64 chars")
	}

	var artAny any = nil
	if artifactRef != nil && strings.TrimSpace(*artifactRef) != "" {
		artAny = strings.TrimSpace(*artifactRef)
	}
	var diffAny any = nil
	if diffSummary != nil && strings.TrimSpace(*diffSummary) != "" {
		diffAny = strings.TrimSpace(*diffSummary)
	}
	var evAny any = nil
	if detailEvidenceAssetID != nil && *detailEvidenceAssetID > 0 {
		evAny = *detailEvidenceAssetID
	}

	const q = `
SELECT public.compat_contract_insert_v13(
  $1,$2,$3,$4::char(64),
  $5,$6,
  NULLIF($7::bigint,0)
) AS id;
`
	var id int64
	evVal := int64(0)
	if evAny != nil {
		evVal = evAny.(int64)
	}
	if err := r.db.QueryRowContext(ctx, q,
		projectID, contractType, contractVersion, checksumSha256,
		artAny, diffAny, evVal,
	).Scan(&id); err != nil {
		return 0, err
	}
	return id, nil
}

func (r *V13Repository) DlqEnqueueRunEvidence(
	ctx context.Context,
	projectID string,
	runID *string,
	traceID string,
	taskType string,
	source string,
	correlationKey *string,
	payloadRunEvidenceAssetID int64,
	lastErrorRunEvidenceAssetID *int64,
) (int64, error) {
	projectID = strings.TrimSpace(projectID)
	traceID = strings.TrimSpace(traceID)
	taskType = strings.TrimSpace(taskType)
	source = strings.TrimSpace(source)

	if projectID == "" {
		return 0, errors.New("project_id is required")
	}
	if traceID == "" {
		return 0, errors.New("trace_id is required")
	}
	if taskType == "" {
		return 0, errors.New("task_type is required")
	}
	if source == "" {
		return 0, errors.New("source is required")
	}
	if payloadRunEvidenceAssetID <= 0 {
		return 0, errors.New("payload_run_evidence_asset_id is required")
	}

	runStr := ""
	if runID != nil && strings.TrimSpace(*runID) != "" {
		runStr = strings.TrimSpace(*runID)
	}

	var corrAny any = nil
	if correlationKey != nil && strings.TrimSpace(*correlationKey) != "" {
		corrAny = strings.TrimSpace(*correlationKey)
	}

	errEvVal := int64(0)
	if lastErrorRunEvidenceAssetID != nil && *lastErrorRunEvidenceAssetID > 0 {
		errEvVal = *lastErrorRunEvidenceAssetID
	}

	const q = `
SELECT public.dlq_enqueue_run_evidence_v13(
  $1,
  NULLIF($2,'')::uuid,
  ($3)::uuid,
  $4,
  $5,
  $6,
  $7::bigint,
  NULLIF($8::bigint,0)
) AS dlq_id;
`
	var dlqID int64
	if err := r.db.QueryRowContext(ctx, q,
		projectID, runStr, traceID, taskType, source, corrAny,
		payloadRunEvidenceAssetID, errEvVal,
	).Scan(&dlqID); err != nil {
		return 0, err
	}
	return dlqID, nil
}
