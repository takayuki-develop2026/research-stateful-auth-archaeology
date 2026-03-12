package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	run "example.com/pisag_go/run"
)

type RuntimeRequestEvidenceRepo struct {
	DB *pgxpool.Pool
}

func (r *RuntimeRequestEvidenceRepo) RegisterJSONEvidence(
	ctx context.Context,
	in run.RegisterRuntimeJSONEvidenceInput,
) (run.RegisterRuntimeJSONEvidenceOutput, error) {
	if r == nil || r.DB == nil {
		return run.RegisterRuntimeJSONEvidenceOutput{}, fmt.Errorf("register runtime json evidence: repo is nil")
	}
	if in.ProjectID == "" {
		return run.RegisterRuntimeJSONEvidenceOutput{}, fmt.Errorf("register runtime json evidence: project_id is required")
	}
	if in.TraceID == "" {
		return run.RegisterRuntimeJSONEvidenceOutput{}, fmt.Errorf("register runtime json evidence: trace_id is required")
	}
	if in.Kind == "" {
		return run.RegisterRuntimeJSONEvidenceOutput{}, fmt.Errorf("register runtime json evidence: kind is required")
	}
	if in.BodyJSON == "" {
		return run.RegisterRuntimeJSONEvidenceOutput{}, fmt.Errorf("register runtime json evidence: body_json is required")
	}
	if in.SHA256 == "" {
		return run.RegisterRuntimeJSONEvidenceOutput{}, fmt.Errorf("register runtime json evidence: sha256 is required")
	}

	var id int64
	err := r.DB.QueryRow(ctx, `
		INSERT INTO public.evidence_assets (
			project_id,
			trace_id,
			run_id,
			kind,
			media_type,
			source_kind,
			source_uri,
			content_sha256,
			content_length,
			mime_type,
			status,
			created_by_type,
			created_by_id,
			created_at,
			updated_at
		)
		VALUES (
			$1,
			$2,
			NULL,
			$3,
			'text',
			'generated',
			$4,
			$5,
			$6,
			'application/json',
			'active',
			'system',
			'v22_runtime_api',
			NOW(),
			NOW()
		)
		RETURNING id
	`,
		in.ProjectID,
		in.TraceID,
		in.Kind,
		in.Description,
		in.SHA256,
		int64(len(in.BodyJSON)),
	).Scan(&id)
	if err != nil {
		return run.RegisterRuntimeJSONEvidenceOutput{}, fmt.Errorf("register runtime json evidence insert: %w", err)
	}

	return run.RegisterRuntimeJSONEvidenceOutput{
		EvidenceAssetID: id,
	}, nil
}