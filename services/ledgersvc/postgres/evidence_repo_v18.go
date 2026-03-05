package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrEvidenceInvalidArgument  = errors.New("evidence_invalid_argument")
	ErrEvidencePermissionDenied = errors.New("evidence_permission_denied")
	ErrEvidenceInvariantFailed  = errors.New("evidence_invariant_failed")
	ErrEvidenceNotFound         = errors.New("evidence_not_found")
)

type EvidenceRepoV18 struct {
	pool     *pgxpool.Pool
	storeDir string // e.g. /var/ledgersvc/evidence
}

func NewEvidenceRepoV18(pool *pgxpool.Pool) *EvidenceRepoV18 {
	dir := os.Getenv("LEDGERSVC_EVIDENCE_DIR")
	if strings.TrimSpace(dir) == "" {
		dir = "/var/ledgersvc/evidence"
	}
	return &EvidenceRepoV18{pool: pool, storeDir: dir}
}

type EvidenceRegisterParams struct {
	ProjectID       string
	TraceID         string // character varying in DB (v18)
	ActorType       string // system|user|service
	ActorID         string // optional
	MediaType       string // text|image|audio|video|binary
	MimeType        string // e.g. application/json
	SourceKind      string // pisag_fetch|upload|webhook|generated|import
	SourceURI       string // optional pointer/uri
	Language        string // optional
	RetentionPolicy string // short|standard|legal_hold
	IdempotencyKey  string // required to dedupe

	ContentBytes []byte // used to compute sha256/length and saved for generated evidence
}

type EvidenceRegisterResult struct {
	EvidenceRef   string // uuid text
	FoundExisting bool
	EvidenceID    int64 // bigint evidence_assets.id
	ContentSHA256 string
	ContentLength int64
}

func (r *EvidenceRepoV18) Register(ctx context.Context, p EvidenceRegisterParams) (EvidenceRegisterResult, error) {
	if strings.TrimSpace(p.ProjectID) == "" || strings.TrimSpace(p.TraceID) == "" {
		return EvidenceRegisterResult{}, fmt.Errorf("%w: project_id/trace_id required", ErrEvidenceInvalidArgument)
	}
	if strings.TrimSpace(p.ActorType) == "" || strings.TrimSpace(p.MediaType) == "" || strings.TrimSpace(p.MimeType) == "" || strings.TrimSpace(p.SourceKind) == "" {
		return EvidenceRegisterResult{}, fmt.Errorf("%w: actor_type/media_type/mime_type/source_kind required", ErrEvidenceInvalidArgument)
	}
	if strings.TrimSpace(p.RetentionPolicy) == "" {
		p.RetentionPolicy = "standard"
	}
	if strings.TrimSpace(p.IdempotencyKey) == "" {
		return EvidenceRegisterResult{}, fmt.Errorf("%w: idempotency_key required", ErrEvidenceInvalidArgument)
	}

	sum := sha256.Sum256(p.ContentBytes)
	shaHex := hex.EncodeToString(sum[:])
	clen := int64(len(p.ContentBytes))

	const q = `
SELECT evidence_ref::text, found_existing
FROM public.evidence_register_v18(
  $1::varchar,  -- project_id
  $2::varchar,  -- trace_id (text)
  $3::varchar,  -- actor_type
  $4::varchar,  -- actor_id
  $5::varchar,  -- media_type
  $6::varchar,  -- mime_type
  $7::varchar,  -- source_kind
  $8::text,     -- source_uri
  $9::text,     -- content_sha256
  $10::bigint,  -- content_length
  $11::varchar, -- language
  $12::varchar, -- retention_policy
  $13::timestamptz, -- expires_at_utc
  $14::text     -- idempotency_key
);
`
	var evidenceRef string
	var found bool

	var actorID any = nil
	if strings.TrimSpace(p.ActorID) != "" {
		actorID = p.ActorID
	}
	var srcURI any = nil
	if strings.TrimSpace(p.SourceURI) != "" {
		srcURI = p.SourceURI
	}
	var lang any = nil
	if strings.TrimSpace(p.Language) != "" {
		lang = p.Language
	}
	var expires any = nil

	err := r.pool.QueryRow(ctx, q,
		p.ProjectID,
		p.TraceID,
		p.ActorType,
		actorID,
		p.MediaType,
		p.MimeType,
		p.SourceKind,
		srcURI,
		shaHex,
		clen,
		lang,
		p.RetentionPolicy,
		expires,
		p.IdempotencyKey,
	).Scan(&evidenceRef, &found)
	if err != nil {
		return EvidenceRegisterResult{}, mapEvidencePgErr(err)
	}

	// resolve bigint id
	const q2 = `
SELECT id
FROM public.evidence_assets
WHERE project_id=$1::varchar(26) AND evidence_ref=$2::uuid
LIMIT 1;
`
	var evidID int64
	err = r.pool.QueryRow(ctx, q2, p.ProjectID, evidenceRef).Scan(&evidID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return EvidenceRegisterResult{}, ErrEvidenceNotFound
		}
		return EvidenceRegisterResult{}, err
	}

	// Save generated content to local store (best-effort but should succeed in dev)
	if strings.TrimSpace(p.SourceKind) == "generated" && len(p.ContentBytes) > 0 {
		if werr := r.writeContentFile(p.ProjectID, evidenceRef, p.ContentBytes); werr != nil {
			// fail-closed for now: if you want "best-effort", change to log only
			return EvidenceRegisterResult{}, fmt.Errorf("%w: write content file: %v", ErrEvidenceInvariantFailed, werr)
		}
	}

	return EvidenceRegisterResult{
		EvidenceRef:   evidenceRef,
		FoundExisting: found,
		EvidenceID:    evidID,
		ContentSHA256: shaHex,
		ContentLength: clen,
	}, nil
}

func (r *EvidenceRepoV18) writeContentFile(projectID, evidenceRef string, b []byte) error {
	dir := filepath.Join(r.storeDir, projectID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(dir, evidenceRef+".json")
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func mapEvidencePgErr(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		if pgErr.Code == "42501" {
			return fmt.Errorf("%w: %s", ErrEvidencePermissionDenied, pgErr.Message)
		}
		msg := pgErr.Message
		if strings.Contains(msg, "required") || strings.Contains(msg, "invalid") {
			return fmt.Errorf("%w: %s", ErrEvidenceInvalidArgument, msg)
		}
		return fmt.Errorf("%w: %s (code=%s)", ErrEvidenceInvariantFailed, msg, pgErr.Code)
	}
	return err
}