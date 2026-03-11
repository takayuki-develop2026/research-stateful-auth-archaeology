package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type PreprocessResultRepo struct {
	db *pgxpool.Pool
}

func NewPreprocessResultRepo(db *pgxpool.Pool) *PreprocessResultRepo {
	return &PreprocessResultRepo{db: db}
}

type CreatePreprocessResultInput struct {
	TaskID                 int64
	ProjectID              string
	SourceEvidenceAssetID  int64
	EngineKind             string
	EngineVersion          string
	OperationsJSON         []byte
	OutputEvidenceAssetID  int64
	QualityScore           *float64
	MetadataJSON           []byte
}

type PreprocessResultRecord struct {
	ID                    int64
	TaskID                int64
	ProjectID             string
	SourceEvidenceAssetID int64
	EngineKind            string
	EngineVersion         string
	OperationsJSON        []byte
	OutputEvidenceAssetID int64
	QualityScore          *float64
	MetadataJSON          []byte
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

func (r *PreprocessResultRepo) Create(ctx context.Context, in CreatePreprocessResultInput) (int64, error) {
	if r == nil || r.db == nil {
		return 0, fmt.Errorf("create preprocess result: repo is nil")
	}
	if in.TaskID <= 0 {
		return 0, fmt.Errorf("create preprocess result: task_id is required")
	}
	if in.ProjectID == "" {
		return 0, fmt.Errorf("create preprocess result: project_id is required")
	}
	if in.SourceEvidenceAssetID <= 0 {
		return 0, fmt.Errorf("create preprocess result: source_evidence_asset_id is required")
	}
	if in.EngineKind == "" {
		return 0, fmt.Errorf("create preprocess result: engine_kind is required")
	}
	if in.EngineVersion == "" {
		return 0, fmt.Errorf("create preprocess result: engine_version is required")
	}
	if in.OutputEvidenceAssetID <= 0 {
		return 0, fmt.Errorf("create preprocess result: output_evidence_asset_id is required")
	}
	if in.OperationsJSON == nil {
		in.OperationsJSON = []byte(`[]`)
	}
	if in.MetadataJSON == nil {
		in.MetadataJSON = []byte(`{}`)
	}

	var id int64
	err := r.db.QueryRow(ctx, `
		INSERT INTO public.preprocess_results (
			task_id,
			project_id,
			source_evidence_asset_id,
			engine_kind,
			engine_version,
			operations_json,
			output_evidence_asset_id,
			quality_score,
			metadata_json,
			created_at,
			updated_at
		)
		VALUES (
			$1,$2,$3,$4,$5,$6::jsonb,$7,$8,$9::jsonb,NOW(),NOW()
		)
		RETURNING id
	`,
		in.TaskID,
		in.ProjectID,
		in.SourceEvidenceAssetID,
		in.EngineKind,
		in.EngineVersion,
		string(in.OperationsJSON),
		in.OutputEvidenceAssetID,
		in.QualityScore,
		string(in.MetadataJSON),
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("create preprocess result insert: %w", err)
	}

	return id, nil
}

func (r *PreprocessResultRepo) FindByID(ctx context.Context, id int64) (PreprocessResultRecord, error) {
	if r == nil || r.db == nil {
		return PreprocessResultRecord{}, fmt.Errorf("find preprocess result by id: repo is nil")
	}
	if id <= 0 {
		return PreprocessResultRecord{}, fmt.Errorf("find preprocess result by id: id is required")
	}

	var rec PreprocessResultRecord
	err := r.db.QueryRow(ctx, `
		SELECT
			id,
			task_id,
			project_id,
			source_evidence_asset_id,
			engine_kind,
			engine_version,
			operations_json::text,
			output_evidence_asset_id,
			quality_score,
			metadata_json::text,
			created_at,
			updated_at
		FROM public.preprocess_results
		WHERE id = $1
	`, id).Scan(
		&rec.ID,
		&rec.TaskID,
		&rec.ProjectID,
		&rec.SourceEvidenceAssetID,
		&rec.EngineKind,
		&rec.EngineVersion,
		&rec.OperationsJSON,
		&rec.OutputEvidenceAssetID,
		&rec.QualityScore,
		&rec.MetadataJSON,
		&rec.CreatedAt,
		&rec.UpdatedAt,
	)
	if err != nil {
		return PreprocessResultRecord{}, fmt.Errorf("find preprocess result by id query: %w", err)
	}

	return rec, nil
}

func (r *PreprocessResultRepo) ListByTaskID(ctx context.Context, projectID string, taskID int64) ([]PreprocessResultRecord, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("list preprocess results by task id: repo is nil")
	}
	if projectID == "" {
		return nil, fmt.Errorf("list preprocess results by task id: project_id is required")
	}
	if taskID <= 0 {
		return nil, fmt.Errorf("list preprocess results by task id: task_id is required")
	}

	rows, err := r.db.Query(ctx, `
		SELECT
			id,
			task_id,
			project_id,
			source_evidence_asset_id,
			engine_kind,
			engine_version,
			operations_json::text,
			output_evidence_asset_id,
			quality_score,
			metadata_json::text,
			created_at,
			updated_at
		FROM public.preprocess_results
		WHERE project_id = $1
		  AND task_id = $2
		ORDER BY created_at ASC, id ASC
	`, projectID, taskID)
	if err != nil {
		return nil, fmt.Errorf("list preprocess results by task id query: %w", err)
	}
	defer rows.Close()

	out := make([]PreprocessResultRecord, 0)
	for rows.Next() {
		var rec PreprocessResultRecord
		if err := rows.Scan(
			&rec.ID,
			&rec.TaskID,
			&rec.ProjectID,
			&rec.SourceEvidenceAssetID,
			&rec.EngineKind,
			&rec.EngineVersion,
			&rec.OperationsJSON,
			&rec.OutputEvidenceAssetID,
			&rec.QualityScore,
			&rec.MetadataJSON,
			&rec.CreatedAt,
			&rec.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("list preprocess results by task id scan: %w", err)
		}
		out = append(out, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list preprocess results by task id rows: %w", err)
	}

	return out, nil
}