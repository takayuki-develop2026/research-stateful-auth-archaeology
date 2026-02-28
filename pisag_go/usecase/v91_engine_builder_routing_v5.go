package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"example.com/pisag_go/run"
)

// v9.1: EngineDecisionBuilder that delegates to v5 routing (preview->commit->v6 UTL ingest).
//
// - Input: EngineDecideInput.InputJSON must contain routing fields.
// - Output: decision_type "route" (chosen) or "review_required" (otherwise).
// - Funds tasks are NOT handled here (v15 proposal-only is separate).
type RoutingV5EngineDecisionBuilder struct {
	CommitToUtl *RoutingCommitToUtlV6Usecase // v5 commit + v6 UTL ingest (already implemented)
}

func NewRoutingV5EngineDecisionBuilder(commitToUtl *RoutingCommitToUtlV6Usecase) *RoutingV5EngineDecisionBuilder {
	return &RoutingV5EngineDecisionBuilder{CommitToUtl: commitToUtl}
}

// Routing input payload for engine.
// Keep it minimal and deterministic; heavy details are in evidence assets already.
type engineRoutingInputV91 struct {
	SubjectType       string          `json:"subject_type"`
	SubjectInternalID string          `json:"subject_internal_id"`
	Region            string          `json:"region"`
	Currency          string          `json:"currency"`
	PaymentMethod     string          `json:"payment_method"`
	AmountMinor       int64           `json:"amount_minor"`
	Constraints       json.RawMessage `json:"constraints,omitempty"` // optional object
	AcceptSuggested   *bool           `json:"accept_suggested,omitempty"`
	OverrideRouteID   *string         `json:"override_route_id,omitempty"`
}

func (b *RoutingV5EngineDecisionBuilder) Build(
	ctx context.Context,
	in EngineDecideInput,
	principalHash string,
	inputHash string,
) (decisionType string, result any, rationale any, constraints any, status string, err error) {
	_ = principalHash
	_ = inputHash

	if b == nil || b.CommitToUtl == nil {
		return "review_required", nil, nil, nil, "failed_recorded", errors.New("routing builder requires CommitToUtl usecase")
	}

	// Parse routing input
	var ri engineRoutingInputV91
	if len(in.InputJSON) == 0 {
		return "review_required", nil, nil, nil, "failed_recorded", errors.New("input json is required for routing_decide_v5")
	}
	if err := json.Unmarshal(in.InputJSON, &ri); err != nil {
		return "review_required", nil, nil, nil, "failed_recorded", err
	}

	ri.SubjectType = strings.TrimSpace(ri.SubjectType)
	ri.SubjectInternalID = strings.TrimSpace(ri.SubjectInternalID)
	ri.Region = strings.TrimSpace(ri.Region)
	ri.Currency = strings.TrimSpace(ri.Currency)
	ri.PaymentMethod = strings.TrimSpace(ri.PaymentMethod)

	if ri.SubjectType == "" || ri.SubjectInternalID == "" {
		return "review_required", nil, nil, nil, "failed_recorded", errors.New("subject_type and subject_internal_id are required")
	}
	if ri.Region == "" || ri.Currency == "" || ri.PaymentMethod == "" {
		return "review_required", nil, nil, nil, "failed_recorded", errors.New("region/currency/payment_method are required")
	}

	accept := true
	if ri.AcceptSuggested != nil {
		accept = *ri.AcceptSuggested
	}

	// constraints json bytes (must be object or empty)
	constraintsBytes := []byte(`{}`)
	if len(ri.Constraints) > 0 {
		trim := strings.TrimSpace(string(ri.Constraints))
		if trim != "" && trim != "null" {
			constraintsBytes = []byte(trim)
		}
	}

	// Build RoutingCommitInput (v5)
	commitIn := run.RoutingCommitInput{
		RoutingInput: run.RoutingInput{
			ProjectID: in.ProjectID,

			SubjectType:       ri.SubjectType,
			SubjectInternalID: ri.SubjectInternalID,

			Region:        ri.Region,
			Currency:      ri.Currency,
			PaymentMethod: ri.PaymentMethod,
			AmountMinor:   ri.AmountMinor,

			ConstraintsJSON: constraintsBytes,

			// Use engine versions as policy/pipeline references (OK for v5; deterministic).
			PolicyVersion:   in.PolicyVersion,
			PipelineVersion: "v5",
			RoutingVersion:  "v5",

			TraceID: in.TraceID,
			RunID:   in.RunID,
		},

		// Preview fingerprint will be computed inside commit flow (we pass empty; commit usecase already re-previews).
		ExpectedInputFingerprint: "",
		AcceptSuggested:          accept,
		OverrideRouteID:          ri.OverrideRouteID,
	}

	commitOut, utlRes, err := b.CommitToUtl.Handle(ctx, commitIn)
	if err != nil {
		// caller (engine decide) will convert this to failed_recorded evidence.
		return "review_required", nil, nil, nil, "failed_recorded", err
	}

	// Gate result (P0): default deny stays, but routing is internal-safe.
	g := map[string]any{
		"budget":           map[string]any{"allowed": true, "reason": "ok"},
		"role":             map[string]any{"allowed": true, "reason": "ok"},
		"policy":           map[string]any{"allowed": true, "policy_version": in.PolicyVersion},
		"kill_switch":      map[string]any{"blocked": false},
		"regression_guard": map[string]any{"blocked": false},
		"action_policy":    map[string]any{"mode": "proposal_only", "reason": "default_deny"},
		"routing": map[string]any{
			"status": commitOut.Status,
		},
	}

	// Decision mapping
	if commitOut.Status == "chosen" {
		decisionType = "route"
		status = "succeeded"
	} else {
		decisionType = "review_required"
		status = "review_required"
	}

	// Result is a lightweight summary; heavy is in evidence assets (why_ref / engine decision evidence)
	result = map[string]any{
		"routing_version": "v5",
		"route_decision": map[string]any{
			"decision_id": commitOut.DecisionID,
			"status":      commitOut.Status,
			"chosen_route_id":   commitOut.ChosenRouteID,
			"chosen_provider_id": commitOut.ChosenProviderID,
			"denied_reason": commitOut.DeniedReason,
			"why_evidence_ref": commitOut.WhyEvidenceRef,
			"v5_utl_commit_event_key": commitOut.UtlCommitEventKey,
		},
		"utl": map[string]any{
			"utl_event_id": utlRes.UtlEventID,
			"status":       utlRes.Status,
			"event_key":    utlRes.EventKey,
			"posting_key":  utlRes.PostingKey,
		},
	}

	rationale = map[string]any{
		"why": "engine task routing_decide_v5 delegated to v5 routing + v6 utl_ingest_v6",
	}

	constraints = g
	return decisionType, result, rationale, constraints, status, nil
}