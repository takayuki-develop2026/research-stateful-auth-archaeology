package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"


)

type ModelRunRepo struct {
	db *pgxpool.Pool
}

func NewModelRunRepo(db *pgxpool.Pool) *ModelRunRepo {
	return &ModelRunRepo{db: db}
}

type CreateModelRunInput struct {
	TaskID         int64
	ProjectID      string
	Capability     string
	EngineKind     string
	EngineVersion  string
	Provider       string
	TaskKind       *string
	Status         string
	InputHash      string
	StartedAt      time.Time
	FinishedAt     *time.Time
	LatencyMS      *int64
	TokenUsageJSON []byte
	CostEstimate   *float64
	MetadataJSON   []byte
}

func (r *ModelRunRepo) Create(ctx context.Context, in CreateModelRunInput) (int64, error) {
	if r == nil || r.db == nil {
		return 0, fmt.Errorf("create model run: repo is nil")
	}
	if in.TaskID <= 0 {
		return 0, fmt.Errorf("create model run: task_id is required")
	}
	if in.ProjectID == "" {
		return 0, fmt.Errorf("create model run: project_id is required")
	}
	if in.Capability == "" {
		return 0, fmt.Errorf("create model run: capability is required")
	}
	if in.EngineKind == "" {
		return 0, fmt.Errorf("create model run: engine_kind is required")
	}
	if in.EngineVersion == "" {
		return 0, fmt.Errorf("create model run: engine_version is required")
	}
	if in.Provider == "" {
		return 0, fmt.Errorf("create model run: provider is required")
	}
	if in.Status == "" {
		return 0, fmt.Errorf("create model run: status is required")
	}
	if in.InputHash == "" {
		return 0, fmt.Errorf("create model run: input_hash is required")
	}
	if in.TokenUsageJSON == nil {
		in.TokenUsageJSON = []byte(`{}`)
	}
	if in.MetadataJSON == nil {
		in.MetadataJSON = []byte(`{}`)
	}
	if in.StartedAt.IsZero() {
		in.StartedAt = time.Now().UTC()
	}

	var id int64
	err := r.db.QueryRow(ctx, `
		INSERT INTO public.model_runs (
			task_id,
			project_id,
			capability,
			engine_kind,
			engine_version,
			provider,
			task_kind,
			status,
			input_hash,
			started_at,
			finished_at,
			latency_ms,
			token_usage_json,
			cost_estimate,
			metadata_json,
			created_at,
			updated_at
		)
		VALUES (
			$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13::jsonb,$14,$15::jsonb,NOW(),NOW()
		)
		RETURNING id
	`,
		in.TaskID,
		in.ProjectID,
		in.Capability,
		in.EngineKind,
		in.EngineVersion,
		in.Provider,
		in.TaskKind,
		in.Status,
		in.InputHash,
		in.StartedAt,
		in.FinishedAt,
		in.LatencyMS,
		string(in.TokenUsageJSON),
		in.CostEstimate,
		string(in.MetadataJSON),
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("create model run insert: %w", err)
	}

	return id, nil
}

type ModelRunRecord struct {
	ID            int64
	TaskID        int64
	ProjectID     string
	Capability    string
	EngineKind    string
	EngineVersion string
	Provider      string
	TaskKind      *string
	Status        string
	InputHash     string
	StartedAt     time.Time
	FinishedAt    *time.Time
	LatencyMS     *int64
	TokenUsageJSON []byte
	CostEstimate  *float64
	MetadataJSON  []byte
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

func (r *ModelRunRepo) FindByID(ctx context.Context, id int64) (ModelRunRecord, error) {
	if r == nil || r.db == nil {
		return ModelRunRecord{}, fmt.Errorf("find model run by id: repo is nil")
	}
	if id <= 0 {
		return ModelRunRecord{}, fmt.Errorf("find model run by id: id is required")
	}

	var rec ModelRunRecord
	err := r.db.QueryRow(ctx, `
		SELECT
			id,
			task_id,
			project_id,
			capability,
			engine_kind,
			engine_version,
			provider,
			task_kind,
			status,
			input_hash,
			started_at,
			finished_at,
			latency_ms,
			token_usage_json::text,
			cost_estimate,
			metadata_json::text,
			created_at,
			updated_at
		FROM public.model_runs
		WHERE id = $1
	`, id).Scan(
		&rec.ID,
		&rec.TaskID,
		&rec.ProjectID,
		&rec.Capability,
		&rec.EngineKind,
		&rec.EngineVersion,
		&rec.Provider,
		&rec.TaskKind,
		&rec.Status,
		&rec.InputHash,
		&rec.StartedAt,
		&rec.FinishedAt,
		&rec.LatencyMS,
		&rec.TokenUsageJSON,
		&rec.CostEstimate,
		&rec.MetadataJSON,
		&rec.CreatedAt,
		&rec.UpdatedAt,
	)
	if err != nil {
		return ModelRunRecord{}, fmt.Errorf("find model run by id query: %w", err)
	}

	return rec, nil
}

func (r *ModelRunRepo) ListByTaskID(ctx context.Context, projectID string, taskID int64) ([]ModelRunRecord, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("list model runs by task id: repo is nil")
	}
	if projectID == "" {
		return nil, fmt.Errorf("list model runs by task id: project_id is required")
	}
	if taskID <= 0 {
		return nil, fmt.Errorf("list model runs by task id: task_id is required")
	}

	rows, err := r.db.Query(ctx, `
		SELECT
			id,
			task_id,
			project_id,
			capability,
			engine_kind,
			engine_version,
			provider,
			task_kind,
			status,
			input_hash,
			started_at,
			finished_at,
			latency_ms,
			token_usage_json::text,
			cost_estimate,
			metadata_json::text,
			created_at,
			updated_at
		FROM public.model_runs
		WHERE project_id = $1
		  AND task_id = $2
		ORDER BY started_at ASC, id ASC
	`, projectID, taskID)
	if err != nil {
		return nil, fmt.Errorf("list model runs by task id query: %w", err)
	}
	defer rows.Close()

	out := make([]ModelRunRecord, 0)
	for rows.Next() {
		var rec ModelRunRecord
		if err := rows.Scan(
			&rec.ID,
			&rec.TaskID,
			&rec.ProjectID,
			&rec.Capability,
			&rec.EngineKind,
			&rec.EngineVersion,
			&rec.Provider,
			&rec.TaskKind,
			&rec.Status,
			&rec.InputHash,
			&rec.StartedAt,
			&rec.FinishedAt,
			&rec.LatencyMS,
			&rec.TokenUsageJSON,
			&rec.CostEstimate,
			&rec.MetadataJSON,
			&rec.CreatedAt,
			&rec.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("list model runs by task id scan: %w", err)
		}
		out = append(out, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list model runs by task id rows: %w", err)
	}

	return out, nil
}