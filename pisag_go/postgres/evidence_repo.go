package postgres

import (
	"context"
	"database/sql"

	"example.com/pisag_go/run"
)

type EvidenceRepository struct{ db *sql.DB }

func NewEvidenceRepository(db *sql.DB) *EvidenceRepository {
	return &EvidenceRepository{db: db}
}

func (r *EvidenceRepository) InsertEvidence(ctx context.Context, ev run.EvidenceAsset) error {
	_, err := r.db.ExecContext(ctx, `
INSERT INTO run_evidence_assets
(run_id, trace_id, kind, content_type, byte_size, sha256, final_url, stored_path)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
ON CONFLICT (run_id, kind, sha256) DO NOTHING
`, ev.RunID, ev.TraceID, ev.Kind, ev.ContentType, ev.ByteSize, ev.SHA256, ev.FinalURL, ev.StoredPath)
	return err
}