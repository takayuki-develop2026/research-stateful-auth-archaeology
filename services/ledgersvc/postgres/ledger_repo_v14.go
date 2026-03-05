package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNotFound         = errors.New("not_found")
	ErrAlreadyExists    = errors.New("already_exists")
	ErrUnknownAccount   = errors.New("unknown_or_inactive_account")
	ErrInvariantFailed  = errors.New("ledger_invariant_failed")
	ErrInvalidArgument  = errors.New("invalid_argument")
	ErrPermissionDenied = errors.New("permission_denied")
	ErrPostingNotFound  = errors.New("posting_not_found")
)

type RepoV14 struct {
	pool *pgxpool.Pool
}

func NewRepoV14(pool *pgxpool.Pool) *RepoV14 {
	return &RepoV14{pool: pool}
}

type PostingCreateParams struct {
	ProjectID       string
	PostingKey      string
	SourceEventKey  string
	PostingType     string // ledger_posting_type_v14 enum text: sale/refund/fee/...
	Currency        string
	PostedAt        time.Time
	RunID           string
	TraceID         string
	PolicyVersionID string
	EvidenceRefs    []string
}

type PostingCreateResult struct {
	PostingID string // uuid text
	Status    string // "created" | "already_exists"
}

type EntryInput struct {
	AccountKey   string   `json:"account_key"`
	Direction    string   `json:"direction"` // "debit" | "credit"
	Amount       int64    `json:"amount"`
	Currency     string   `json:"currency"`
	EntryKey     string   `json:"entry_key"`
	EvidenceRefs []string `json:"evidence_refs,omitempty"`
}

type FinalizeResult struct {
	PostingID   string
	Status      string // posted | failed_recorded | ...
	DebitTotal  int64
	CreditTotal int64
}

// marshalJSONBArrayOrEmpty guarantees JSON array bytes ("[]") even when slice is nil.
// This is critical because json.Marshal(nilSlice) => "null" (NOT array).


// CreatePosting => ledger_posting_create_v14 (EXECUTE ONLY)
func (r *RepoV14) CreatePosting(ctx context.Context, p PostingCreateParams) (PostingCreateResult, error) {
	if p.ProjectID == "" || p.PostingKey == "" || p.SourceEventKey == "" || p.PostingType == "" ||
		p.Currency == "" || p.RunID == "" || p.TraceID == "" || p.PolicyVersionID == "" || p.PostedAt.IsZero() {
		return PostingCreateResult{}, fmt.Errorf("%w: required field missing", ErrInvalidArgument)
	}

	ev, err := marshalJSONBArrayOrEmpty(p.EvidenceRefs)
	if err != nil {
		return PostingCreateResult{}, fmt.Errorf("%w: evidence_refs marshal: %v", ErrInvalidArgument, err)
	}

	const q = `
SELECT posting_id::text, status
FROM ledger_posting_create_v14(
  $1::text,  -- project_id
  $2::text,  -- posting_key
  $3::text,  -- source_event_key
  $4::ledger_posting_type_v14,
  $5::text,  -- currency
  $6::timestamptz, -- posted_at
  $7::text,  -- run_id
  $8::text,  -- trace_id
  $9::text,  -- policy_version_id
  $10::jsonb -- evidence_refs
);`

	var out PostingCreateResult
	err = r.pool.QueryRow(ctx, q,
		p.ProjectID,
		p.PostingKey,
		p.SourceEventKey,
		p.PostingType,
		p.Currency,
		p.PostedAt,
		p.RunID,
		p.TraceID,
		p.PolicyVersionID,
		ev, // ✅ []byte JSON ARRAY
	).Scan(&out.PostingID, &out.Status)

	if err != nil {
		return PostingCreateResult{}, mapPgErr(err)
	}
	return out, nil
}

// InsertEntries => ledger_entries_insert_v14 (EXECUTE ONLY)
func (r *RepoV14) InsertEntries(ctx context.Context, postingID string, entries []EntryInput) error {
	if postingID == "" {
		return fmt.Errorf("%w: posting_id is required", ErrInvalidArgument)
	}
	if len(entries) == 0 {
		return fmt.Errorf("%w: entries is required", ErrInvalidArgument)
	}

	// entries is not nil (len > 0), so it will be a JSON array.
	b, err := json.Marshal(entries)
	if err != nil {
		return fmt.Errorf("%w: entries marshal: %v", ErrInvalidArgument, err)
	}
	if string(b) == "null" {
		// defensive: should never happen, but keep fail-closed sanity
		b = []byte("[]")
	}

	const q = `SELECT ledger_entries_insert_v14($1::uuid, $2::jsonb);`
	_, execErr := r.pool.Exec(ctx, q, postingID, b) // ✅ []byte
	return mapPgErr(execErr)
}

// FinalizePosting => ledger_posting_finalize_v14 (EXECUTE ONLY)
func (r *RepoV14) FinalizePosting(ctx context.Context, postingID string, appendEvidenceRefs []string) (FinalizeResult, error) {
	if postingID == "" {
		return FinalizeResult{}, fmt.Errorf("%w: posting_id is required", ErrInvalidArgument)
	}

	ev, err := marshalJSONBArrayOrEmpty(appendEvidenceRefs)
	if err != nil {
		return FinalizeResult{}, fmt.Errorf("%w: append_evidence_refs marshal: %v", ErrInvalidArgument, err)
	}

	const q = `
SELECT posting_id::text, status::text, debit_total, credit_total
FROM ledger_posting_finalize_v14($1::uuid, $2::jsonb);`

	var out FinalizeResult
	err = r.pool.QueryRow(ctx, q, postingID, ev).
		Scan(&out.PostingID, &out.Status, &out.DebitTotal, &out.CreditTotal)
	if err != nil {
		return FinalizeResult{}, mapPgErr(err)
	}
	return out, nil
}

// ---- smoke-only helper (NOT exec-only) ----
func (r *RepoV14) UpsertAccountForSmoke(ctx context.Context, projectID, accountKey, accountType, currency, ownerType, ownerID string) error {
	if projectID == "" || accountKey == "" || accountType == "" || currency == "" || ownerType == "" {
		return fmt.Errorf("%w: required field missing", ErrInvalidArgument)
	}

	const q = `
INSERT INTO ledger_accounts(project_id, account_key, account_type, currency, owner_type, owner_id, status)
VALUES ($1,$2,$3::ledger_account_type_v14,$4,$5::ledger_owner_type_v14,$6,'active')
ON CONFLICT (project_id, account_key)
DO UPDATE SET
  account_type = EXCLUDED.account_type,
  currency = EXCLUDED.currency,
  owner_type = EXCLUDED.owner_type,
  owner_id = EXCLUDED.owner_id,
  status = 'active',
  updated_at = now();`

	var owner any = nil
	if ownerID != "" {
		owner = ownerID
	}
	_, err := r.pool.Exec(ctx, q, projectID, accountKey, accountType, currency, ownerType, owner)
	return mapPgErr(err)
}

func mapPgErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		if pgErr.Code == "42501" { // insufficient_privilege
			return fmt.Errorf("%w: %s", ErrPermissionDenied, pgErr.Message)
		}
		if pgErr.Code == "23505" { // unique_violation
			return fmt.Errorf("%w: %s", ErrAlreadyExists, pgErr.ConstraintName)
		}

		msg := pgErr.Message
		if strings.Contains(msg, "unknown_or_inactive_account") {
			return fmt.Errorf("%w: %s", ErrUnknownAccount, msg)
		}
		if strings.Contains(msg, "posting not found") {
			return fmt.Errorf("%w: %s", ErrPostingNotFound, msg)
		}
		if strings.Contains(msg, "is required") || strings.Contains(msg, "must be") {
			return fmt.Errorf("%w: %s", ErrInvalidArgument, msg)
		}
		return fmt.Errorf("%w: %s (code=%s)", ErrInvariantFailed, msg, pgErr.Code)
	}
	return err
}