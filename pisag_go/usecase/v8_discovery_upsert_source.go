package usecase

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"example.com/pisag_go/run"
)

type UpsertDiscoverySourceUsecase struct {
	Sources run.DiscoverySourceRepo

	// Optional: provide defaults centrally (avoid caller forgetting)
	DefaultPipelineVersion string // e.g. "v8.0"
	DefaultPolicyVersion   string // e.g. "p8.0-default"
}

func NewUpsertDiscoverySourceUsecase(
	sources run.DiscoverySourceRepo,
	defaultPipelineVersion string,
	defaultPolicyVersion string,
) *UpsertDiscoverySourceUsecase {
	return &UpsertDiscoverySourceUsecase{
		Sources:                sources,
		DefaultPipelineVersion: strings.TrimSpace(defaultPipelineVersion),
		DefaultPolicyVersion:   strings.TrimSpace(defaultPolicyVersion),
	}
}

// ComputeSourceHashV8 is the canonical v8 hash rule (must match DB UNIQUE convergence intent).
// source_hash = sha256hex("v8|" + project_id + "|" + source_type + "|" + source_ref)
func ComputeSourceHashV8(projectID, sourceType, sourceRef string) string {
	projectID = strings.TrimSpace(projectID)
	sourceType = strings.TrimSpace(sourceType)
	sourceRef = strings.TrimSpace(sourceRef)
	h := sha256.Sum256([]byte("v8|" + projectID + "|" + sourceType + "|" + sourceRef))
	return hex.EncodeToString(h[:])
}

// Handle validates minimal invariants and delegates to repository upsert.
// Notes:
// - This usecase does NOT normalize URLs itself; pass already-normalized source_ref.
// - If Status/FailureState are not provided, repo will keep existing (on conflict) or defaults (insert).
func (uc *UpsertDiscoverySourceUsecase) Handle(
	ctx context.Context,
	in run.DiscoverySourceUpsertInput,
) (run.DiscoverySourceUpsertResult, error) {
	in.ProjectID = strings.TrimSpace(in.ProjectID)
	in.RunID = strings.TrimSpace(in.RunID)
	in.TraceID = strings.TrimSpace(in.TraceID)
	in.PipelineVersion = strings.TrimSpace(in.PipelineVersion)
	in.PolicyVersion = strings.TrimSpace(in.PolicyVersion)

	in.SourceType = strings.TrimSpace(in.SourceType)
	in.SourceRefRaw = strings.TrimSpace(in.SourceRefRaw)
	in.SourceRef = strings.TrimSpace(in.SourceRef)
	in.SourceHash = strings.TrimSpace(in.SourceHash)

	// Required
	if in.ProjectID == "" {
		return run.DiscoverySourceUpsertResult{}, errors.New("project_id is required")
	}
	if in.RunID == "" {
		return run.DiscoverySourceUpsertResult{}, errors.New("run_id is required")
	}
	if in.TraceID == "" {
		return run.DiscoverySourceUpsertResult{}, errors.New("trace_id is required")
	}
	if in.SourceType == "" {
		return run.DiscoverySourceUpsertResult{}, errors.New("source_type is required")
	}
	if in.SourceRefRaw == "" {
		return run.DiscoverySourceUpsertResult{}, errors.New("source_ref_raw is required")
	}
	if in.SourceRef == "" {
		return run.DiscoverySourceUpsertResult{}, errors.New("source_ref is required (normalized)")
	}

	// Defaults for versions
	if in.PipelineVersion == "" {
		if uc.DefaultPipelineVersion == "" {
			return run.DiscoverySourceUpsertResult{}, errors.New("pipeline_version is required (no default set)")
		}
		in.PipelineVersion = uc.DefaultPipelineVersion
	}
	if in.PolicyVersion == "" {
		if uc.DefaultPolicyVersion == "" {
			return run.DiscoverySourceUpsertResult{}, errors.New("policy_version is required (no default set)")
		}
		in.PolicyVersion = uc.DefaultPolicyVersion
	}

	// Hash: compute if absent
	if in.SourceHash == "" {
		in.SourceHash = ComputeSourceHashV8(in.ProjectID, in.SourceType, in.SourceRef)
	}
	if len(in.SourceHash) != 64 {
		return run.DiscoverySourceUpsertResult{}, errors.New("source_hash must be 64 hex chars")
	}

	// Optional but safety: normalize status/failure values to known set when provided
	if in.Status != nil {
		switch *in.Status {
		case run.SourceStatusDetected, run.SourceStatusAcquired, run.SourceStatusExtracted:
		default:
			return run.DiscoverySourceUpsertResult{}, errors.New("invalid status (must be detected|acquired|extracted)")
		}
	}
	if in.FailureState != nil {
		switch *in.FailureState {
		case run.SourceFailNone, run.SourceFailNeedsRetry, run.SourceFailFailed:
		default:
			return run.DiscoverySourceUpsertResult{}, errors.New("invalid failure_state (must be none|needs_retry|failed)")
		}
	}

	// Enforce short failure message if supplied (full details must go to evidence_assets)
	if in.FailureMsg != nil {
		msg := strings.TrimSpace(*in.FailureMsg)
		if msg == "" {
			in.FailureMsg = nil
		} else {
			// keep it short (align with DB varchar(256))
			if len(msg) > 256 {
				trunc := msg[:256]
				in.FailureMsg = &trunc
			} else {
				in.FailureMsg = &msg
			}
		}
	}
	if in.FailureCode != nil {
		code := strings.TrimSpace(*in.FailureCode)
		if code == "" {
			in.FailureCode = nil
		} else if len(code) > 64 {
			trunc := code[:64]
			in.FailureCode = &trunc
		} else {
			in.FailureCode = &code
		}
	}

	// Optional: prevent nonsense timestamp regressions by ensuring caller doesn't pass stale refs
	// (DB sets first_seen/last_seen; we only sanity-check time if needed in future.)
	_ = time.Now() // keep import; can be used for logging later.

	return uc.Sources.Upsert(ctx, in)
}
