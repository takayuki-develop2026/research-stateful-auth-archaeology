package usecase

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"ledgersvc/postgres"
)

type UtlRangeRepo interface {
	ListRange(ctx context.Context, projectID string, from, to time.Time, status *string, limit int) ([]postgres.UtlRangeItemV61, error)
}

type UtlStatusRepo interface {
	MarkProcessed(ctx context.Context, projectID, eventKey, traceID string, runID *string) (string, error)
	MarkNeedsRetry(ctx context.Context, projectID, eventKey, traceID string, runID *string, errorCode *string, errorEvidenceAssetID *int64) (string, error)
}

type EvidenceRepo interface {
	Register(ctx context.Context, p postgres.EvidenceRegisterParams) (postgres.EvidenceRegisterResult, error)
}

type V1412UtlToLedgerRange struct {
	ingest    IngestRepo
	utlRange  UtlRangeRepo
	utlStatus UtlStatusRepo
	evidence  EvidenceRepo
	ledger    LedgerRepo

	debitAccountKey  string
	creditAccountKey string
}

func NewV1412UtlToLedgerRange(ingest IngestRepo, utlRange UtlRangeRepo, utlStatus UtlStatusRepo, evidence EvidenceRepo, ledger LedgerRepo) *V1412UtlToLedgerRange {
	return &V1412UtlToLedgerRange{
		ingest:          ingest,
		utlRange:        utlRange,
		utlStatus:       utlStatus,
		evidence:        evidence,
		ledger:          ledger,
		debitAccountKey:  "platform:cash:clearing",
		creditAccountKey: "platform:revenue:sales",
	}
}

type V1412Input struct {
	ProjectID       string
	PolicyVersionID string

	FromTS time.Time
	ToTS   time.Time
	StatusFilter *string
	Limit int

	IdempotencyKey string
	RunID          string
	TraceID        string
}

type V1412Output struct {
	IngestRunID          string
	Processed            int
	Posted               int
	AlreadyExists        int
	Failed               int
	UtlMarkedProcessed   int
	UtlMarkedNeedsRetry  int
	RejectEvidenceRef    string // uuid string (optional)
	RejectEvidenceID     int64  // bigint (optional)
}

type RejectItem struct {
	UtlEventID   int64  `json:"utl_event_id"`
	EventKey     string `json:"event_key"`
	EventName    string `json:"event_name"`
	PostingKey   string `json:"posting_key"`
	ReceivedAt   string `json:"received_at"`
	ReasonCode   string `json:"reason_code"`
	Detail       any    `json:"detail,omitempty"`
}

func (uc *V1412UtlToLedgerRange) RunOnce(ctx context.Context, in V1412Input) (V1412Output, error) {
	acc, err := uc.ingest.Accept(ctx, postgres.IngestAcceptParams{
		ProjectID:       in.ProjectID,
		Mode:            "range",
		SourceEventKey:  "",
		FromTS:          &in.FromTS,
		ToTS:            &in.ToTS,
		Filter:          map[string]any{"v": "v6.2+v14.1.2.1", "status": derefStr(in.StatusFilter)},
		IdempotencyKey:  in.IdempotencyKey,
		RunID:           in.RunID,
		TraceID:         in.TraceID,
		PolicyVersionID: in.PolicyVersionID,
		EvidenceRefs:    nil,
	})
	if err != nil {
		return V1412Output{}, fmt.Errorf("accept(range): %w", err)
	}

	claimed, err := uc.ingest.ClaimNext(ctx, in.ProjectID)
	if err != nil {
		return V1412Output{}, fmt.Errorf("claim_next: %w", err)
	}
	if claimed == nil {
		return V1412Output{IngestRunID: acc.IngestRunID}, nil
	}

	_ = uc.ingest.Touch(ctx, claimed.IngestRunID)

	limit := in.Limit
	if limit <= 0 {
		limit = 500
	}
	items, err := uc.utlRange.ListRange(ctx, in.ProjectID, in.FromTS, in.ToTS, in.StatusFilter, limit)
	if err != nil {
		_ = uc.ingest.MarkFailedRecorded(ctx, claimed.IngestRunID,
			map[string]any{"failed": "utl_list_range", "error": err.Error()},
			nil,
		)
		return V1412Output{}, fmt.Errorf("utl_list_range: %w", err)
	}

	var processed, posted, alreadyExists, failed int
	var markedProcessed, markedNeedsRetry int
	rejects := make([]RejectItem, 0, 16)

	addReject := func(it postgres.UtlRangeItemV61, reason string, detail any) {
		rejects = append(rejects, RejectItem{
			UtlEventID: it.ID,
			EventKey:   it.EventKey,
			EventName:  it.EventName,
			PostingKey: it.PostingKey,
			ReceivedAt: it.ReceivedAt.Format(time.RFC3339),
			ReasonCode: reason,
			Detail:     detail,
		})
	}

	for _, it := range items {
		processed++

		// Missing amount/currency -> reject
		if it.AmountMinor == nil || it.Currency == nil || *it.Currency == "" {
			failed++
			addReject(it, "missing_amount_or_currency", map[string]any{
				"amount_minor": it.AmountMinor,
				"currency":     it.Currency,
			})
			continue
		}

		amount := *it.AmountMinor
		ccy := *it.Currency

		postingType, mapErr := mapEventNameToPostingType(it.EventName)
		if mapErr != nil {
			failed++
			addReject(it, "unsupported_event_name", map[string]any{"error": mapErr.Error()})
			continue
		}

		createRes, err := uc.ledger.CreatePosting(ctx, postgres.PostingCreateParams{
			ProjectID:       in.ProjectID,
			PostingKey:      it.PostingKey,
			SourceEventKey:  it.EventKey,
			PostingType:     postingType,
			Currency:        ccy,
			PostedAt:        it.EventTime,
			RunID:           claimed.RunID,
			TraceID:         claimed.TraceID,
			PolicyVersionID: claimed.PolicyVersionID,
			EvidenceRefs:    nil,
		})
		if err != nil {
			failed++
			addReject(it, "ledger_create_failed", map[string]any{"error": err.Error()})
			continue
		}
		if createRes.Status == "already_exists" {
			alreadyExists++
		}

		entries := []postgres.EntryInput{
			{AccountKey: uc.debitAccountKey, Direction: "debit", Amount: amount, Currency: ccy, EntryKey: "line:1"},
			{AccountKey: uc.creditAccountKey, Direction: "credit", Amount: amount, Currency: ccy, EntryKey: "line:2"},
		}
		if err := uc.ledger.InsertEntries(ctx, createRes.PostingID, entries); err != nil {
			failed++
			addReject(it, "ledger_insert_failed", map[string]any{"error": err.Error()})
			continue
		}

		finalRes, err := uc.ledger.FinalizePosting(ctx, createRes.PostingID, nil)
		if err != nil || finalRes.Status != "posted" {
			failed++
			addReject(it, "ledger_finalize_failed", map[string]any{
				"error":  errString(err),
				"status": finalRes.Status,
				"debit":  finalRes.DebitTotal,
				"credit": finalRes.CreditTotal,
			})
			continue
		}

		posted++

		// SUCCESS: mark processed (if fails -> reject)
		st, perr := uc.utlStatus.MarkProcessed(ctx, in.ProjectID, it.EventKey, claimed.TraceID, &claimed.RunID)
		if perr != nil {
			failed++
			addReject(it, "utl_mark_processed_failed", map[string]any{"error": perr.Error()})
			continue
		}
		if st == "processed" {
			markedProcessed++
		}
	}

	// If any rejects exist, create ONE reject list evidence and link it to ingest_run + UTL needs_retry.
	var rejectEvidenceRef string
	var rejectEvidenceID int64 = 0

	if len(rejects) > 0 {
		report := map[string]any{
			"kind":           "ledger_ingest_reject_list_v14.1.2",
			"project_id":      in.ProjectID,
			"ingest_run_id":   claimed.IngestRunID,
			"from_ts":         in.FromTS.Format(time.RFC3339),
			"to_ts":           in.ToTS.Format(time.RFC3339),
			"status_filter":   derefStr(in.StatusFilter),
			"generated_at":    time.Now().UTC().Format(time.RFC3339Nano),
			"reject_count":    len(rejects),
			"rejects":         rejects,
		}
		b, _ := json.Marshal(report)

		idem := fmt.Sprintf("ledger_ingest_reject_list:%s:%s", in.ProjectID, claimed.IngestRunID)
		evr, evErr := uc.evidence.Register(ctx, postgres.EvidenceRegisterParams{
			ProjectID:       in.ProjectID,
			TraceID:         claimed.TraceID, // v18 expects text
			ActorType:       "service",
			ActorID:         "ledgersvc",
			MediaType:       "text",
			MimeType:        "application/json",
			SourceKind:      "generated",
			SourceURI:       fmt.Sprintf("ledgersvc://v14.1.2/reject_list/%s", claimed.IngestRunID),
			Language:        "ja",
			RetentionPolicy: "standard",
			IdempotencyKey:  idem,
			ContentBytes:    b,
		})
		if evErr == nil {
			rejectEvidenceRef = evr.EvidenceRef
			rejectEvidenceID = evr.EvidenceID
		}
	}

	// For rejects: mark needs_retry with same evidence id (if available).
	if rejectEvidenceID > 0 {
		for _, rj := range rejects {
			ec := rj.ReasonCode
			_, e := uc.utlStatus.MarkNeedsRetry(ctx, in.ProjectID, rj.EventKey, claimed.TraceID, &claimed.RunID, &ec, &rejectEvidenceID)
			if e == nil {
				markedNeedsRetry++
			}
		}
	} else {
		// If we couldn't create evidence, still mark needs_retry without evidence id.
		for _, rj := range rejects {
			ec := rj.ReasonCode
			_, e := uc.utlStatus.MarkNeedsRetry(ctx, in.ProjectID, rj.EventKey, claimed.TraceID, &claimed.RunID, &ec, nil)
			if e == nil {
				markedNeedsRetry++
			}
		}
	}

	stats := map[string]any{
		"event_count":            len(items),
		"processed_count":        processed,
		"posted_count":           posted,
		"already_exists_count":   alreadyExists,
		"failed_count":           failed,
		"utl_marked_processed":   markedProcessed,
		"utl_marked_needs_retry": markedNeedsRetry,
		"reject_evidence_ref":    rejectEvidenceRef,
		"reject_evidence_id":     rejectEvidenceID,
		"from_ts":                in.FromTS.Format(time.RFC3339),
		"to_ts":                  in.ToTS.Format(time.RFC3339),
		"status_filter":          derefStr(in.StatusFilter),
		"limit":                  limit,
	}

	appendRefs := []string{}
	if rejectEvidenceRef != "" {
		// v14.1 ingest_run evidence_refs: store evidence_ref (uuid string)
		appendRefs = []string{rejectEvidenceRef}
	}

	if failed > 0 {
		_ = uc.ingest.MarkFailedRecorded(ctx, claimed.IngestRunID, stats, appendRefs)
	} else {
		_ = uc.ingest.MarkSucceeded(ctx, claimed.IngestRunID, stats, appendRefs)
	}

	return V1412Output{
		IngestRunID:         acc.IngestRunID,
		Processed:           processed,
		Posted:              posted,
		AlreadyExists:       alreadyExists,
		Failed:              failed,
		UtlMarkedProcessed:  markedProcessed,
		UtlMarkedNeedsRetry: markedNeedsRetry,
		RejectEvidenceRef:   rejectEvidenceRef,
		RejectEvidenceID:    rejectEvidenceID,
	}, nil
}

func errString(err error) any {
	if err == nil {
		return nil
	}
	return err.Error()
}

func derefStr(p *string) any {
	if p == nil || *p == "" {
		return nil
	}
	return *p
}