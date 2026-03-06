package shared

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

func Sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// Deterministic object key (trace searchable)
func ObjectKeyV21(projectID string, createdAt time.Time, eventID int64, traceID string, eventType string) string {
	day := createdAt.UTC().Format("2006-01-02")
	eventType = sanitizePath(eventType)
	traceID = sanitizePath(traceID)
	projectID = sanitizePath(projectID)
	return fmt.Sprintf(
		"compliance/v21/project_id=%s/date=%s/event_id=%d/trace_id=%s/event_type=%s.json",
		projectID, day, eventID, traceID, eventType,
	)
}

// Idempotency key for v13
func IdemKeyV13(projectID string, eventID int64, sink string, objectKey string) string {
	raw := fmt.Sprintf("%s|%d|%s|%s",
		strings.TrimSpace(projectID),
		eventID,
		strings.TrimSpace(sink),
		strings.TrimSpace(objectKey),
	)
	return Sha256Hex(raw)
}

func sanitizePath(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "unknown"
	}
	s = strings.ReplaceAll(s, "/", "_")
	s = strings.ReplaceAll(s, " ", "_")
	return s
}