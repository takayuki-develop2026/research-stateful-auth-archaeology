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

// ★変更：evidence id (bigint) を返す（worker SELECT禁止のため関数経由）
func (r *EvidenceRepository) InsertEvidence(ctx context.Context, ev run.EvidenceAsset) (int64, error) {
	var id int64
	err := r.db.QueryRowContext(ctx, `
SELECT public.run_evidence_asset_upsert_id(
  $1::uuid,
  $2::uuid,
  $3,
  $4,
  $5::integer,
  $6,
  $7,
  $8
) AS id;
`,
		ev.RunID,
		ev.TraceID,
		ev.Kind,
		ev.ContentType, // nil OK
		ev.ByteSize,
		ev.SHA256,
		ev.FinalURL,
		ev.StoredPath,
	).Scan(&id)
	return id, err
}