package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type DocParseResultRepo struct {
	db *pgxpool.Pool
}

func NewDocParseResultRepo(db *pgxpool.Pool) *DocParseResultRepo {
	return &DocParseResultRepo{db: db}
}

type CreateDocParseResultInput struct {
	TaskID                    int64
	ProjectID                 string
	EngineKind                string
	EngineVersion             string
	BlocksJSON                []byte
	ReadingOrderJSON          []byte
	TablesJSON                []byte
	MarkdownText              string
	PayloadEvidenceAssetID    int64
	ConfidenceEvidenceAssetID *int64
	MetadataJSON              []byte
}

type DocParseResultRecord struct {
	ID                       int64
	TaskID                   int64
	ProjectID                string
	EngineKind               string
	EngineVersion            string
	BlocksJSON               []byte
	ReadingOrderJSON         []byte
	TablesJSON               []byte
	MarkdownText             string
	PayloadEvidenceAssetID   int64
	ConfidenceEvidenceAssetID *int64
	MetadataJSON             []byte
	CreatedAt                time.Time
	UpdatedAt                time.Time
}

func (r *DocParseResultRepo) Create(ctx context.Context, in CreateDocParseResultInput) (int64, error) {
	if r == nil || r.db == nil {
		return 0, fmt.Errorf("create docparse result: repo is nil")
	}
	if in.TaskID <= 0 {
		return 0, fmt.Errorf("create docparse result: task_id is required")
	}
	if in.ProjectID == "" {
		return 0, fmt.Errorf("create docparse result: project_id is required")
	}
	if in.EngineKind == "" {
		return 0, fmt.Errorf("create docparse result: engine_kind is required")
	}
	if in.EngineVersion == "" {
		return 0, fmt.Errorf("create docparse result: engine_version is required")
	}
	if in.PayloadEvidenceAssetID <= 0 {
		return 0, fmt.Errorf("create docparse result: payload_evidence_asset_id is required")
	}
	if in.BlocksJSON == nil {
		in.BlocksJSON = []byte(`[]`)
	}
	if in.ReadingOrderJSON == nil {
		in.ReadingOrderJSON = []byte(`[]`)
	}
	if in.TablesJSON == nil {
		in.TablesJSON = []byte(`[]`)
	}
	if in.MetadataJSON == nil {
		in.MetadataJSON = []byte(`{}`)
	}

	var id int64
	err := r.db.QueryRow(ctx, `
		INSERT INTO public.docparse_results (
			task_id,
			project_id,
			engine_kind,
			engine_version,
			blocks_json,
			reading_order_json,
			tables_json,
			markdown_text,
			payload_evidence_asset_id,
			confidence_evidence_asset_id,
			metadata_json,
			created_at,
			updated_at
		)
		VALUES (
			$1,$2,$3,$4,$5::jsonb,$6::jsonb,$7::jsonb,$8,$9,$10,$11::jsonb,NOW(),NOW()
		)
		RETURNING id
	`,
		in.TaskID,
		in.ProjectID,
		in.EngineKind,
		in.EngineVersion,
		string(in.BlocksJSON),
		string(in.ReadingOrderJSON),
		string(in.TablesJSON),
		in.MarkdownText,
		in.PayloadEvidenceAssetID,
		in.ConfidenceEvidenceAssetID,
		string(in.MetadataJSON),
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("create docparse result insert: %w", err)
	}

	return id, nil
}

func (r *DocParseResultRepo) FindByID(ctx context.Context, id int64) (DocParseResultRecord, error) {
	if r == nil || r.db == nil {
		return DocParseResultRecord{}, fmt.Errorf("find docparse result by id: repo is nil")
	}
	if id <= 0 {
		return DocParseResultRecord{}, fmt.Errorf("find docparse result by id: id is required")
	}

	var rec DocParseResultRecord
	err := r.db.QueryRow(ctx, `
		SELECT
			id,
			task_id,
			project_id,
			engine_kind,
			engine_version,
			blocks_json::text,
			reading_order_json::text,
			tables_json::text,
			markdown_text,
			payload_evidence_asset_id,
			confidence_evidence_asset_id,
			metadata_json::text,
			created_at,
			updated_at
		FROM public.docparse_results
		WHERE id = $1
	`, id).Scan(
		&rec.ID,
		&rec.TaskID,
		&rec.ProjectID,
		&rec.EngineKind,
		&rec.EngineVersion,
		&rec.BlocksJSON,
		&rec.ReadingOrderJSON,
		&rec.TablesJSON,
		&rec.MarkdownText,
		&rec.PayloadEvidenceAssetID,
		&rec.ConfidenceEvidenceAssetID,
		&rec.MetadataJSON,
		&rec.CreatedAt,
		&rec.UpdatedAt,
	)
	if err != nil {
		return DocParseResultRecord{}, fmt.Errorf("find docparse result by id query: %w", err)
	}

	return rec, nil
}

func (r *DocParseResultRepo) ListByTaskID(ctx context.Context, projectID string, taskID int64) ([]DocParseResultRecord, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("list docparse results by task id: repo is nil")
	}
	if projectID == "" {
		return nil, fmt.Errorf("list docparse results by task id: project_id is required")
	}
	if taskID <= 0 {
		return nil, fmt.Errorf("list docparse results by task id: task_id is required")
	}

	rows, err := r.db.Query(ctx, `
		SELECT
			id,
			task_id,
			project_id,
			engine_kind,
			engine_version,
			blocks_json::text,
			reading_order_json::text,
			tables_json::text,
			markdown_text,
			payload_evidence_asset_id,
			confidence_evidence_asset_id,
			metadata_json::text,
			created_at,
			updated_at
		FROM public.docparse_results
		WHERE project_id = $1
		  AND task_id = $2
		ORDER BY created_at ASC, id ASC
	`, projectID, taskID)
	if err != nil {
		return nil, fmt.Errorf("list docparse results by task id query: %w", err)
	}
	defer rows.Close()

	out := make([]DocParseResultRecord, 0)
	for rows.Next() {
		var rec DocParseResultRecord
		if err := rows.Scan(
			&rec.ID,
			&rec.TaskID,
			&rec.ProjectID,
			&rec.EngineKind,
			&rec.EngineVersion,
			&rec.BlocksJSON,
			&rec.ReadingOrderJSON,
			&rec.TablesJSON,
			&rec.MarkdownText,
			&rec.PayloadEvidenceAssetID,
			&rec.ConfidenceEvidenceAssetID,
			&rec.MetadataJSON,
			&rec.CreatedAt,
			&rec.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("list docparse results by task id scan: %w", err)
		}
		out = append(out, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list docparse results by task id rows: %w", err)
	}

	return out, nil
}