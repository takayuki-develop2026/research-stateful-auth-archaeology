package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"time"
)

// RegisterTextEvidenceAssetV18 creates an evidence_assets row via evidence_register_v18 and returns evidence_assets.id (BIGINT).
func RegisterTextEvidenceAssetV18(
	ctx context.Context,
	db *DB,
	projectID string,
	traceID string,
	actorType string, // system|user|service
	actorID string,
	sourceKind string, // must satisfy evidence_assets_source_kind_ck: pisag_fetch|upload|webhook|generated|import
	sourceURI string,
	content string,
	idempotencyKey string,
) (int64, error) {
	sum := sha256.Sum256([]byte(content))
	sha := hex.EncodeToString(sum[:])

	// evidence_register_v18 -> evidence_ref uuid
	var evidenceRef string
	var foundExisting bool
	err := db.Pool.QueryRow(ctx, `
		SELECT evidence_ref::text, found_existing
		FROM evidence_register_v18(
			$1,$2,$3,$4,
			$5,$6,$7,$8,
			$9,$10,$11,$12,$13,$14
		)
	`, projectID,
		traceID,
		actorType,
		actorID,
		"text",
		"text/plain",
		sourceKind,
		sourceURI,
		sha,
		int64(len(content)),
		"en",
		"standard",
		time.Now().UTC().Add(24*time.Hour),
		idempotencyKey,
	).Scan(&evidenceRef, &foundExisting)
	if err != nil {
		return 0, err
	}
	_ = foundExisting

	// lookup evidence_assets.id by (project_id, evidence_ref)
	var assetID int64
	err = db.Pool.QueryRow(ctx, `
		SELECT id
		FROM evidence_assets
		WHERE project_id=$1 AND evidence_ref=$2::uuid
	`, projectID, evidenceRef).Scan(&assetID)

	if err != nil {
		return 0, err
	}
	return assetID, nil
}