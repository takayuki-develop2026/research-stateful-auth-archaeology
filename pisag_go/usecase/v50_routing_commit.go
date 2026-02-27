package usecase

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"
	"strings"

	"example.com/pisag_go/run"
)

type RoutingCommitUsecaseV5 struct {
	Preview   *RoutingPreviewUsecaseV5
	Decisions run.RouteDecisionsRepoV5
	// v6 UTL ingest は次段（ここでは event_key 生成→台帳保存まで）
}

func NewRoutingCommitUsecaseV5(preview *RoutingPreviewUsecaseV5, decisions run.RouteDecisionsRepoV5) *RoutingCommitUsecaseV5 {
	return &RoutingCommitUsecaseV5{
		Preview:   preview,
		Decisions: decisions,
	}
}

func (uc *RoutingCommitUsecaseV5) Handle(ctx context.Context, in run.RoutingCommitInput) (run.RoutingCommitResult, error) {
	if uc.Preview == nil {
		return run.RoutingCommitResult{}, errors.New("preview usecase is required")
	}

	// trim
	in.ProjectID = strings.TrimSpace(in.ProjectID)
	in.SubjectType = strings.TrimSpace(in.SubjectType)
	in.SubjectInternalID = strings.TrimSpace(in.SubjectInternalID)
	in.PolicyVersion = strings.TrimSpace(in.PolicyVersion)
	in.PipelineVersion = strings.TrimSpace(in.PipelineVersion)
	in.RoutingVersion = strings.TrimSpace(in.RoutingVersion)
	in.TraceID = strings.TrimSpace(in.TraceID)
	in.RunID = strings.TrimSpace(in.RunID)
	in.ExpectedInputFingerprint = strings.TrimSpace(in.ExpectedInputFingerprint)

	if in.RoutingVersion == "" {
		in.RoutingVersion = "v5"
	}

	if in.RunID == "" {
		return run.RoutingCommitResult{}, errors.New("run_id is required for commit")
	}
	if in.ProjectID == "" || in.SubjectType == "" || in.SubjectInternalID == "" {
		return run.RoutingCommitResult{}, errors.New("project_id/subject_type/subject_internal_id are required")
	}
	if in.PolicyVersion == "" || in.PipelineVersion == "" || in.TraceID == "" {
		return run.RoutingCommitResult{}, errors.New("policy_version/pipeline_version/trace_id are required")
	}

	prev, err := uc.Preview.Handle(ctx, run.RoutingPreviewInput{RoutingInput: in.RoutingInput})
	if err != nil {
		return run.RoutingCommitResult{}, err
	}

	if in.ExpectedInputFingerprint != "" && in.ExpectedInputFingerprint != prev.InputFingerprint {
		return uc.v50CommitAsReviewRequired(ctx, in, prev, "fingerprint_mismatch")
	}

	if prev.Status == "denied" {
		dr := "no_candidates"
		return uc.v50CommitDenied(ctx, in, prev, &dr)
	}

	var chosenRouteID *string
	var chosenProviderID *string

	if in.OverrideRouteID != nil && strings.TrimSpace(*in.OverrideRouteID) != "" {
		rid := strings.TrimSpace(*in.OverrideRouteID)
		chosenRouteID = &rid
		for _, c := range prev.Candidates {
			if !c.Excluded && c.RouteID == rid {
				pid := c.ProviderID
				chosenProviderID = &pid
				break
			}
		}
	} else if in.AcceptSuggested && prev.SuggestedRouteID != nil {
		chosenRouteID = prev.SuggestedRouteID
		chosenProviderID = prev.SuggestedProviderID
	} else {
		return uc.v50CommitAsReviewRequired(ctx, in, prev, "no_confirm_instruction")
	}

	if chosenRouteID == nil || chosenProviderID == nil {
		return uc.v50CommitAsReviewRequired(ctx, in, prev, "chosen_not_resolved")
	}

	utlKey := v50MakeUtlInternalEventKey(in.ProjectID, in.RunID, v50InferProviderKey(prev, *chosenRouteID), "routing.committed", 10)

	why := map[string]any{
		"input_fingerprint": prev.InputFingerprint,
		"chosen": map[string]any{
			"route_id":     chosenRouteID,
			"provider_id":  chosenProviderID,
			"provider_key": v50InferProviderKey(prev, *chosenRouteID),
		},
		"versions": map[string]any{
			"policy_version":   in.PolicyVersion,
			"pipeline_version": in.PipelineVersion,
			"routing_version":  in.RoutingVersion,
		},
		"identity": map[string]any{
			"subject_type":        in.SubjectType,
			"subject_internal_id": in.SubjectInternalID,
		},
		"fallback_used": false,
	}
	whyBytes, _ := json.Marshal(why)

	ins, err := uc.Decisions.InsertIfAbsent(ctx, run.RouteDecisionInsertInput{
		ProjectID:         in.ProjectID,
		SubjectType:       in.SubjectType,
		SubjectInternalID: in.SubjectInternalID,

		PolicyVersion:   in.PolicyVersion,
		PipelineVersion: in.PipelineVersion,
		RoutingVersion:  in.RoutingVersion,

		InputFingerprint: prev.InputFingerprint,

		ChosenRouteID:    chosenRouteID,
		ChosenProviderID: chosenProviderID,

		FallbackUsed: false,

		Status:       "chosen",
		DeniedReason: nil,

		WhyJSON:        whyBytes,
		WhyEvidenceRef: prev.WhyEvidenceRef,

		UtlCommitEventKey: utlKey,

		TraceID: in.TraceID,
		RunID:   in.RunID,
	})
	if err != nil {
		return run.RoutingCommitResult{}, err
	}

	return run.RoutingCommitResult{
		DecisionID:        ins.DecisionID,
		Status:            "chosen",
		ChosenRouteID:     chosenRouteID,
		ChosenProviderID:  chosenProviderID,
		DeniedReason:      nil,
		WhyEvidenceRef:    prev.WhyEvidenceRef,
		UtlCommitEventKey: utlKey,
		InputFingerprint:  prev.InputFingerprint,
		TraceID:           in.TraceID,
		RunID:             in.RunID,
	}, nil
}

func (uc *RoutingCommitUsecaseV5) v50CommitDenied(ctx context.Context, in run.RoutingCommitInput, prev run.RoutingPreviewResult, denied *string) (run.RoutingCommitResult, error) {
	utlKey := v50MakeUtlInternalEventKey(in.ProjectID, in.RunID, "internal", "routing.denied", 30)

	why := map[string]any{
		"input_fingerprint": prev.InputFingerprint,
		"status":            "denied",
		"denied_reason":     denied,
		"versions": map[string]any{
			"policy_version":   in.PolicyVersion,
			"pipeline_version": in.PipelineVersion,
			"routing_version":  in.RoutingVersion,
		},
	}
	whyBytes, _ := json.Marshal(why)

	ins, err := uc.Decisions.InsertIfAbsent(ctx, run.RouteDecisionInsertInput{
		ProjectID:         in.ProjectID,
		SubjectType:       in.SubjectType,
		SubjectInternalID: in.SubjectInternalID,
		PolicyVersion:     in.PolicyVersion,
		PipelineVersion:   in.PipelineVersion,
		RoutingVersion:    in.RoutingVersion,
		InputFingerprint:  prev.InputFingerprint,
		ChosenRouteID:     nil,
		ChosenProviderID:  nil,
		FallbackUsed:      false,
		Status:            "denied",
		DeniedReason:      denied,
		WhyJSON:           whyBytes,
		WhyEvidenceRef:    prev.WhyEvidenceRef,
		UtlCommitEventKey: utlKey,
		TraceID:           in.TraceID,
		RunID:             in.RunID,
	})
	if err != nil {
		return run.RoutingCommitResult{}, err
	}

	return run.RoutingCommitResult{
		DecisionID:        ins.DecisionID,
		Status:            "denied",
		ChosenRouteID:     nil,
		ChosenProviderID:  nil,
		DeniedReason:      denied,
		WhyEvidenceRef:    prev.WhyEvidenceRef,
		UtlCommitEventKey: utlKey,
		InputFingerprint:  prev.InputFingerprint,
		TraceID:           in.TraceID,
		RunID:             in.RunID,
	}, nil
}

func (uc *RoutingCommitUsecaseV5) v50CommitAsReviewRequired(ctx context.Context, in run.RoutingCommitInput, prev run.RoutingPreviewResult, reason string) (run.RoutingCommitResult, error) {
	utlKey := v50MakeUtlInternalEventKey(in.ProjectID, in.RunID, "internal", "routing.review_required", 20)
	rr := reason

	why := map[string]any{
		"input_fingerprint": prev.InputFingerprint,
		"status":            "review_required",
		"reason_code":       rr,
		"versions": map[string]any{
			"policy_version":   in.PolicyVersion,
			"pipeline_version": in.PipelineVersion,
			"routing_version":  in.RoutingVersion,
		},
	}
	whyBytes, _ := json.Marshal(why)

	ins, err := uc.Decisions.InsertIfAbsent(ctx, run.RouteDecisionInsertInput{
		ProjectID:         in.ProjectID,
		SubjectType:       in.SubjectType,
		SubjectInternalID: in.SubjectInternalID,
		PolicyVersion:     in.PolicyVersion,
		PipelineVersion:   in.PipelineVersion,
		RoutingVersion:    in.RoutingVersion,
		InputFingerprint:  prev.InputFingerprint,
		ChosenRouteID:     nil,
		ChosenProviderID:  nil,
		FallbackUsed:      false,
		Status:            "review_required",
		DeniedReason:      nil,
		WhyJSON:           whyBytes,
		WhyEvidenceRef:    prev.WhyEvidenceRef,
		UtlCommitEventKey: utlKey,
		TraceID:           in.TraceID,
		RunID:             in.RunID,
	})
	if err != nil {
		return run.RoutingCommitResult{}, err
	}

	return run.RoutingCommitResult{
		DecisionID:        ins.DecisionID,
		Status:            "review_required",
		ChosenRouteID:     nil,
		ChosenProviderID:  nil,
		DeniedReason:      nil,
		WhyEvidenceRef:    prev.WhyEvidenceRef,
		UtlCommitEventKey: utlKey,
		InputFingerprint:  prev.InputFingerprint,
		TraceID:           in.TraceID,
		RunID:             in.RunID,
	}, nil
}

// ---- v50 helpers (namespaced) ----

func v50InferProviderKey(prev run.RoutingPreviewResult, routeID string) string {
	for _, c := range prev.Candidates {
		if c.RouteID == routeID {
			if strings.TrimSpace(c.ProviderKey) != "" {
				return c.ProviderKey
			}
		}
	}
	return "internal"
}

// v6 contract:
// "utl_internal:" + sha256("internal|project_id|correlation_id|provider|event_name|event_seq")
func v50MakeUtlInternalEventKey(projectID, correlationID, providerKey, eventName string, eventSeq int) string {
	s := "internal|" + strings.TrimSpace(projectID) + "|" + strings.TrimSpace(correlationID) + "|" +
		strings.TrimSpace(providerKey) + "|" + strings.TrimSpace(eventName) + "|" + strconv.Itoa(eventSeq)

	h := sha256.Sum256([]byte(s))
	return "utl_internal:" + hex.EncodeToString(h[:])
}
