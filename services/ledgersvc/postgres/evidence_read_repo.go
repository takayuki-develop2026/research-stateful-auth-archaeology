package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type EvidenceReadRepo struct {
	pool *pgxpool.Pool
}

func NewEvidenceReadRepo(pool *pgxpool.Pool) *EvidenceReadRepo {
	return &EvidenceReadRepo{pool: pool}
}

type EvidenceAssetRow struct {
	ID              int64      `json:"id"`
	ProjectID        string     `json:"project_id"`
	EvidenceRef      string     `json:"evidence_ref"` // uuid string
	MediaType        string     `json:"media_type"`
	SourceKind       string     `json:"source_kind"`
	SourceURI        *string    `json:"source_uri,omitempty"`
	ContentSHA256    string     `json:"content_sha256"`
	ContentLength    int64      `json:"content_length"`
	MimeType         string     `json:"mime_type"`
	Language         *string    `json:"language,omitempty"`
	RetentionPolicy  string     `json:"retention_policy"`
	ExpiresAtUTC     *time.Time `json:"expires_at_utc,omitempty"`
	Status           string     `json:"status"`
	CreatedByType    string     `json:"created_by_type"`
	CreatedByID      *string    `json:"created_by_id,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

func (r *EvidenceReadRepo) GetByRef(ctx context.Context, projectID, evidenceRef string) (*EvidenceAssetRow, error) {
	if strings.TrimSpace(projectID) == "" || strings.TrimSpace(evidenceRef) == "" {
		return nil, fmt.Errorf("%w: project_id/evidence_ref required", ErrReadInvalidArgument)
	}

	const q = `
SELECT
  id,
  project_id,
  evidence_ref::text,
  media_type,
  source_kind,
  source_uri,
  content_sha256,
  content_length,
  mime_type,
  language,
  retention_policy,
  expires_at_utc,
  status,
  created_by_type,
  created_by_id,
  created_at,
  updated_at
FROM public.evidence_assets
WHERE project_id = $1::varchar(26) AND evidence_ref = $2::uuid
LIMIT 1;
`
	var row EvidenceAssetRow
	err := r.pool.QueryRow(ctx, q, projectID, evidenceRef).Scan(
		&row.ID,
		&row.ProjectID,
		&row.EvidenceRef,
		&row.MediaType,
		&row.SourceKind,
		&row.SourceURI,
		&row.ContentSHA256,
		&row.ContentLength,
		&row.MimeType,
		&row.Language,
		&row.RetentionPolicy,
		&row.ExpiresAtUTC,
		&row.Status,
		&row.CreatedByType,
		&row.CreatedByID,
		&row.CreatedAt,
		&row.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrReadNotFound
		}
		return nil, err
	}
	return &row, nil
}