package run

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

// sha256Hex returns lowercase hex sha256.
func sha256Hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// Canonicalize trims and normalizes whitespace for deterministic keys.
func Canonicalize(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	s = strings.Join(strings.Fields(s), " ")
	return s
}

// SpanKey = sha256(trace_id + "|" + service + "|" + operation + "|" + started_at_utc_rfc3339nano)
func SpanKey(traceID, service, operation string, startedAtUTC time.Time) string {
	base := fmt.Sprintf("%s|%s|%s|%s",
		Canonicalize(traceID),
		Canonicalize(service),
		Canonicalize(operation),
		startedAtUTC.UTC().Format(time.RFC3339Nano),
	)
	return sha256Hex(base)
}

// DimensionsKey = sha256(canonical_dimensions_text)
// IMPORTANT: dimensions must be canonicalized string, not JSON.
func DimensionsKey(canonicalDimensionsText string) string {
	return sha256Hex(Canonicalize(canonicalDimensionsText))
}

// EvaluationKey = sha256(project_id + "|" + slo_id + "|" + window_start + "|" + window_end)
func EvaluationKey(projectID string, sloID int64, windowStartUTC, windowEndUTC time.Time) string {
	base := fmt.Sprintf("%s|%d|%s|%s",
		Canonicalize(projectID),
		sloID,
		windowStartUTC.UTC().Format(time.RFC3339Nano),
		windowEndUTC.UTC().Format(time.RFC3339Nano),
	)
	return sha256Hex(base)
}

// IncidentKey = sha256(project_id + "|" + incident_type + "|" + root_trace_id + "|" + window_start + "|" + window_end)
// rootTraceID can be empty for manual incidents, but include placeholder to keep deterministic.
func IncidentKey(projectID, incidentType, rootTraceID string, windowStartUTC, windowEndUTC time.Time) string {
	rt := Canonicalize(rootTraceID)
	if rt == "" {
		rt = "-"
	}
	base := fmt.Sprintf("%s|%s|%s|%s|%s",
		Canonicalize(projectID),
		Canonicalize(incidentType),
		rt,
		windowStartUTC.UTC().Format(time.RFC3339Nano),
		windowEndUTC.UTC().Format(time.RFC3339Nano),
	)
	return sha256Hex(base)
}

// ProposalKey = sha256(project_id + "|" + incident_id + "|" + proposal_type + "|" + primary_target_key + "|" + created_bucket)
// createdBucket should be coarse (e.g. 10min bucket) to avoid noisy dupes.
func ProposalKey(projectID string, incidentID int64, proposalType, primaryTargetKey string, createdAtUTC time.Time, bucketMinutes int) string {
	if bucketMinutes <= 0 {
		bucketMinutes = 10
	}
	// floor to bucket
	t := createdAtUTC.UTC()
	floored := t.Truncate(time.Duration(bucketMinutes) * time.Minute)

	pt := Canonicalize(primaryTargetKey)
	if pt == "" {
		pt = "-"
	}
	base := fmt.Sprintf("%s|%d|%s|%s|%s",
		Canonicalize(projectID),
		incidentID,
		Canonicalize(proposalType),
		pt,
		floored.Format(time.RFC3339Nano),
	)
	return sha256Hex(base)
}

// ActionKey = sha256(project_id + "|" + proposal_id + "|" + apply_attempt)
// applyAttempt starts from 1 and increments when re-applying.
func ActionKey(projectID string, proposalID int64, applyAttempt int64) string {
	if applyAttempt <= 0 {
		applyAttempt = 1
	}
	base := fmt.Sprintf("%s|%d|%d",
		Canonicalize(projectID),
		proposalID,
		applyAttempt,
	)
	return sha256Hex(base)
}
