package usecase

import (
	"context"
	"errors"
	"fmt"
	"time"

	"ledgersvc/postgres"
)

var (
	ErrUtlMissingAmountCurrency = errors.New("utl_missing_amount_currency")
	ErrUnsupportedEventName     = errors.New("unsupported_event_name")
)



type V1411UtlToLedgerSingleEvent struct {
	ingest IngestRepo
	utl    UtlRepo
	ledger LedgerRepo

	// minimal mapping config (v14.1.1)
	debitAccountKey  string
	creditAccountKey string
}

func NewV1411UtlToLedgerSingleEvent(ingest IngestRepo, utl UtlRepo, ledger LedgerRepo) *V1411UtlToLedgerSingleEvent {
	return &V1411UtlToLedgerSingleEvent{
		ingest:          ingest,
		utl:            utl,
		ledger:          ledger,
		debitAccountKey:  "platform:cash:clearing",
		creditAccountKey: "platform:revenue:sales",
	}
}

type V1411SmokeInput struct {
	ProjectID       string
	PolicyVersionID string
	UTLEventKey     string

	// orchestration metadata
	IdempotencyKey string
	RunID          string
	TraceID        string
}

type V1411SmokeOutput struct {
	IngestRunID string
	LedgerPostingID string
	LedgerPostingKey string
}

func (uc *V1411UtlToLedgerSingleEvent) RunOnce(ctx context.Context, in V1411SmokeInput) (V1411SmokeOutput, error) {
	// 1) accept ingest run
	acc, err := uc.ingest.Accept(ctx, postgres.IngestAcceptParams{
		ProjectID:       in.ProjectID,
		Mode:            "single_event",
		SourceEventKey:  in.UTLEventKey, // store UTL event_key here
		IdempotencyKey:  in.IdempotencyKey,
		RunID:           in.RunID,
		TraceID:         in.TraceID,
		PolicyVersionID: in.PolicyVersionID,
		Filter:          map[string]any{"v": "v14.1.1-smoke"},
		EvidenceRefs:    nil,
	})
	if err != nil {
		return V1411SmokeOutput{}, fmt.Errorf("accept: %w", err)
	}

	// 2) claim
	claimed, err := uc.ingest.ClaimNext(ctx, in.ProjectID)
	if err != nil {
		return V1411SmokeOutput{}, fmt.Errorf("claim_next: %w", err)
	}
	if claimed == nil {
		// already running/succeeded; idempotent accept may have produced an existing run that is not claimable now.
		return V1411SmokeOutput{IngestRunID: acc.IngestRunID}, nil
	}

	// 3) touch
	_ = uc.ingest.Touch(ctx, claimed.IngestRunID)

	// 4) fetch UTL event
	if claimed.SourceEventKey == nil || *claimed.SourceEventKey == "" {
		_ = uc.ingest.MarkFailedRecorded(ctx, claimed.IngestRunID,
			map[string]any{"failed": "missing_source_event_key"},
			nil,
		)
		return V1411SmokeOutput{}, fmt.Errorf("claimed run missing source_event_key")
	}

	utlEv, err := uc.utl.GetByEventKey(ctx, in.ProjectID, *claimed.SourceEventKey)
	if err != nil {
		_ = uc.ingest.MarkFailedRecorded(ctx, claimed.IngestRunID,
			map[string]any{"failed": "utl_get_event", "error": err.Error()},
			nil,
		)
		return V1411SmokeOutput{}, fmt.Errorf("utl_get_event: %w", err)
	}

	// 5) validate amount/currency
	if utlEv.AmountMinor == nil || utlEv.Currency == nil || *utlEv.Currency == "" {
		_ = uc.ingest.MarkFailedRecorded(ctx, claimed.IngestRunID,
			map[string]any{
				"event_key": utlEv.EventKey,
				"failed":    "missing_amount_or_currency",
			},
			nil,
		)
		return V1411SmokeOutput{}, ErrUtlMissingAmountCurrency
	}

	amount := *utlEv.AmountMinor
	ccy := *utlEv.Currency

	// 6) map event_name -> posting_type (minimal)
	postingType, err := mapEventNameToPostingType(utlEv.EventName)
	if err != nil {
		_ = uc.ingest.MarkFailedRecorded(ctx, claimed.IngestRunID,
			map[string]any{"failed": "unsupported_event_name", "event_name": utlEv.EventName},
			nil,
		)
		return V1411SmokeOutput{}, err
	}

	// 7) create ledger posting using UTL posting_key (v6 invariant)
	postedAt := utlEv.EventTime
	// v14.0 requires run_id/trace_id text; use ingest run fields (already text)
	createRes, err := uc.ledger.CreatePosting(ctx, postgres.PostingCreateParams{
		ProjectID:       in.ProjectID,
		PostingKey:      utlEv.PostingKey,
		SourceEventKey:  utlEv.EventKey,
		PostingType:     postingType,
		Currency:        ccy,
		PostedAt:        postedAt,
		RunID:           claimed.RunID,
		TraceID:         claimed.TraceID,
		PolicyVersionID: claimed.PolicyVersionID,
		EvidenceRefs:    nil,
	})
	if err != nil {
		_ = uc.ingest.MarkFailedRecorded(ctx, claimed.IngestRunID,
			map[string]any{"failed": "ledger_create_posting", "error": err.Error()},
			nil,
		)
		return V1411SmokeOutput{}, fmt.Errorf("ledger_create_posting: %w", err)
	}

	// 8) insert entries (double-entry)
	entries := []postgres.EntryInput{
		{
			AccountKey: uc.debitAccountKey,
			Direction:  "debit",
			Amount:     amount,
			Currency:   ccy,
			EntryKey:   "line:1",
		},
		{
			AccountKey: uc.creditAccountKey,
			Direction:  "credit",
			Amount:     amount,
			Currency:   ccy,
			EntryKey:   "line:2",
		},
	}
	if err := uc.ledger.InsertEntries(ctx, createRes.PostingID, entries); err != nil {
		_ = uc.ingest.MarkFailedRecorded(ctx, claimed.IngestRunID,
			map[string]any{"failed": "ledger_insert_entries", "error": err.Error()},
			nil,
		)
		return V1411SmokeOutput{}, fmt.Errorf("ledger_insert_entries: %w", err)
	}

	// 9) finalize (zero-sum enforced DB-side; fail-closed)
	finalRes, err := uc.ledger.FinalizePosting(ctx, createRes.PostingID, nil)
	if err != nil {
		_ = uc.ingest.MarkFailedRecorded(ctx, claimed.IngestRunID,
			map[string]any{"failed": "ledger_finalize", "error": err.Error()},
			nil,
		)
		return V1411SmokeOutput{}, fmt.Errorf("ledger_finalize: %w", err)
	}
	if finalRes.Status != "posted" {
		_ = uc.ingest.MarkFailedRecorded(ctx, claimed.IngestRunID,
			map[string]any{"failed": "ledger_not_posted", "status": finalRes.Status, "debit": finalRes.DebitTotal, "credit": finalRes.CreditTotal},
			nil,
		)
		return V1411SmokeOutput{}, fmt.Errorf("ledger_not_posted: status=%s debit=%d credit=%d", finalRes.Status, finalRes.DebitTotal, finalRes.CreditTotal)
	}

	// 10) mark succeeded
	stats := map[string]any{
		"event_count":          1,
		"posted_count":         1,
		"already_exists_count": ifAlreadyExists(createRes.Status),
		"failed_count":         0,
		"utl_event_id":         utlEv.ID,
		"utl_event_key":        utlEv.EventKey,
		"ledger_posting_id":    finalRes.PostingID,
		"ledger_posting_key":   utlEv.PostingKey,
		"amount_minor":         amount,
		"currency":             ccy,
	}
	_ = uc.ingest.MarkSucceeded(ctx, claimed.IngestRunID, stats, nil)

	return V1411SmokeOutput{
		IngestRunID: acc.IngestRunID,
		LedgerPostingID: finalRes.PostingID,
		LedgerPostingKey: utlEv.PostingKey,
	}, nil
}

func ifAlreadyExists(status string) int {
	if status == "already_exists" {
		return 1
	}
	return 0
}

func mapEventNameToPostingType(eventName string) (string, error) {
	// minimal mapping for v14.1.1 smoke
	switch eventName {
	case "sale.succeeded", "routing.committed":
		// For smoke, treat routing.committed as sale only if amount/currency exists; otherwise earlier validation fails.
		return "sale", nil
	default:
		return "", ErrUnsupportedEventName
	}
}

// convenience: for deterministic smoke metadata
func NowRunID() string  { return fmt.Sprintf("run_v1411_%d", time.Now().UnixNano()) }
func NowTraceID() string { return fmt.Sprintf("trace_v1411_%d", time.Now().UnixNano()) }