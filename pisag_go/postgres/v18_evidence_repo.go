package postgres

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"example.com/pisag_go/run"
)

type EvidenceV18Repository struct{ db *sql.DB }

func NewEvidenceV18Repository(db *sql.DB) *EvidenceV18Repository {
	return &EvidenceV18Repository{db: db}
}

// Register calls public.evidence_register_v18(...) and returns (evidence_ref, found_existing).
func (r *EvidenceV18Repository) Register(ctx context.Context, in run.EvidenceRegisterInput) (run.EvidenceRegisterResult, error) {
	// ---- validate (hard-fail) ----
	projectID := strings.TrimSpace(in.ProjectID)
	if projectID == "" {
		return run.EvidenceRegisterResult{}, errors.New("project_id is required")
	}
	traceID := strings.TrimSpace(in.TraceID)
	if traceID == "" {
		return run.EvidenceRegisterResult{}, errors.New("trace_id is required")
	}
	actorType := strings.TrimSpace(in.ActorType)
	if actorType == "" {
		return run.EvidenceRegisterResult{}, errors.New("actor_type is required")
	}

	mediaType := strings.TrimSpace(in.MediaType)
	if mediaType == "" {
		return run.EvidenceRegisterResult{}, errors.New("media_type is required")
	}
	mimeType := strings.TrimSpace(in.MimeType)
	if mimeType == "" {
		return run.EvidenceRegisterResult{}, errors.New("mime_type is required")
	}
	sourceKind := strings.TrimSpace(in.SourceKind)
	if sourceKind == "" {
		return run.EvidenceRegisterResult{}, errors.New("source_kind is required")
	}
	contentSHA := strings.TrimSpace(in.ContentSHA256)
	if contentSHA == "" {
		return run.EvidenceRegisterResult{}, errors.New("content_sha256 is required")
	}
	if in.ContentLength < 0 {
		return run.EvidenceRegisterResult{}, errors.New("content_length must be >= 0")
	}
	idem := strings.TrimSpace(in.IdempotencyKey)
	if idem == "" {
		return run.EvidenceRegisterResult{}, errors.New("idempotency_key is required")
	}

	var actorID sql.NullString
	if in.ActorID != nil && strings.TrimSpace(*in.ActorID) != "" {
		actorID = sql.NullString{String: strings.TrimSpace(*in.ActorID), Valid: true}
	}

	var sourceURI sql.NullString
	if in.SourceURI != nil && strings.TrimSpace(*in.SourceURI) != "" {
		sourceURI = sql.NullString{String: strings.TrimSpace(*in.SourceURI), Valid: true}
	}

	var lang sql.NullString
	if in.Language != nil && strings.TrimSpace(*in.Language) != "" {
		lang = sql.NullString{String: strings.TrimSpace(*in.Language), Valid: true}
	}

	ret := strings.TrimSpace(in.RetentionPolicy)
	if ret == "" {
		return run.EvidenceRegisterResult{}, errors.New("retention_policy is required")
	}

	var expiresAt sql.NullTime
	if in.ExpiresAtUTC != nil && strings.TrimSpace(*in.ExpiresAtUTC) != "" {
		// allow RFC3339/RFC3339Nano strings. DB expects timestamptz.
		t, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(*in.ExpiresAtUTC))
		if err != nil {
			// try RFC3339
			t2, err2 := time.Parse(time.RFC3339, strings.TrimSpace(*in.ExpiresAtUTC))
			if err2 != nil {
				return run.EvidenceRegisterResult{}, errors.New("expires_at_utc must be RFC3339/RFC3339Nano")
			}
			t = t2
		}
		expiresAt = sql.NullTime{Time: t.UTC(), Valid: true}
	}

	const q = `
SELECT evidence_ref::text, found_existing
FROM public.evidence_register_v18(
  $1::varchar,          -- project_id
  $2::varchar,          -- trace_id
  $3::varchar,          -- actor_type
  $4::varchar,          -- actor_id
  $5::varchar,          -- media_type
  $6::varchar,          -- mime_type
  $7::varchar,          -- source_kind
  $8::text,             -- source_uri
  $9::text,             -- content_sha256
  $10::bigint,          -- content_length
  $11::varchar,         -- language
  $12::varchar,         -- retention_policy
  $13::timestamptz,     -- expires_at_utc
  $14::text             -- idempotency_key
);
`

	var evidenceRef string
	var found bool

	err := r.db.QueryRowContext(
		ctx,
		q,
		projectID,
		traceID,
		actorType,
		nullString(actorID),
		mediaType,
		mimeType,
		sourceKind,
		nullString(sourceURI),
		contentSHA,
		in.ContentLength,
		nullString(lang),
		ret,
		nullTime(expiresAt),
		idem,
	).Scan(&evidenceRef, &found)
	if err != nil {
		return run.EvidenceRegisterResult{}, err
	}

	return run.EvidenceRegisterResult{
		EvidenceRef:   evidenceRef,
		FoundExisting: found,
	}, nil
}

func nullString(ns sql.NullString) any {
	if ns.Valid {
		return ns.String
	}
	return nil
}
func nullTime(nt sql.NullTime) any {
	if nt.Valid {
		return nt.Time
	}
	return nil
}
