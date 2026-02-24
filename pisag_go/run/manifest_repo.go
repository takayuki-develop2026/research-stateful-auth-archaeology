package run

import "context"

// EvidenceManifestRepo persists manifest + links.
// v4.5: one manifest per run_id (unique).
type EvidenceManifestRepo interface {
	// CreateOrGetBuilding returns the building manifest for a run.
	// If already exists, returns existing (status may be building/complete).
	CreateOrGetBuilding(ctx context.Context, runID string, traceID string) (EvidenceManifest, error)

	// AppendLink inserts a link idempotently (manifest_id, kind, asset_sha256 unique).
	AppendLink(ctx context.Context, link EvidenceLink) error

	// MarkComplete sets status=complete and fixes manifest_hash.
	// Should be idempotent: if already complete with same hash, treat as success.
	MarkComplete(ctx context.Context, manifestID string, manifestHash string) error

	// (Optional helper) ListLinks can be used by builder to compute hash from DB state.
	// You can implement it now or later; if omitted, builder can hash from in-memory list.
	ListLinks(ctx context.Context, manifestID string) ([]EvidenceLink, error)
}