package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type EmbeddingResultRepo struct {
	db *pgxpool.Pool
}

func NewEmbeddingResultRepo(db *pgxpool.Pool) *EmbeddingResultRepo {
	return &EmbeddingResultRepo{db: db}
}

type CreateEmbeddingResultInput struct {
	TaskID                 int64
	ProjectID              string
	EngineKind             string
	EngineVersion          string
	EmbeddingVectorRef     string
	EmbeddingDim           int
	TopCandidatesJSON      []byte
	PayloadEvidenceAssetID int64
	MetadataJSON           []byte
}

type EmbeddingResultRecord struct {
	ID                    int64
	TaskID                int64
	ProjectID             string
	EngineKind            string
	EngineVersion         string
	EmbeddingVectorRef    string
	EmbeddingDim          int
	TopCandidatesJSON     []byte
	PayloadEvidenceAssetID int64
	MetadataJSON          []byte
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

func (r *EmbeddingResultRepo) Create(ctx context.Context, in CreateEmbeddingResultInput) (int64, error) {
	if r == nil || r.db == nil {
		return 0, fmt.Errorf("create embedding result: repo is nil")
	}
	if in.TaskID <= 0 {
		return 0, fmt.Errorf("create embedding result: task_id is required")
	}
	if in.ProjectID == "" {
		return 0, fmt.Errorf("create embedding result: project_id is required")
	}
	if in.EngineKind == "" {
		return 0, fmt.Errorf("create embedding result: engine_kind is required")
	}
	if in.EngineVersion == "" {
		return 0, fmt.Errorf("create embedding result: engine_version is required")
	}
	if in.EmbeddingVectorRef == "" {
		return 0, fmt.Errorf("create embedding result: embedding_vector_ref is required")
	}
	if in.EmbeddingDim <= 0 {
		return 0, fmt.Errorf("create embedding result: embedding_dim is required")
	}
	if in.PayloadEvidenceAssetID <= 0 {
		return 0, fmt.Errorf("create embedding result: payload_evidence_asset_id is required")
	}
	if in.TopCandidatesJSON == nil {
		in.TopCandidatesJSON = []byte(`[]`)
	}
	if in.MetadataJSON == nil {
		in.MetadataJSON = []byte(`{}`)
	}

	var id int64
	err := r.db.QueryRow(ctx, `
		INSERT INTO public.embedding_results (
			task_id,
			project_id,
			engine_kind,
			engine_version,
			embedding_vector_ref,
			embedding_dim,
			top_candidates_json,
			payload_evidence_asset_id,
			metadata_json,
			created_at,
			updated_at
		)
		VALUES (
			$1,$2,$3,$4,$5,$6,$7::jsonb,$8,$9::jsonb,NOW(),NOW()
		)
		RETURNING id
	`,
		in.TaskID,
		in.ProjectID,
		in.EngineKind,
		in.EngineVersion,
		in.EmbeddingVectorRef,
		in.EmbeddingDim,
		string(in.TopCandidatesJSON),
		in.PayloadEvidenceAssetID,
		string(in.MetadataJSON),
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("create embedding result insert: %w", err)
	}

	return id, nil
}

func (r *EmbeddingResultRepo) FindByID(ctx context.Context, id int64) (EmbeddingResultRecord, error) {
	if r == nil || r.db == nil {
		return EmbeddingResultRecord{}, fmt.Errorf("find embedding result by id: repo is nil")
	}
	if id <= 0 {
		return EmbeddingResultRecord{}, fmt.Errorf("find embedding result by id: id is required")
	}

	var rec EmbeddingResultRecord
	err := r.db.QueryRow(ctx, `
		SELECT
			id,
			task_id,
			project_id,
			engine_kind,
			engine_version,
			embedding_vector_ref,
			embedding_dim,
			top_candidates_json::text,
			payload_evidence_asset_id,
			metadata_json::text,
			created_at,
			updated_at
		FROM public.embedding_results
		WHERE id = $1
	`, id).Scan(
		&rec.ID,
		&rec.TaskID,
		&rec.ProjectID,
		&rec.EngineKind,
		&rec.EngineVersion,
		&rec.EmbeddingVectorRef,
		&rec.EmbeddingDim,
		&rec.TopCandidatesJSON,
		&rec.PayloadEvidenceAssetID,
		&rec.MetadataJSON,
		&rec.CreatedAt,
		&rec.UpdatedAt,
	)
	if err != nil {
		return EmbeddingResultRecord{}, fmt.Errorf("find embedding result by id query: %w", err)
	}

	return rec, nil
}

func (r *EmbeddingResultRepo) ListByTaskID(ctx context.Context, projectID string, taskID int64) ([]EmbeddingResultRecord, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("list embedding results by task id: repo is nil")
	}
	if projectID == "" {
		return nil, fmt.Errorf("list embedding results by task id: project_id is required")
	}
	if taskID <= 0 {
		return nil, fmt.Errorf("list embedding results by task id: task_id is required")
	}

	rows, err := r.db.Query(ctx, `
		SELECT
			id,
			task_id,
			project_id,
			engine_kind,
			engine_version,
			embedding_vector_ref,
			embedding_dim,
			top_candidates_json::text,
			payload_evidence_asset_id,
			metadata_json::text,
			created_at,
			updated_at
		FROM public.embedding_results
		WHERE project_id = $1
		  AND task_id = $2
		ORDER BY created_at ASC, id ASC
	`, projectID, taskID)
	if err != nil {
		return nil, fmt.Errorf("list embedding results by task id query: %w", err)
	}
	defer rows.Close()

	out := make([]EmbeddingResultRecord, 0)
	for rows.Next() {
		var rec EmbeddingResultRecord
		if err := rows.Scan(
			&rec.ID,
			&rec.TaskID,
			&rec.ProjectID,
			&rec.EngineKind,
			&rec.EngineVersion,
			&rec.EmbeddingVectorRef,
			&rec.EmbeddingDim,
			&rec.TopCandidatesJSON,
			&rec.PayloadEvidenceAssetID,
			&rec.MetadataJSON,
			&rec.CreatedAt,
			&rec.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("list embedding results by task id scan: %w", err)
		}
		out = append(out, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list embedding results by task id rows: %w", err)
	}

	return out, nil
}