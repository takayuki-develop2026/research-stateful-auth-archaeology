package postgres

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"example.com/pisag_go/run"
)

type PublishRepository struct{ db *sql.DB }

func NewPublishRepository(db *sql.DB) *PublishRepository {
	return &PublishRepository{db: db}
}

func (r *PublishRepository) CreateProposed(
	ctx context.Context,
	in run.PublishCommit,
) (out run.PublishCommit, foundExisting bool, err error) {
	if strings.TrimSpace(in.ProjectID) == "" {
		return run.PublishCommit{}, false, errors.New("project_id is required")
	}
	if strings.TrimSpace(in.CommitKey) == "" {
		return run.PublishCommit{}, false, errors.New("commit_key is required")
	}
	if strings.TrimSpace(in.ManifestID) == "" {
		return run.PublishCommit{}, false, errors.New("manifest_id is required")
	}
	if strings.TrimSpace(in.ManifestHash) == "" {
		return run.PublishCommit{}, false, errors.New("manifest_hash is required")
	}
	if strings.TrimSpace(in.TraceID) == "" {
		return run.PublishCommit{}, false, errors.New("trace_id is required")
	}
	if strings.TrimSpace(in.Target) == "" {
		in.Target = "catalog_v1"
	}
	if strings.TrimSpace(in.Status) == "" {
		in.Status = run.PublishStatusProposed
	}
	if len(in.MetaJSON) == 0 {
		in.MetaJSON = []byte(`{}`)
	}

	var runID any = nil
	if in.RunID != nil && strings.TrimSpace(*in.RunID) != "" {
		runID = strings.TrimSpace(*in.RunID)
	}

	// 1) Try insert idempotently.
	// If inserted, RETURNING gives the created row.
	const ins = `
INSERT INTO public.catalog_publish_commits
(project_id, commit_key, manifest_id, manifest_hash, run_id, trace_id, target, status, meta_json)
VALUES ($1, $2, $3::uuid, $4, $5::uuid, $6::uuid, $7, $8, $9::jsonb)
ON CONFLICT (project_id, commit_key) DO NOTHING
RETURNING
  commit_id,
  project_id,
  commit_key,
  manifest_id,
  manifest_hash,
  run_id,
  trace_id,
  target,
  status,
  error_code,
  error_message,
  meta_json,
  created_at,
  updated_at;
`
	var (
		commitID                 string
		projectID, commitKey     string
		manifestID, manifestHash string
		runIDOut                 sql.NullString
		traceID, target, status  string
		errorCode, errorMessage  sql.NullString
		metaJSON                 []byte
		createdAt, updatedAt     time.Time
	)

	row := r.db.QueryRowContext(ctx, ins,
		strings.TrimSpace(in.ProjectID),
		strings.TrimSpace(in.CommitKey),
		strings.TrimSpace(in.ManifestID),
		strings.TrimSpace(in.ManifestHash),
		runID,
		strings.TrimSpace(in.TraceID),
		strings.TrimSpace(in.Target),
		strings.TrimSpace(in.Status),
		string(in.MetaJSON),
	)

	scanErr := row.Scan(
		&commitID,
		&projectID,
		&commitKey,
		&manifestID,
		&manifestHash,
		&runIDOut,
		&traceID,
		&target,
		&status,
		&errorCode,
		&errorMessage,
		&metaJSON,
		&createdAt,
		&updatedAt,
	)

	if scanErr == nil {
		out = mapPublishCommit(commitID, projectID, commitKey, manifestID, manifestHash, runIDOut, traceID, target, status, errorCode, errorMessage, metaJSON, createdAt, updatedAt)
		return out, false, nil
	}

	// When ON CONFLICT DO NOTHING happens, RETURNING yields no rows -> Scan returns sql.ErrNoRows.
	if !errors.Is(scanErr, sql.ErrNoRows) {
		return run.PublishCommit{}, false, scanErr
	}

	// 2) Conflict: fetch existing row.
	ex, err := r.GetByProjectAndKey(ctx, in.ProjectID, in.CommitKey)
	if err != nil {
		return run.PublishCommit{}, false, err
	}
	return ex, true, nil
}

func (r *PublishRepository) GetByProjectAndKey(ctx context.Context, projectID string, commitKey string) (run.PublishCommit, error) {
	if strings.TrimSpace(projectID) == "" {
		return run.PublishCommit{}, errors.New("project_id is required")
	}
	if strings.TrimSpace(commitKey) == "" {
		return run.PublishCommit{}, errors.New("commit_key is required")
	}

	const q = `
SELECT
  commit_id,
  project_id,
  commit_key,
  manifest_id,
  manifest_hash,
  run_id,
  trace_id,
  target,
  status,
  error_code,
  error_message,
  meta_json,
  created_at,
  updated_at
FROM public.catalog_publish_commits
WHERE project_id=$1 AND commit_key=$2
LIMIT 1;
`
	var (
		commitID                 string
		pid, ckey                string
		manifestID, manifestHash string
		runIDOut                 sql.NullString
		traceID, target, status  string
		errorCode, errorMessage  sql.NullString
		metaJSON                 []byte
		createdAt, updatedAt     time.Time
	)

	err := r.db.QueryRowContext(ctx, q,
		strings.TrimSpace(projectID),
		strings.TrimSpace(commitKey),
	).Scan(
		&commitID,
		&pid,
		&ckey,
		&manifestID,
		&manifestHash,
		&runIDOut,
		&traceID,
		&target,
		&status,
		&errorCode,
		&errorMessage,
		&metaJSON,
		&createdAt,
		&updatedAt,
	)
	if err != nil {
		return run.PublishCommit{}, err
	}

	return mapPublishCommit(commitID, pid, ckey, manifestID, manifestHash, runIDOut, traceID, target, status, errorCode, errorMessage, metaJSON, createdAt, updatedAt), nil
}

func (r *PublishRepository) MarkConfirmed(ctx context.Context, commitID string) error {
	if strings.TrimSpace(commitID) == "" {
		return errors.New("commit_id is required")
	}
	_, err := r.db.ExecContext(ctx, `
UPDATE public.catalog_publish_commits
SET status='confirmed', error_code=NULL, error_message=NULL, updated_at=now()
WHERE commit_id=$1::uuid
`, strings.TrimSpace(commitID))
	return err
}

func (r *PublishRepository) MarkFailed(ctx context.Context, commitID string, code string, msg string) error {
	if strings.TrimSpace(commitID) == "" {
		return errors.New("commit_id is required")
	}
	if strings.TrimSpace(code) == "" {
		return errors.New("error_code is required")
	}
	_, err := r.db.ExecContext(ctx, `
UPDATE public.catalog_publish_commits
SET status='failed', error_code=$2, error_message=$3, updated_at=now()
WHERE commit_id=$1::uuid
`, strings.TrimSpace(commitID), strings.TrimSpace(code), msg)
	return err
}

func mapPublishCommit(
	commitID string,
	projectID string,
	commitKey string,
	manifestID string,
	manifestHash string,
	runIDOut sql.NullString,
	traceID string,
	target string,
	status string,
	errorCode sql.NullString,
	errorMessage sql.NullString,
	metaJSON []byte,
	createdAt time.Time,
	updatedAt time.Time,
) run.PublishCommit {
	var rid *string
	if runIDOut.Valid && strings.TrimSpace(runIDOut.String) != "" {
		s := strings.TrimSpace(runIDOut.String)
		rid = &s
	}

	var ec *string
	if errorCode.Valid && strings.TrimSpace(errorCode.String) != "" {
		s := strings.TrimSpace(errorCode.String)
		ec = &s
	}

	var em *string
	if errorMessage.Valid && strings.TrimSpace(errorMessage.String) != "" {
		s := errorMessage.String
		em = &s
	}

	if len(metaJSON) == 0 {
		metaJSON = []byte(`{}`)
	}

	return run.PublishCommit{
		CommitID:     commitID,
		ProjectID:    projectID,
		CommitKey:    commitKey,
		ManifestID:   manifestID,
		ManifestHash: manifestHash,
		RunID:        rid,
		TraceID:      traceID,
		Target:       target,
		Status:       status,
		ErrorCode:    ec,
		ErrorMessage: em,
		MetaJSON:     metaJSON,
		CreatedAt:    createdAt,
		UpdatedAt:    updatedAt,
	}
}
