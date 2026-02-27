package postgres

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"example.com/pisag_go/run"
)

type ArtifactV18Repository struct{ db *sql.DB }

func NewArtifactV18Repository(db *sql.DB) *ArtifactV18Repository {
	return &ArtifactV18Repository{db: db}
}

// Register calls public.artifact_register_v18(...) and returns (artifact_ref, found_existing).
func (r *ArtifactV18Repository) Register(ctx context.Context, in run.ArtifactRegisterInput) (run.ArtifactRegisterResult, error) {
	projectID := strings.TrimSpace(in.ProjectID)
	if projectID == "" {
		return run.ArtifactRegisterResult{}, errors.New("project_id is required")
	}
	artifactType := strings.TrimSpace(in.ArtifactType)
	if artifactType == "" {
		return run.ArtifactRegisterResult{}, errors.New("artifact_type is required")
	}
	schema := strings.TrimSpace(in.SchemaVersion)
	if schema == "" {
		return run.ArtifactRegisterResult{}, errors.New("schema_version is required")
	}
	if in.ContentLength < 0 {
		return run.ArtifactRegisterResult{}, errors.New("content_length must be >= 0")
	}
	mime := strings.TrimSpace(in.MimeType)
	if mime == "" {
		return run.ArtifactRegisterResult{}, errors.New("mime_type is required")
	}
	status := strings.TrimSpace(in.Status)
	if status == "" {
		status = "active"
	}
	idem := strings.TrimSpace(in.IdempotencyKey)
	if idem == "" {
		return run.ArtifactRegisterResult{}, errors.New("idempotency_key is required")
	}

	var sha any = nil
	if in.ContentSHA256 != nil && strings.TrimSpace(*in.ContentSHA256) != "" {
		sha = strings.TrimSpace(*in.ContentSHA256)
	}

	const q = `
SELECT artifact_ref::text, found_existing
FROM public.artifact_register_v18(
  $1::varchar,    -- project_id
  $2::varchar,    -- artifact_type
  $3::varchar,    -- schema_version
  $4::text,       -- content_sha256 (nullable)
  $5::bigint,     -- content_length
  $6::varchar,    -- mime_type
  $7::varchar,    -- status
  $8::text        -- idempotency_key
);
`

	var artifactRef string
	var found bool

	if err := r.db.QueryRowContext(
		ctx, q,
		projectID,
		artifactType,
		schema,
		sha,
		in.ContentLength,
		mime,
		status,
		idem,
	).Scan(&artifactRef, &found); err != nil {
		return run.ArtifactRegisterResult{}, err
	}

	return run.ArtifactRegisterResult{
		ArtifactRef:   artifactRef,
		FoundExisting: found,
	}, nil
}
