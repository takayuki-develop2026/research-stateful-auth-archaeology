package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// RegisterTextEvidenceAssetV18 registers a small evidence record via evidence_register_v18,
// then looks up evidence_assets.id by (project_id, evidence_ref).
//
// E条文: 本文は evidence に置く。DB SoT は参照だけ。
// created_by_type must satisfy evidence_assets_created_by_type_ck: system|user|service
// source_kind must satisfy evidence_assets_source_kind_ck: pisag_fetch|upload|webhook|generated|import
func RegisterTextEvidenceAssetV18(
	ctx context.Context,
	db *DB,
	projectID string,
	traceID string,
	createdByType string,
	createdByID string,
	sourceKind string,
	sourceURI string,
	content any, // string or JSON-able
	idempotencyKey string,
) (int64, error) {
	if db == nil || db.Pool == nil {
		return 0, fmt.Errorf("db required")
	}
	projectID = strings.TrimSpace(projectID)
	traceID = strings.TrimSpace(traceID)
	if projectID == "" {
		return 0, fmt.Errorf("projectID required")
	}
	if traceID == "" {
		return 0, fmt.Errorf("traceID required")
	}

	createdByType = strings.ToLower(strings.TrimSpace(createdByType))
	if createdByType == "" {
		createdByType = "service"
	}
	createdByID = strings.TrimSpace(createdByID)
	if createdByID == "" {
		createdByID = "wormexportersvc"
	}

	sourceKind = strings.TrimSpace(sourceKind)
	if sourceKind == "" {
		sourceKind = "generated"
	}
	sourceURI = strings.TrimSpace(sourceURI)
	if sourceURI == "" {
		sourceURI = "wormexportersvc://export_result"
	}

	// ---- serialize content
	var body []byte
	switch v := content.(type) {
	case string:
		body = []byte(v)
	default:
		b, err := json.Marshal(v)
		if err != nil {
			// fail-closed for evidence body: still store something deterministic
			body = []byte(`{"error":"marshal_failed"}`)
		} else {
			body = b
		}
	}

	sum := sha256.Sum256(body)
	sha := hex.EncodeToString(sum[:])

	// ---- evidence_register_v18 returns (evidence_ref uuid, found_existing bool)
	var evidenceRef string
	var foundExisting bool

	// media_type: text
	// mime_type: text/plain (or application/json if JSON)
	mime := "text/plain"
	if len(body) > 0 && body[0] == '{' {
		mime = "application/json"
	}

	err := db.Pool.QueryRow(ctx, `
		SELECT evidence_ref::text, found_existing
		FROM evidence_register_v18(
			$1,  -- project_id
			$2,  -- trace_id
			$3,  -- actor_type
			$4,  -- actor_id
			$5,  -- media_type
			$6,  -- mime_type
			$7,  -- source_kind
			$8,  -- source_uri
			$9,  -- content_sha256
			$10, -- content_length
			$11, -- language
			$12, -- retention_policy
			$13, -- expires_at_utc
			$14  -- idempotency_key
		)
	`,
		projectID,
		traceID,
		createdByType,
		createdByID,
		"text",
		mime,
		sourceKind,
		sourceURI,
		sha,
		int64(len(body)),
		"en",
		"standard",
		time.Now().UTC().Add(24*time.Hour),
		strings.TrimSpace(idempotencyKey),
	).Scan(&evidenceRef, &foundExisting)
	if err != nil {
		return 0, err
	}
	_ = foundExisting

	var assetID int64
	err = db.Pool.QueryRow(ctx, `
		SELECT id
		FROM evidence_assets
		WHERE project_id = $1
		  AND evidence_ref = $2::uuid
	`, projectID, evidenceRef).Scan(&assetID)
	if err != nil {
		return 0, err
	}
	return assetID, nil
}