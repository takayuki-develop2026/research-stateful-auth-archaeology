package postgres

import (
	"context"
	"fmt"
	"strings"
)

// AppendComplianceEventV21 appends a compliance event (v21) and returns event_id.
// DB signature (confirmed):
//   public.compliance_event_append_v21(
//     p_project_id varchar,
//     p_trace_id text,
//     p_event_type text,
//     p_event_evidence_asset_id bigint,
//     p_primary_artifact_asset_id bigint
//   ) RETURNS TABLE(event_id bigint)
func AppendComplianceEventV21(
	ctx context.Context,
	db *DB,
	projectID string,
	traceID string,
	eventType string,
	eventEvidenceAssetID int64,
	primaryArtifactAssetID int64, // 0 => NULL
	_ string, // idempotencyKey ignored (DB fn has no idem arg)
) (eventID int64, foundExisting bool, err error) {
	projectID = strings.TrimSpace(projectID)
	traceID = strings.TrimSpace(traceID)
	eventType = strings.TrimSpace(eventType)

	if projectID == "" {
		return 0, false, fmt.Errorf("projectID is required")
	}
	if traceID == "" {
		return 0, false, fmt.Errorf("traceID is required")
	}
	if eventType == "" {
		return 0, false, fmt.Errorf("eventType is required")
	}
	if eventEvidenceAssetID <= 0 {
		return 0, false, fmt.Errorf("eventEvidenceAssetID must be > 0")
	}

	var pa any = nil
	if primaryArtifactAssetID > 0 {
		pa = primaryArtifactAssetID
	}

	// RETURNS TABLE(event_id bigint) => select first column
	err = db.Pool.QueryRow(ctx, `
		SELECT event_id
		FROM public.compliance_event_append_v21(
			$1::varchar,
			$2::text,
			$3::text,
			$4::bigint,
			$5::bigint
		)
	`, projectID, traceID, eventType, eventEvidenceAssetID, pa).Scan(&eventID)
	if err != nil {
		return 0, false, err
	}

	return eventID, false, nil
}

// HasComplianceEventV21ForTrace checks if a compliance event exists for (project_id, trace_id, event_type).
func HasComplianceEventV21ForTrace(
	ctx context.Context,
	db *DB,
	projectID string,
	traceID string,
	eventType string,
) (bool, error) {
	projectID = strings.TrimSpace(projectID)
	traceID = strings.TrimSpace(traceID)
	eventType = strings.TrimSpace(eventType)

	if projectID == "" {
		return false, fmt.Errorf("projectID is required")
	}
	if traceID == "" {
		return false, fmt.Errorf("traceID is required")
	}
	if eventType == "" {
		return false, fmt.Errorf("eventType is required")
	}

	var n int
	err := db.Pool.QueryRow(ctx, `
		SELECT COUNT(1)
		FROM public.compliance_events_v21
		WHERE project_id = $1
		  AND trace_id = $2
		  AND event_type = $3
	`, projectID, traceID, eventType).Scan(&n)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}