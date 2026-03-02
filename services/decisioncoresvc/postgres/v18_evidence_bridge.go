package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

type EvidenceRegisterV18Input struct {
	ProjectID       string
	TraceID         string
	ActorType       string
	ActorID         string // v18 function accepts character varying; empty allowed
	MediaType       string
	MimeType        string
	SourceKind      string
	SourceURI       string // text; empty allowed
	ContentSHA256   string // must be 64 lowercase hex
	ContentLength   int64  // must be > 0
	Language        string // empty allowed
	RetentionPolicy string // short|standard|legal_hold (empty -> standard)
	ExpiresAtUTC    *time.Time
	IdempotencyKey  string
}

type EvidenceRegisterV18Result struct {
	EvidenceRef    string // uuid text
	FoundExisting  bool
	EvidenceAssetID int64 // resolved from evidence_ref
}

type EvidenceV18Bridge struct {
	db *sql.DB
}

func NewEvidenceV18Bridge(db *sql.DB) *EvidenceV18Bridge {
	return &EvidenceV18Bridge{db: db}
}

// Register calls exec-only v18 function and resolves evidence_assets.id by evidence_ref.
// NOTE: evidence_register_v18 dedups by (project_id, content_sha256).
func (b *EvidenceV18Bridge) Register(ctx context.Context, in EvidenceRegisterV18Input) (EvidenceRegisterV18Result, error) {
	var evidenceRef string
	var found bool

	err := b.db.QueryRowContext(ctx, `
SELECT evidence_ref::text, found_existing
FROM public.evidence_register_v18(
  $1,$2,$3,$4,
  $5,$6,$7,$8,
  $9,$10,$11,$12,
  $13,$14
)
`,
		in.ProjectID,
		in.TraceID,
		in.ActorType,
		in.ActorID,
		in.MediaType,
		in.MimeType,
		in.SourceKind,
		nullIfEmpty(in.SourceURI),
		in.ContentSHA256,
		in.ContentLength,
		nullIfEmpty(in.Language),
		nullIfEmpty(in.RetentionPolicy),
		in.ExpiresAtUTC,
		in.IdempotencyKey,
	).Scan(&evidenceRef, &found)
	if err != nil {
		return EvidenceRegisterV18Result{}, fmt.Errorf("evidence_register_v18 failed: %w", err)
	}

	// Resolve evidence_assets.id by (project_id, evidence_ref)
	var assetID int64
	err = b.db.QueryRowContext(ctx, `
SELECT id
FROM public.evidence_assets
WHERE project_id = $1 AND evidence_ref = $2::uuid
LIMIT 1
`, in.ProjectID, evidenceRef).Scan(&assetID)
	if err != nil {
		return EvidenceRegisterV18Result{}, fmt.Errorf("resolve evidence_assets.id failed: %w", err)
	}

	return EvidenceRegisterV18Result{
		EvidenceRef:     evidenceRef,
		FoundExisting:   found,
		EvidenceAssetID: assetID,
	}, nil
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}