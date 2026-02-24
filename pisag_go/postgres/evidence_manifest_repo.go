package postgres

import (
	"context"
	"database/sql"
	"errors"
	"sort"
	"strings"
	"time"

	"example.com/pisag_go/run"
)

type EvidenceManifestRepository struct{ db *sql.DB }

func NewEvidenceManifestRepository(db *sql.DB) *EvidenceManifestRepository {
	return &EvidenceManifestRepository{db: db}
}

// CreateOrGetBuilding creates (or returns existing) manifest for a run_id.
// v4.5: run_id unique.
func (r *EvidenceManifestRepository) CreateOrGetBuilding(ctx context.Context, runID string, traceID string) (run.EvidenceManifest, error) {
	if strings.TrimSpace(runID) == "" {
		return run.EvidenceManifest{}, errors.New("run_id is required")
	}
	if strings.TrimSpace(traceID) == "" {
		return run.EvidenceManifest{}, errors.New("trace_id is required")
	}

	// まず INSERT を試す（競合したら既存を返す）
	const ins = `
INSERT INTO public.run_evidence_manifests (run_id, trace_id, status)
VALUES ($1::uuid, $2::uuid, 'building')
ON CONFLICT (run_id) DO NOTHING;
`
	if _, err := r.db.ExecContext(ctx, ins, runID, traceID); err != nil {
		return run.EvidenceManifest{}, err
	}

	// 既存を読む（building/complete どちらでも返す）
	const sel = `
SELECT manifest_id, run_id, trace_id, status, manifest_hash, created_at, updated_at
FROM public.run_evidence_manifests
WHERE run_id=$1::uuid
LIMIT 1;
`
	var out run.EvidenceManifest
	var mh sql.NullString
	if err := r.db.QueryRowContext(ctx, sel, runID).Scan(
		&out.ManifestID,
		&out.RunID,
		&out.TraceID,
		&out.Status,
		&mh,
		&out.CreatedAt,
		&out.UpdatedAt,
	); err != nil {
		return run.EvidenceManifest{}, err
	}
	if mh.Valid {
		out.ManifestHash = &mh.String
	}
	return out, nil
}

// AppendLink inserts a link idempotently (manifest_id, kind, asset_sha256 unique).
func (r *EvidenceManifestRepository) AppendLink(ctx context.Context, link run.EvidenceLink) error {
	if strings.TrimSpace(link.ManifestID) == "" {
		return errors.New("manifest_id is required")
	}
	if strings.TrimSpace(link.Kind) == "" {
		return errors.New("kind is required")
	}
	if strings.TrimSpace(link.AssetSHA256) == "" {
		return errors.New("asset_sha256 is required")
	}
	if link.ByteSize <= 0 {
		return errors.New("byte_size is required")
	}
	if strings.TrimSpace(link.FinalURL) == "" {
		return errors.New("final_url is required")
	}
	if strings.TrimSpace(link.StoredPath) == "" {
		return errors.New("stored_path is required")
	}

	const q = `
INSERT INTO public.run_evidence_links
(manifest_id, kind, asset_sha256, content_type, byte_size, final_url, stored_path)
VALUES ($1::uuid, $2, $3, $4, $5, $6, $7)
ON CONFLICT (manifest_id, kind, asset_sha256) DO NOTHING;
`
	_, err := r.db.ExecContext(ctx, q,
		link.ManifestID,
		link.Kind,
		link.AssetSHA256,
		link.ContentType,
		link.ByteSize,
		link.FinalURL,
		link.StoredPath,
	)
	return err
}

// ListLinks returns all links for the manifest.
// NOTE: hashing will be performed on a canonical ordering; we return raw rows,
// and caller may sort if needed. Here we already return in stable order.
func (r *EvidenceManifestRepository) ListLinks(ctx context.Context, manifestID string) ([]run.EvidenceLink, error) {
	if strings.TrimSpace(manifestID) == "" {
		return nil, errors.New("manifest_id is required")
	}

	const q = `
SELECT id, manifest_id, kind, asset_sha256, content_type, byte_size, final_url, stored_path, created_at
FROM public.run_evidence_links
WHERE manifest_id=$1::uuid
ORDER BY kind ASC, asset_sha256 ASC, id ASC;
`
	rows, err := r.db.QueryContext(ctx, q, manifestID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []run.EvidenceLink
	for rows.Next() {
		var l run.EvidenceLink
		var ct sql.NullString
		var created time.Time

		if err := rows.Scan(
			&l.ID,
			&l.ManifestID,
			&l.Kind,
			&l.AssetSHA256,
			&ct,
			&l.ByteSize,
			&l.FinalURL,
			&l.StoredPath,
			&created,
		); err != nil {
			return nil, err
		}
		l.CreatedAt = created
		if ct.Valid {
			l.ContentType = &ct.String
		}
		out = append(out, l)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// 念のため（DB ORDER で安定しているが、将来変更の保険）
	sort.Slice(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		if out[i].AssetSHA256 != out[j].AssetSHA256 {
			return out[i].AssetSHA256 < out[j].AssetSHA256
		}
		return out[i].ID < out[j].ID
	})

	return out, nil
}

// MarkComplete fixes manifest_hash and sets status=complete.
// Idempotent: if already complete with same hash => success.
func (r *EvidenceManifestRepository) MarkComplete(ctx context.Context, manifestID string, manifestHash string) error {
	if strings.TrimSpace(manifestID) == "" {
		return errors.New("manifest_id is required")
	}
	manifestHash = strings.TrimSpace(manifestHash)
	if manifestHash == "" {
		return errors.New("manifest_hash is required")
	}

	// 1) まず update してみる（building -> complete）
	const upd = `
UPDATE public.run_evidence_manifests
SET status='complete', manifest_hash=$2, updated_at=now()
WHERE manifest_id=$1::uuid
  AND (status <> 'complete' OR manifest_hash IS DISTINCT FROM $2);
`
	res, err := r.db.ExecContext(ctx, upd, manifestID, manifestHash)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n > 0 {
		return nil
	}

	// 2) すでに complete で同一hashならOK、違うなら競合としてエラー
	const sel = `
SELECT status, manifest_hash
FROM public.run_evidence_manifests
WHERE manifest_id=$1::uuid
LIMIT 1;
`
	var status string
	var mh sql.NullString
	if err := r.db.QueryRowContext(ctx, sel, manifestID).Scan(&status, &mh); err != nil {
		return err
	}
	if status == "complete" && mh.Valid && mh.String == manifestHash {
		return nil
	}
	return errors.New("manifest already complete with different hash (conflict)")
}