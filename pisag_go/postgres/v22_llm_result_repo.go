package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type LLMResultRepo struct {
	db *pgxpool.Pool
}

func NewLLMResultRepo(db *pgxpool.Pool) *LLMResultRepo {
	return &LLMResultRepo{db: db}
}

type CreateLLMResultInput struct {
	TaskID                    int64
	ProjectID                 string
	EngineKind                string
	EngineVersion             string
	Provider                  string
	TaskKind                  string
	InputHash                 string
	OutputText                string
	OutputJSON                []byte
	RationaleText             string
	PromptVersion             string
	TokenUsageJSON            []byte
	CostEstimate              *float64
	PayloadEvidenceAssetID    int64
	ConfidenceEvidenceAssetID *int64
	MetadataJSON              []byte
}

type LLMResultRecord struct {
	ID                       int64
	TaskID                   int64
	ProjectID                string
	EngineKind               string
	EngineVersion            string
	Provider                 string
	TaskKind                 string
	InputHash                string
	OutputText               string
	OutputJSON               []byte
	RationaleText            string
	PromptVersion            string
	TokenUsageJSON           []byte
	CostEstimate             *float64
	PayloadEvidenceAssetID   int64
	ConfidenceEvidenceAssetID *int64
	MetadataJSON             []byte
	CreatedAt                time.Time
	UpdatedAt                time.Time
}

func (r *LLMResultRepo) Create(ctx context.Context, in CreateLLMResultInput) (int64, error) {
	if r == nil || r.db == nil {
		return 0, fmt.Errorf("create llm result: repo is nil")
	}
	if in.TaskID <= 0 {
		return 0, fmt.Errorf("create llm result: task_id is required")
	}
	if in.ProjectID == "" {
		return 0, fmt.Errorf("create llm result: project_id is required")
	}
	if in.EngineKind == "" {
		return 0, fmt.Errorf("create llm result: engine_kind is required")
	}
	if in.EngineVersion == "" {
		return 0, fmt.Errorf("create llm result: engine_version is required")
	}
	if in.Provider == "" {
		return 0, fmt.Errorf("create llm result: provider is required")
	}
	if in.TaskKind == "" {
		return 0, fmt.Errorf("create llm result: task_kind is required")
	}
	if in.InputHash == "" {
		return 0, fmt.Errorf("create llm result: input_hash is required")
	}
	if in.PayloadEvidenceAssetID <= 0 {
		return 0, fmt.Errorf("create llm result: payload_evidence_asset_id is required")
	}
	if in.OutputJSON == nil {
		in.OutputJSON = []byte(`{}`)
	}
	if in.TokenUsageJSON == nil {
		in.TokenUsageJSON = []byte(`{}`)
	}
	if in.MetadataJSON == nil {
		in.MetadataJSON = []byte(`{}`)
	}

	var id int64
	err := r.db.QueryRow(ctx, `
		INSERT INTO public.llm_results (
			task_id,
			project_id,
			engine_kind,
			engine_version,
			provider,
			task_kind,
			input_hash,
			output_text,
			output_json,
			rationale_text,
			prompt_version,
			token_usage_json,
			cost_estimate,
			payload_evidence_asset_id,
			confidence_evidence_asset_id,
			metadata_json,
			created_at,
			updated_at
		)
		VALUES (
			$1,$2,$3,$4,$5,$6,$7,$8,$9::jsonb,$10,$11,$12::jsonb,$13,$14,$15,$16::jsonb,NOW(),NOW()
		)
		RETURNING id
	`,
		in.TaskID,
		in.ProjectID,
		in.EngineKind,
		in.EngineVersion,
		in.Provider,
		in.TaskKind,
		in.InputHash,
		in.OutputText,
		string(in.OutputJSON),
		in.RationaleText,
		in.PromptVersion,
		string(in.TokenUsageJSON),
		in.CostEstimate,
		in.PayloadEvidenceAssetID,
		in.ConfidenceEvidenceAssetID,
		string(in.MetadataJSON),
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("create llm result insert: %w", err)
	}

	return id, nil
}

func (r *LLMResultRepo) FindByID(ctx context.Context, id int64) (LLMResultRecord, error) {
	if r == nil || r.db == nil {
		return LLMResultRecord{}, fmt.Errorf("find llm result by id: repo is nil")
	}
	if id <= 0 {
		return LLMResultRecord{}, fmt.Errorf("find llm result by id: id is required")
	}

	var rec LLMResultRecord
	err := r.db.QueryRow(ctx, `
		SELECT
			id,
			task_id,
			project_id,
			engine_kind,
			engine_version,
			provider,
			task_kind,
			input_hash,
			output_text,
			output_json::text,
			rationale_text,
			prompt_version,
			token_usage_json::text,
			cost_estimate,
			payload_evidence_asset_id,
			confidence_evidence_asset_id,
			metadata_json::text,
			created_at,
			updated_at
		FROM public.llm_results
		WHERE id = $1
	`, id).Scan(
		&rec.ID,
		&rec.TaskID,
		&rec.ProjectID,
		&rec.EngineKind,
		&rec.EngineVersion,
		&rec.Provider,
		&rec.TaskKind,
		&rec.InputHash,
		&rec.OutputText,
		&rec.OutputJSON,
		&rec.RationaleText,
		&rec.PromptVersion,
		&rec.TokenUsageJSON,
		&rec.CostEstimate,
		&rec.PayloadEvidenceAssetID,
		&rec.ConfidenceEvidenceAssetID,
		&rec.MetadataJSON,
		&rec.CreatedAt,
		&rec.UpdatedAt,
	)
	if err != nil {
		return LLMResultRecord{}, fmt.Errorf("find llm result by id query: %w", err)
	}

	return rec, nil
}

func (r *LLMResultRepo) ListByTaskID(ctx context.Context, projectID string, taskID int64) ([]LLMResultRecord, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("list llm results by task id: repo is nil")
	}
	if projectID == "" {
		return nil, fmt.Errorf("list llm results by task id: project_id is required")
	}
	if taskID <= 0 {
		return nil, fmt.Errorf("list llm results by task id: task_id is required")
	}

	rows, err := r.db.Query(ctx, `
		SELECT
			id,
			task_id,
			project_id,
			engine_kind,
			engine_version,
			provider,
			task_kind,
			input_hash,
			output_text,
			output_json::text,
			rationale_text,
			prompt_version,
			token_usage_json::text,
			cost_estimate,
			payload_evidence_asset_id,
			confidence_evidence_asset_id,
			metadata_json::text,
			created_at,
			updated_at
		FROM public.llm_results
		WHERE project_id = $1
		  AND task_id = $2
		ORDER BY created_at ASC, id ASC
	`, projectID, taskID)
	if err != nil {
		return nil, fmt.Errorf("list llm results by task id query: %w", err)
	}
	defer rows.Close()

	out := make([]LLMResultRecord, 0)
	for rows.Next() {
		var rec LLMResultRecord
		if err := rows.Scan(
			&rec.ID,
			&rec.TaskID,
			&rec.ProjectID,
			&rec.EngineKind,
			&rec.EngineVersion,
			&rec.Provider,
			&rec.TaskKind,
			&rec.InputHash,
			&rec.OutputText,
			&rec.OutputJSON,
			&rec.RationaleText,
			&rec.PromptVersion,
			&rec.TokenUsageJSON,
			&rec.CostEstimate,
			&rec.PayloadEvidenceAssetID,
			&rec.ConfidenceEvidenceAssetID,
			&rec.MetadataJSON,
			&rec.CreatedAt,
			&rec.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("list llm results by task id scan: %w", err)
		}
		out = append(out, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list llm results by task id rows: %w", err)
	}

	return out, nil
}