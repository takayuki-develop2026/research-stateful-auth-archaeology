package postgres

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

type UtlRepoV6 struct{ db *sql.DB }

func NewUtlRepoV6(db *sql.DB) *UtlRepoV6 { return &UtlRepoV6{db: db} }

type UtlIngestInputV6 struct {
	ProjectID string

	EventSource     string // webhook|internal
	Provider        string // stripe|adyen|internal
	ProviderEventID *string
	EventName       string

	EventTime  *time.Time
	ReceivedAt *time.Time

	CorrelationID *string
	EventSeq      *int

	TraceID string  // uuid text
	RunID   *string // uuid text

	AmountMinor      *int64
	Currency         *string // char(3)
	Region           *string
	InternalObjectID *string
	ProviderObjectID *string

	PayloadEvidenceAssetID *int64 // bigint evidence_assets.id
}

type UtlIngestResultV6 struct {
	UtlEventID int64
	Status     string
	EventKey   string
	PostingKey string
}

// Resolve evidence_assets.id by (project_id, evidence_ref uuid)
func (r *UtlRepoV6) ResolveEvidenceAssetIDByRef(ctx context.Context, projectID, evidenceRef string) (int64, error) {
	projectID = strings.TrimSpace(projectID)
	evidenceRef = strings.TrimSpace(evidenceRef)
	if projectID == "" || evidenceRef == "" {
		return 0, errors.New("project_id and evidence_ref are required")
	}

	const q = `
SELECT id
FROM public.evidence_assets
WHERE project_id=$1 AND evidence_ref=$2::uuid
LIMIT 1;
`
	var id int64
	if err := r.db.QueryRowContext(ctx, q, projectID, evidenceRef).Scan(&id); err != nil {
		return 0, err
	}
	return id, nil
}

// Call DB function public.utl_ingest_v6(...)
// IMPORTANT: Use "SELECT *" to avoid depending on returned column names.
// (Your DB function may return id with a different column name than "utl_event_id".)
func (r *UtlRepoV6) Ingest(ctx context.Context, in UtlIngestInputV6) (UtlIngestResultV6, error) {
	in.ProjectID = strings.TrimSpace(in.ProjectID)
	in.EventSource = strings.TrimSpace(in.EventSource)
	in.Provider = strings.TrimSpace(in.Provider)
	in.EventName = strings.TrimSpace(in.EventName)
	in.TraceID = strings.TrimSpace(in.TraceID)

	if in.ProjectID == "" || in.EventSource == "" || in.Provider == "" || in.EventName == "" || in.TraceID == "" {
		return UtlIngestResultV6{}, errors.New("project_id/event_source/provider/event_name/trace_id are required")
	}

	const q = `
SELECT *
FROM public.utl_ingest_v6(
  $1::varchar,
  $2::varchar,
  $3::varchar,
  $4::varchar,
  $5::varchar,

  $6::timestamptz,
  $7::timestamptz,

  $8::varchar,
  $9::int,

  $10::uuid,
  $11::uuid,

  $12::bigint,
  $13::char(3),
  $14::varchar,
  $15::varchar,
  $16::varchar,

  $17::bigint
);
`

	// build args (nilable)
	var providerEventID any = nil
	if in.ProviderEventID != nil && strings.TrimSpace(*in.ProviderEventID) != "" {
		providerEventID = strings.TrimSpace(*in.ProviderEventID)
	}

	var eventTime any = nil
	if in.EventTime != nil {
		eventTime = *in.EventTime
	}

	var receivedAt any = nil
	if in.ReceivedAt != nil {
		receivedAt = *in.ReceivedAt
	}

	var corr any = nil
	if in.CorrelationID != nil && strings.TrimSpace(*in.CorrelationID) != "" {
		corr = strings.TrimSpace(*in.CorrelationID)
	}

	var seq any = nil
	if in.EventSeq != nil {
		seq = *in.EventSeq
	}

	var runID any = nil
	if in.RunID != nil && strings.TrimSpace(*in.RunID) != "" {
		runID = strings.TrimSpace(*in.RunID)
	}

	var amount any = nil
	if in.AmountMinor != nil {
		amount = *in.AmountMinor
	}

	var currency any = nil
	if in.Currency != nil && strings.TrimSpace(*in.Currency) != "" {
		currency = strings.TrimSpace(*in.Currency)
	}

	var region any = nil
	if in.Region != nil && strings.TrimSpace(*in.Region) != "" {
		region = strings.TrimSpace(*in.Region)
	}

	var internalObj any = nil
	if in.InternalObjectID != nil && strings.TrimSpace(*in.InternalObjectID) != "" {
		internalObj = strings.TrimSpace(*in.InternalObjectID)
	}

	var providerObj any = nil
	if in.ProviderObjectID != nil && strings.TrimSpace(*in.ProviderObjectID) != "" {
		providerObj = strings.TrimSpace(*in.ProviderObjectID)
	}

	var payloadID any = nil
	if in.PayloadEvidenceAssetID != nil && *in.PayloadEvidenceAssetID > 0 {
		payloadID = *in.PayloadEvidenceAssetID
	}

	var res UtlIngestResultV6
	if err := r.db.QueryRowContext(ctx, q,
		in.ProjectID,
		in.EventSource,
		in.Provider,
		providerEventID,
		in.EventName,

		eventTime,
		receivedAt,

		corr,
		seq,

		in.TraceID,
		runID,

		amount,
		currency,
		region,
		internalObj,
		providerObj,

		payloadID,
	).Scan(&res.UtlEventID, &res.Status, &res.EventKey, &res.PostingKey); err != nil {
		return UtlIngestResultV6{}, err
	}

	return res, nil
}
