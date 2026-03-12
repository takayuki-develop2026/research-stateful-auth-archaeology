package postgres

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	run "example.com/pisag_go/run"
)

type RuntimeUploadEvidenceRepo struct {
	DB *pgxpool.Pool
}

func NewRuntimeUploadEvidenceRepo(db *pgxpool.Pool) *RuntimeUploadEvidenceRepo {
	return &RuntimeUploadEvidenceRepo{DB: db}
}

func (r *RuntimeUploadEvidenceRepo) RegisterUploadedEvidence(
	ctx context.Context,
	in run.RegisterRuntimeUploadedEvidenceInput,
) (run.RegisterRuntimeUploadedEvidenceOutput, error) {
	if r == nil || r.DB == nil {
		return run.RegisterRuntimeUploadedEvidenceOutput{}, fmt.Errorf("register uploaded evidence: repo is nil")
	}
	if strings.TrimSpace(in.ProjectID) == "" {
		return run.RegisterRuntimeUploadedEvidenceOutput{}, fmt.Errorf("register uploaded evidence: project_id is required")
	}
	if strings.TrimSpace(in.TraceID) == "" {
		return run.RegisterRuntimeUploadedEvidenceOutput{}, fmt.Errorf("register uploaded evidence: trace_id is required")
	}
	if strings.TrimSpace(in.OriginalFilename) == "" {
		return run.RegisterRuntimeUploadedEvidenceOutput{}, fmt.Errorf("register uploaded evidence: original_filename is required")
	}
	if strings.TrimSpace(in.ContentType) == "" {
		return run.RegisterRuntimeUploadedEvidenceOutput{}, fmt.Errorf("register uploaded evidence: content_type is required")
	}
	if strings.TrimSpace(in.SHA256) == "" {
		return run.RegisterRuntimeUploadedEvidenceOutput{}, fmt.Errorf("register uploaded evidence: sha256 is required")
	}
	if in.SizeBytes <= 0 {
		return run.RegisterRuntimeUploadedEvidenceOutput{}, fmt.Errorf("register uploaded evidence: size_bytes must be > 0")
	}

	kind := "runtime_input_file"
	mediaType := runtimeMediaTypeFromContentType(in.ContentType)
	sourceKind := "upload"

	sourceURI := strings.TrimSpace(in.SourceURI)
	if sourceURI == "" {
		return run.RegisterRuntimeUploadedEvidenceOutput{}, fmt.Errorf("register uploaded evidence: source_uri is required (must be stored file path)")
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
			$4,
			$5,
			$6,
			$7,
			$8,
			$9,
			'active',
			'system',
			'v22_runtime_api',
			NOW(),
			NOW()
		)
		RETURNING id
	`,
		strings.TrimSpace(in.ProjectID),
		strings.TrimSpace(in.TraceID),
		kind,
		mediaType,
		sourceKind,
		sourceURI,
		strings.TrimSpace(in.SHA256),
		in.SizeBytes,
		strings.TrimSpace(in.ContentType),
	).Scan(&id)
	if err != nil {
		return run.RegisterRuntimeUploadedEvidenceOutput{}, fmt.Errorf("register uploaded evidence insert: %w", err)
	}

	return run.RegisterRuntimeUploadedEvidenceOutput{
		EvidenceAssetID: id,
		Kind:            kind,
		Bytes:           in.SizeBytes,
		SHA256:          strings.TrimSpace(in.SHA256),
		Filename:        filepath.Base(strings.TrimSpace(in.OriginalFilename)),
	}, nil
}

func (r *RuntimeUploadEvidenceRepo) GetUploadedEvidenceSummary(
	ctx context.Context,
	in run.GetRuntimeUploadedEvidenceSummaryInput,
) (run.GetRuntimeUploadedEvidenceSummaryOutput, error) {
	if r == nil || r.DB == nil {
		return run.GetRuntimeUploadedEvidenceSummaryOutput{}, fmt.Errorf("get uploaded evidence summary: repo is nil")
	}
	if strings.TrimSpace(in.ProjectID) == "" {
		return run.GetRuntimeUploadedEvidenceSummaryOutput{}, fmt.Errorf("get uploaded evidence summary: project_id is required")
	}
	if in.EvidenceID <= 0 {
		return run.GetRuntimeUploadedEvidenceSummaryOutput{}, fmt.Errorf("get uploaded evidence summary: evidence_id must be > 0")
	}

	var (
		id            int64
		kind          string
		contentLength int64
		contentSHA    string
		sourceURI     string
	)
	err := r.DB.QueryRow(ctx, `
		SELECT
			id,
			kind,
			content_length,
			content_sha256,
			COALESCE(source_uri, '')
		FROM public.evidence_assets
		WHERE project_id = $1
		  AND id = $2
		LIMIT 1
	`,
		strings.TrimSpace(in.ProjectID),
		in.EvidenceID,
	).Scan(&id, &kind, &contentLength, &contentSHA, &sourceURI)
	if err != nil {
		return run.GetRuntimeUploadedEvidenceSummaryOutput{}, fmt.Errorf("get uploaded evidence summary select: %w", err)
	}

	return run.GetRuntimeUploadedEvidenceSummaryOutput{
		EvidenceAssetID: id,
		Kind:            kind,
		Bytes:           contentLength,
		SHA256:          contentSHA,
		Filename:        filepath.Base(strings.TrimSpace(sourceURI)),
	}, nil
}

func (r *RuntimeUploadEvidenceRepo) GetEvidenceSourceURI(
	ctx context.Context,
	projectID string,
	evidenceID int64,
) (string, error) {
	if r == nil || r.DB == nil {
		return "", fmt.Errorf("get evidence source uri: repo is nil")
	}
	if strings.TrimSpace(projectID) == "" {
		return "", fmt.Errorf("get evidence source uri: project_id is required")
	}
	if evidenceID <= 0 {
		return "", fmt.Errorf("get evidence source uri: evidence_id must be > 0")
	}

	var sourceURI string
	err := r.DB.QueryRow(ctx, `
		SELECT COALESCE(source_uri, '')
		FROM public.evidence_assets
		WHERE project_id = $1
		  AND id = $2
		LIMIT 1
	`,
		strings.TrimSpace(projectID),
		evidenceID,
	).Scan(&sourceURI)
	if err != nil {
		return "", fmt.Errorf("get evidence source uri select: %w", err)
	}

	sourceURI = strings.TrimSpace(sourceURI)
	if sourceURI == "" {
		return "", fmt.Errorf("get evidence source uri: source_uri is empty")
	}
	return sourceURI, nil
}

func runtimeMediaTypeFromContentType(contentType string) string {
	ct := strings.TrimSpace(strings.ToLower(contentType))
	switch {
	case strings.HasPrefix(ct, "image/"):
		return "image"
	case strings.HasPrefix(ct, "text/"):
		return "text"
	case strings.HasPrefix(ct, "audio/"):
		return "audio"
	case strings.HasPrefix(ct, "video/"):
		return "video"
	default:
		return "binary"
	}
}