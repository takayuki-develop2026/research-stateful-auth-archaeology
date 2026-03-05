package usecase

import (
	"context"

	"ledgersvc/postgres"
)

// Shared contracts for usecase package (avoid redeclare collisions)

type LedgerRepo interface {
	CreatePosting(ctx context.Context, p postgres.PostingCreateParams) (postgres.PostingCreateResult, error)
	InsertEntries(ctx context.Context, postingID string, entries []postgres.EntryInput) error
	FinalizePosting(ctx context.Context, postingID string, appendEvidenceRefs []string) (postgres.FinalizeResult, error)
}

type IngestRepo interface {
	Accept(ctx context.Context, p postgres.IngestAcceptParams) (postgres.IngestAcceptResult, error)
	ClaimNext(ctx context.Context, projectID string) (*postgres.IngestClaimResult, error)
	Touch(ctx context.Context, ingestRunID string) error
	MarkSucceeded(ctx context.Context, ingestRunID string, stats map[string]any, appendEvidenceRefs []string) error
	MarkFailedRecorded(ctx context.Context, ingestRunID string, stats map[string]any, appendEvidenceRefs []string) error
}

type UtlRepo interface {
	GetByEventKey(ctx context.Context, projectID, eventKey string) (*postgres.UtlEventV6, error)
}