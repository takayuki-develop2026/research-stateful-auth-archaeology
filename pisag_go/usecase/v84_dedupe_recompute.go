package usecase

import (
	"context"
	"errors"
	"strings"

	"example.com/pisag_go/run"
)

type DedupeRecomputeUsecase struct {
	Candidates run.DiscoveryCandidateRepo
	Dedupe     run.DedupeRepo
}

func NewDedupeRecomputeUsecase(c run.DiscoveryCandidateRepo, d run.DedupeRepo) *DedupeRecomputeUsecase {
	return &DedupeRecomputeUsecase{Candidates: c, Dedupe: d}
}

// NOTE: You define namespace/identity rules; here expects caller supplies dedupe_key per candidate.
// For shortest path: compute dedupe_key = sha256(project_id|type|namespace|identity) in your service layer
// and write back into discovery_candidates (UPDATE) + group linkage.
func (uc *DedupeRecomputeUsecase) Handle(ctx context.Context, projectID, runID, traceID string, candidateIDs []int64) error {
	projectID = strings.TrimSpace(projectID)
	runID = strings.TrimSpace(runID)
	traceID = strings.TrimSpace(traceID)
	if projectID == "" || runID == "" || traceID == "" {
		return errors.New("project_id/run_id/trace_id are required")
	}

	// Implementation note:
	// 1) Load candidates by IDs
	// 2) For each: compute dedupe_key (deterministic) + UPDATE candidate.dedupe_key
	// 3) Upsert group, link member
	// 4) If group size >=2 => mark review_required
	return nil
}
