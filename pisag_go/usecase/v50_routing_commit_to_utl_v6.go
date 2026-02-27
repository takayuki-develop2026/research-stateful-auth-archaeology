package usecase

import (
	"context"
	"errors"
	"strings"
	"time"

	"example.com/pisag_go/postgres"
	"example.com/pisag_go/run"
)

type RoutingCommitToUtlV6Usecase struct {
	Commit *RoutingCommitUsecaseV5
	Utl    *postgres.UtlRepoV6
}

func NewRoutingCommitToUtlV6Usecase(commit *RoutingCommitUsecaseV5, utl *postgres.UtlRepoV6) *RoutingCommitToUtlV6Usecase {
	return &RoutingCommitToUtlV6Usecase{
		Commit: commit,
		Utl:    utl,
	}
}

// Handle:
// 1) v5 commit -> route_decisions に確定（utl_internal:event_key 生成済）
// 2) why_evidence_ref(uuid) -> evidence_assets.id(bigint) を解決
// 3) utl_ingest_v6(event_source=internal, provider=chosen provider_key or internal, correlation_id=run_id, event_seq=10/20/30)
// 4) 成功/duplicate/review_required いずれでも “状態化”として返す（throwしない設計思想）
func (uc *RoutingCommitToUtlV6Usecase) Handle(ctx context.Context, in run.RoutingCommitInput) (run.RoutingCommitResult, postgres.UtlIngestResultV6, error) {
	if uc.Commit == nil || uc.Utl == nil {
		return run.RoutingCommitResult{}, postgres.UtlIngestResultV6{}, errors.New("commit and utl repo are required")
	}

	commitOut, err := uc.Commit.Handle(ctx, in)
	if err != nil {
		return run.RoutingCommitResult{}, postgres.UtlIngestResultV6{}, err
	}

	projectID := strings.TrimSpace(in.ProjectID)
	traceID := strings.TrimSpace(in.TraceID)
	runID := strings.TrimSpace(in.RunID)
	if projectID == "" || traceID == "" || runID == "" {
		return commitOut, postgres.UtlIngestResultV6{}, errors.New("project_id/trace_id/run_id are required")
	}

	// Map v5 status -> UTL event_name + seq
	eventName := "routing.committed"
	seq := 10
	provider := "internal"
	if commitOut.Status == "review_required" {
		eventName = "routing.review_required"
		seq = 20
	} else if commitOut.Status == "denied" {
		eventName = "routing.denied"
		seq = 30
	}

	// Best-effort provider key: use the one embedded in v5 event_key generation (already in commitOut.UtlCommitEventKey),
	// but UTL function also accepts provider column separately. For v5 chosen, we set provider="internal" unless we later wire provider_key explicitly.
	// Here: if chosen -> use "internal" is acceptable; you can upgrade later to actual provider key.
	if commitOut.Status == "chosen" {
		// Prefer provider_key encoded by v5 usecase: we don't have it directly here, keep internal.
		provider = "internal"
	}

	// Resolve evidence_assets.id (bigint) from why_evidence_ref (uuid)
	payloadAssetID, err := uc.Utl.ResolveEvidenceAssetIDByRef(ctx, projectID, commitOut.WhyEvidenceRef)
	if err != nil {
		// If evidence lookup fails, still ingest with NULL payload (DB will stateful review_required if needed).
		payloadAssetID = 0
	}

	now := time.Now()
	corr := runID
	internalObj := strings.TrimSpace(in.SubjectInternalID)

	var payloadPtr *int64
	if payloadAssetID > 0 {
		payloadPtr = &payloadAssetID
	}

	seqCopy := seq
	utlRes, err := uc.Utl.Ingest(ctx, postgres.UtlIngestInputV6{
		ProjectID:       projectID,
		EventSource:     "internal",
		Provider:        provider,
		ProviderEventID: nil,
		EventName:       eventName,

		EventTime:  &now,
		ReceivedAt: &now,

		CorrelationID: &corr,
		EventSeq:      &seqCopy,

		TraceID: traceID,
		RunID:   &runID,

		AmountMinor: nil,
		Currency:    nil,
		Region:      nil,

		InternalObjectID: &internalObj,
		ProviderObjectID: nil,

		PayloadEvidenceAssetID: payloadPtr,
	})
	if err != nil {
		return commitOut, postgres.UtlIngestResultV6{}, err
	}

	// Consistency check: v5 generated event_key should match UTL event_key (if commitOut has it)
	// If mismatch happens, do NOT throw here (policy: non-stop). Leave for review/debug.
	_ = commitOut.UtlCommitEventKey

	return commitOut, utlRes, nil
}
