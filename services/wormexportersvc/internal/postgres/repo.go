package postgres

import (
	"context"
	"fmt"
	"strings"
	"time"

	"services/wormexportersvc/internal/shared"

	"github.com/jackc/pgx/v5"
)

type Repo struct {
	db *DB
}

func NewRepo(db *DB) *Repo {
	return &Repo{db: db}
}

type claimCandidate struct {
	ev      shared.ComplianceEvent
	objKey  string
	idemKey string
}

// ClaimBatch selects candidates from compliance_events_v21 and claims them via idempotency_records_v13 (scope=worm_export_v21).
//
// ✅ Hardening points (完成版):
// - 2-phase: read all rows first, then do idempotency writes (avoid pgx "conn busy").
// - starvation fix: over-scan (limit*scanFactor) and stop after "claimed==limit".
// - stale reclaim: started_at older than (now - staleAfter) => reclaim by resetting started_at/updated_at.
// - optional reclaimFailed: failed/review_required => set started again (replay).
func (r *Repo) ClaimBatch(
	ctx context.Context,
	projectID string,
	limit int,
	sink string,
	staleAfter time.Duration,
	reclaimFailed bool,
) ([]shared.ComplianceEvent, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return nil, fmt.Errorf("projectID required")
	}
	if limit <= 0 {
		limit = 50
	}
	sink = strings.TrimSpace(sink)
	if sink == "" {
		sink = "localfile"
	}
	if staleAfter <= 0 {
		staleAfter = 5 * time.Minute
	}

	// ✅ starvation fix: over-scan
	const scanFactor = 20
	scanLimit := limit * scanFactor
	if scanLimit < limit {
		scanLimit = limit
	}

	now := time.Now().UTC()
	cutoff := now.Add(-staleAfter)

	tx, err := r.db.Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// ------------------------------------------------------------
	// Phase 1) Load + lock rows (over-scan)
	// ------------------------------------------------------------
	rows, err := tx.Query(ctx, `
		SELECT
			ce.id,
			ce.project_id,
			ce.trace_id,
			ce.event_type,
			ce.event_evidence_asset_id,
			ce.primary_artifact_asset_id,
			ce.created_at_utc
		FROM public.compliance_events_v21 ce
		WHERE ce.project_id = $1
		ORDER BY ce.id ASC
		FOR UPDATE SKIP LOCKED
		LIMIT $2
	`, projectID, scanLimit)
	if err != nil {
		return nil, err
	}

	cands := make([]claimCandidate, 0, scanLimit)
	for rows.Next() {
		var ev shared.ComplianceEvent
		var primary *int64
		if err := rows.Scan(
			&ev.ID,
			&ev.ProjectID,
			&ev.TraceID,
			&ev.EventType,
			&ev.EventEvidenceAssetID,
			&primary,
			&ev.CreatedAtUTC,
		); err != nil {
			rows.Close()
			return nil, err
		}
		ev.PrimaryArtifactID = primary

		objKey := shared.ObjectKeyV21(ev.ProjectID, ev.CreatedAtUTC, ev.ID, ev.TraceID, ev.EventType)
		idemKey := shared.IdemKeyV13(ev.ProjectID, ev.ID, sink, objKey)

		cands = append(cands, claimCandidate{
			ev:      ev,
			objKey:  objKey,
			idemKey: idemKey,
		})
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// ------------------------------------------------------------
	// Phase 2) Claim (idempotent) until claimed reaches limit
	// ------------------------------------------------------------
	claimed := make([]shared.ComplianceEvent, 0, limit)

	for _, c := range cands {
		if len(claimed) >= limit {
			break
		}

		// 2-1) Try insert started (new claim)
		var inserted bool
		err := tx.QueryRow(ctx, `
			INSERT INTO public.idempotency_records_v13(
				project_id, scope, idempotency_key,
				status, result_summary,
				started_at, created_at, updated_at
			)
			VALUES ($1, 'worm_export_v21', $2,
			        'started', NULL,
			        $3, $3, $3)
			ON CONFLICT (project_id, scope, idempotency_key)
			DO NOTHING
			RETURNING true
		`, c.ev.ProjectID, c.idemKey, now).Scan(&inserted)

		if err != nil && err != pgx.ErrNoRows {
			return nil, err
		}
		if inserted {
			claimed = append(claimed, c.ev)
			continue
		}

		// 2-2) Reclaim stale started: bump started_at/updated_at
		tag2, err := tx.Exec(ctx, `
			UPDATE public.idempotency_records_v13
			SET started_at = $3,
			    updated_at = $3
			WHERE project_id = $1
			  AND scope = 'worm_export_v21'
			  AND idempotency_key = $2
			  AND status = 'started'
			  AND started_at < $4
		`, c.ev.ProjectID, c.idemKey, now, cutoff)
		if err != nil {
			return nil, err
		}
		if tag2.RowsAffected() == 1 {
			claimed = append(claimed, c.ev)
			continue
		}

		// 2-3) Optional reclaim failed/review_required => started again
		if reclaimFailed {
			tag3, err := tx.Exec(ctx, `
				UPDATE public.idempotency_records_v13
				SET status = 'started',
				    result_summary = NULL,
				    started_at = $3,
				    finished_at = NULL,
				    updated_at = $3
				WHERE project_id = $1
				  AND scope = 'worm_export_v21'
				  AND idempotency_key = $2
				  AND status IN ('failed','review_required')
			`, c.ev.ProjectID, c.idemKey, now)
			if err != nil {
				return nil, err
			}
			if tag3.RowsAffected() == 1 {
				claimed = append(claimed, c.ev)
				continue
			}
		}
		// else: not claimable (already succeeded OR non-stale started OR reclaim disabled)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return claimed, nil
}

// MarkResult updates idempotency_records_v13 for the export attempt.
//
// ✅ “収束更新” (完成版):
// - ok=true  : started/failed/review_required -> succeeded に収束（succeeded は維持）
// - ok=false : started/review_required/failed -> failed に収束（succeeded は絶対に落とさない）
// - summaryMax: call-site policy (default 256)
func (r *Repo) MarkResult(
	ctx context.Context,
	projectID string,
	eventID int64,
	sink string,
	objectKey string,
	ok bool,
	summary string,
	summaryMax int,
) error {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return fmt.Errorf("projectID required")
	}
	sink = strings.TrimSpace(sink)
	if sink == "" {
		sink = "localfile"
	}
	if summaryMax <= 0 {
		summaryMax = 256
	}
	if len(summary) > summaryMax {
		summary = summary[:summaryMax]
	}

	idemKey := shared.IdemKeyV13(projectID, eventID, sink, objectKey)

	if ok {
		_, err := r.db.Pool.Exec(ctx, `
			UPDATE public.idempotency_records_v13
			SET status = 'succeeded',
			    result_summary = COALESCE(NULLIF($3,''), result_summary),
			    finished_at = COALESCE(finished_at, now()),
			    updated_at = now()
			WHERE project_id = $1
			  AND scope = 'worm_export_v21'
			  AND idempotency_key = $2
			  AND status IN ('started','failed','review_required','succeeded')
		`, projectID, idemKey, summary)
		return err
	}

	_, err := r.db.Pool.Exec(ctx, `
		UPDATE public.idempotency_records_v13
		SET status = CASE WHEN status = 'succeeded' THEN status ELSE 'failed' END,
		    result_summary = COALESCE(NULLIF($3,''), result_summary),
		    finished_at = CASE WHEN status = 'succeeded' THEN finished_at ELSE now() END,
		    updated_at = now()
		WHERE project_id = $1
		  AND scope = 'worm_export_v21'
		  AND idempotency_key = $2
		  AND status IN ('started','failed','review_required','succeeded')
	`, projectID, idemKey, summary)
	return err
}