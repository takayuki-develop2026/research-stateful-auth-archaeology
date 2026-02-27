package postgres

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"example.com/pisag_go/run"
)

type LinksV18Repository struct{ db *sql.DB }

func NewLinksV18Repository(db *sql.DB) *LinksV18Repository {
	return &LinksV18Repository{db: db}
}

func (r *LinksV18Repository) AddRunEvidenceLink(
	ctx context.Context,
	projectID, runID, evidenceRef, role, idempotencyKey string,
) (run.LinkAddResult, error) {
	projectID = strings.TrimSpace(projectID)
	runID = strings.TrimSpace(runID)
	evidenceRef = strings.TrimSpace(evidenceRef)
	role = strings.TrimSpace(role)
	idem := strings.TrimSpace(idempotencyKey)

	if projectID == "" || runID == "" || evidenceRef == "" || role == "" || idem == "" {
		return run.LinkAddResult{}, errors.New("project_id/run_id/evidence_ref/role/idempotency_key are required")
	}

	const q = `
SELECT link_id, found_existing
FROM public.run_evidence_link_add_v18(
  $1::varchar,
  $2::uuid,
  $3::uuid,
  $4::varchar,
  $5::text
);
`
	var out run.LinkAddResult
	if err := r.db.QueryRowContext(ctx, q, projectID, runID, evidenceRef, role, idem).Scan(&out.LinkID, &out.FoundExisting); err != nil {
		return run.LinkAddResult{}, err
	}
	return out, nil
}

func (r *LinksV18Repository) AddRunArtifactLink(
	ctx context.Context,
	projectID, runID, artifactRef, role, idempotencyKey string,
) (run.LinkAddResult, error) {
	projectID = strings.TrimSpace(projectID)
	runID = strings.TrimSpace(runID)
	artifactRef = strings.TrimSpace(artifactRef)
	role = strings.TrimSpace(role)
	idem := strings.TrimSpace(idempotencyKey)

	if projectID == "" || runID == "" || artifactRef == "" || role == "" || idem == "" {
		return run.LinkAddResult{}, errors.New("project_id/run_id/artifact_ref/role/idempotency_key are required")
	}

	const q = `
SELECT link_id, found_existing
FROM public.run_artifact_link_add_v18(
  $1::varchar,
  $2::uuid,
  $3::uuid,
  $4::varchar,
  $5::text
);
`
	var out run.LinkAddResult
	if err := r.db.QueryRowContext(ctx, q, projectID, runID, artifactRef, role, idem).Scan(&out.LinkID, &out.FoundExisting); err != nil {
		return run.LinkAddResult{}, err
	}
	return out, nil
}

func (r *LinksV18Repository) AddArtifactEvidenceLink(
	ctx context.Context,
	projectID, artifactRef, evidenceRef, role, idempotencyKey string,
) (run.LinkAddResult, error) {
	projectID = strings.TrimSpace(projectID)
	artifactRef = strings.TrimSpace(artifactRef)
	evidenceRef = strings.TrimSpace(evidenceRef)
	role = strings.TrimSpace(role)
	idem := strings.TrimSpace(idempotencyKey)

	if projectID == "" || artifactRef == "" || evidenceRef == "" || role == "" || idem == "" {
		return run.LinkAddResult{}, errors.New("project_id/artifact_ref/evidence_ref/role/idempotency_key are required")
	}

	const q = `
SELECT link_id, found_existing
FROM public.artifact_evidence_link_add_v18(
  $1::varchar,
  $2::uuid,
  $3::uuid,
  $4::varchar,
  $5::text
);
`
	var out run.LinkAddResult
	if err := r.db.QueryRowContext(ctx, q, projectID, artifactRef, evidenceRef, role, idem).Scan(&out.LinkID, &out.FoundExisting); err != nil {
		return run.LinkAddResult{}, err
	}
	return out, nil
}
