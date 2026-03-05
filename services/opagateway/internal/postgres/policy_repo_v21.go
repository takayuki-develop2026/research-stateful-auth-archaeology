package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"
)

type PolicyDecisionAppendIn struct {
	ProjectID        string
	DecisionKey      string // 64 hex
	TraceID          string
	RunID            *string // uuid text (optional)
	SubjectType      string  // user|service|api_key
	SubjectID        string
	ActionKey        string
	ActionClass      string // high_risk|low_risk_read|low_risk_write
	PolicyVersionStr string
	Result           string // allow|deny|error
	InputHashSha256  string // 64 hex

	DecisionInputEvidenceAssetID  int64
	DecisionResultEvidenceAssetID int64
	ResourceEvidenceAssetID       int64
	ObligationsEvidenceAssetID     int64
	ReasonCodesEvidenceAssetID     int64
}

func decisionKey(projectID, traceID, actionKey, resourceKey, policyVersionStr string) string {
	s := projectID + "|" + traceID + "|" + actionKey + "|" + resourceKey + "|" + policyVersionStr
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func inputHashSha256(canonicalInput string) string {
	sum := sha256.Sum256([]byte(canonicalInput))
	return hex.EncodeToString(sum[:])
}

// AppendPolicyDecisionV21 calls policy_decision_append_v21 (EXECUTE ONLY) and returns decision_id.
func AppendPolicyDecisionV21(ctx context.Context, db *DB, in PolicyDecisionAppendIn) (decisionID int64, foundExisting bool, err error) {
	in.ProjectID = strings.TrimSpace(in.ProjectID)
	in.TraceID = strings.TrimSpace(in.TraceID)
	in.SubjectType = strings.TrimSpace(in.SubjectType)
	in.SubjectID = strings.TrimSpace(in.SubjectID)
	in.ActionKey = strings.TrimSpace(in.ActionKey)
	in.ActionClass = strings.TrimSpace(in.ActionClass)
	in.PolicyVersionStr = strings.TrimSpace(in.PolicyVersionStr)
	in.Result = strings.TrimSpace(in.Result)

	runID := ""
	if in.RunID != nil {
		runID = strings.TrimSpace(*in.RunID)
	}

	// run_id may be NULL; pass NULL if empty
	var runParam any = nil
	if runID != "" {
		runParam = runID
	}

	err = db.Pool.QueryRow(ctx, `
		SELECT decision_id, found_existing
		FROM policy_decision_append_v21(
			$1,$2,$3,$4::uuid,
			$5,$6,$7,$8,$9,$10,$11,
			$12,$13,$14,$15,$16
		)
	`, in.ProjectID,
		in.DecisionKey,
		in.TraceID,
		runParam,
		in.SubjectType,
		in.SubjectID,
		in.ActionKey,
		in.ActionClass,
		in.PolicyVersionStr,
		in.Result,
		in.InputHashSha256,
		in.DecisionInputEvidenceAssetID,
		in.DecisionResultEvidenceAssetID,
		in.ResourceEvidenceAssetID,
		in.ObligationsEvidenceAssetID,
		in.ReasonCodesEvidenceAssetID,
	).Scan(&decisionID, &foundExisting)

	return decisionID, foundExisting, err
}

// AppendAuditEventV18 is optional but recommended for CI "audit missing" detection.
// It calls audit_event_append_v18 (EXECUTE ONLY).
func AppendAuditEventV18(ctx context.Context, db *DB,
	projectID, traceID, action, actorType, actorID, targetType, targetID, result string,
	reason string, metaJSON string, idempotencyKey string,
) (auditEventID int64, foundExisting bool, err error) {
	// meta is jsonb; pass NULL or '{}' if unknown
	var meta any = nil
	m := strings.TrimSpace(metaJSON)
	if m != "" {
		meta = m
	}

	err = db.Pool.QueryRow(ctx, `
		SELECT audit_event_id, found_existing
		FROM audit_event_append_v18(
			$1,$2,NULL,$3,$4,$5,$6,$7,$8,$9,$10::jsonb,$11
		)
	`, projectID, traceID,
		action,
		actorType,
		actorID,
		targetType,
		targetID,
		result,
		reason,
		meta,
		idempotencyKey,
	).Scan(&auditEventID, &foundExisting)

	return auditEventID, foundExisting, err
}

// Helper for CI: find audits for a trace/action prefix
func HasAuditForTrace(ctx context.Context, db *DB, projectID, traceID, actionPrefix string) (bool, error) {
	var n int64
	err := db.Pool.QueryRow(ctx, `
		SELECT COUNT(1)
		FROM audit_events
		WHERE project_id=$1
		  AND trace_id=$2
		  AND action LIKE $3
	`, projectID, traceID, actionPrefix+"%").Scan(&n)
	return n > 0, err
}

func NowRFC3339NanoUTC() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}