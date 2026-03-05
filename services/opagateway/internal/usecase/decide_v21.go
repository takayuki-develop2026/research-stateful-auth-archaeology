package usecase

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"crypto/sha256"
	"encoding/hex"

	"services/opagateway/internal/opa"
	"services/opagateway/internal/postgres"
)

type DecideInput struct {
	ProjectID        string
	TraceID          string
	RunID            *string // uuid text or nil
	SubjectType      string  // user|service|api_key
	SubjectID        string
	ActionKey        string
	ActionClass      opa.ActionClass
	ResourceKey      string // stable key (e.g., "schedule:1" or "evidence:123")
	PolicyVersionStr string
	PolicyPath       string // OPA package/rule path
	// canonical input for hashing + evidence
	Input any
}

type DecideOutput struct {
	DecisionID int64
	Decision   opa.Decision
	DecisionKey string
	InputHash   string
}

type Service struct {
	DB  *postgres.DB
	OPA opa.Client
}

func NewService(db *postgres.DB, opaClient opa.Client) *Service {
	return &Service{DB: db, OPA: opaClient}
}

func (s *Service) DecideAndRecord(ctx context.Context, in DecideInput) (DecideOutput, error) {
	in.ProjectID = strings.TrimSpace(in.ProjectID)
	in.TraceID = strings.TrimSpace(in.TraceID)
	in.SubjectType = strings.TrimSpace(in.SubjectType)
	in.SubjectID = strings.TrimSpace(in.SubjectID)
	in.ActionKey = strings.TrimSpace(in.ActionKey)
	in.ResourceKey = strings.TrimSpace(in.ResourceKey)
	in.PolicyVersionStr = strings.TrimSpace(in.PolicyVersionStr)
	in.PolicyPath = strings.TrimSpace(in.PolicyPath)

	// canonical input evidence
	raw, _ := json.Marshal(in.Input)
	canonical := string(raw)
	inputHash := sha256Hex(canonical)

	// Evidence: decision input
	inEvID, err := postgres.RegisterTextEvidenceAssetV18(
		ctx, s.DB, in.ProjectID, in.TraceID, "service", "opagateway",
		"generated",
		"opagateway://decision_input/"+in.ActionKey,
		canonical,
		"v21:decision_input:"+in.ProjectID+":"+in.TraceID+":"+inputHash,
	)
	if err != nil {
		return DecideOutput{}, fmt.Errorf("evidence input register failed: %w", err)
	}

	// call PDP (with fail-closed behaviors inside client)
	dec, cacheKey, pdpErr := s.OPA.Decide(ctx, in.Input, in.PolicyPath, in.ActionClass)

	// Evidence: decision result / obligations / reason codes / resource
	resObj := map[string]any{
		"result":       dec.Result,
		"reason_codes": dec.ReasonCodes,
		"obligations":  dec.Obligations,
		"score":        dec.Score,
		"cache_key":    cacheKey,
		"pdp_error":    "",
	}
	if pdpErr != nil {
		resObj["pdp_error"] = pdpErr.Error()
	}
	resJSON, _ := json.Marshal(resObj)

	resEvID, err := postgres.RegisterTextEvidenceAssetV18(
		ctx, s.DB, in.ProjectID, in.TraceID, "service", "opagateway",
		"generated",
		"opagateway://decision_result/"+in.ActionKey,
		string(resJSON),
		"v21:decision_result:"+in.ProjectID+":"+in.TraceID+":"+inputHash,
	)
	if err != nil {
		return DecideOutput{}, fmt.Errorf("evidence result register failed: %w", err)
	}

	// Resource evidence (minimal)
	resourceObj := map[string]any{
		"resource_key": in.ResourceKey,
	}
	resourceJSON, _ := json.Marshal(resourceObj)

	resourceEvID, err := postgres.RegisterTextEvidenceAssetV18(
		ctx, s.DB, in.ProjectID, in.TraceID, "service", "opagateway",
		"generated",
		"opagateway://resource/"+in.ActionKey,
		string(resourceJSON),
		"v21:resource:"+in.ProjectID+":"+in.TraceID+":"+in.ResourceKey,
	)
	if err != nil {
		return DecideOutput{}, fmt.Errorf("evidence resource register failed: %w", err)
	}

	// Obligations evidence
	obJSON, _ := json.Marshal(dec.Obligations)
	obEvID, err := postgres.RegisterTextEvidenceAssetV18(
		ctx, s.DB, in.ProjectID, in.TraceID, "service", "opagateway",
		"generated",
		"opagateway://obligations/"+in.ActionKey,
		string(obJSON),
		"v21:obligations:"+in.ProjectID+":"+in.TraceID+":"+in.ActionKey,
	)
	if err != nil {
		return DecideOutput{}, fmt.Errorf("evidence obligations register failed: %w", err)
	}

	// Reason codes evidence (array)
	rcJSON, _ := json.Marshal(dec.ReasonCodes)
	rcEvID, err := postgres.RegisterTextEvidenceAssetV18(
		ctx, s.DB, in.ProjectID, in.TraceID, "service", "opagateway",
		"generated",
		"opagateway://reason_codes/"+in.ActionKey,
		string(rcJSON),
		"v21:reason_codes:"+in.ProjectID+":"+in.TraceID+":"+in.ActionKey,
	)
	if err != nil {
		return DecideOutput{}, fmt.Errorf("evidence reason_codes register failed: %w", err)
	}

	decisionKey := postgresDecisionKey(in.ProjectID, in.TraceID, in.ActionKey, in.ResourceKey, in.PolicyVersionStr)

	appendIn := postgres.PolicyDecisionAppendIn{
		ProjectID:        in.ProjectID,
		DecisionKey:      decisionKey,
		TraceID:          in.TraceID,
		RunID:            in.RunID,
		SubjectType:      in.SubjectType,
		SubjectID:        in.SubjectID,
		ActionKey:        in.ActionKey,
		ActionClass:      string(in.ActionClass),
		PolicyVersionStr: in.PolicyVersionStr,
		Result:           mapResult(dec.Result, pdpErr),
		InputHashSha256:  inputHash,

		DecisionInputEvidenceAssetID:  inEvID,
		DecisionResultEvidenceAssetID: resEvID,
		ResourceEvidenceAssetID:       resourceEvID,
		ObligationsEvidenceAssetID:     obEvID,
		ReasonCodesEvidenceAssetID:     rcEvID,
	}

	decisionID, _, err := postgres.AppendPolicyDecisionV21(ctx, s.DB, appendIn)
	if err != nil {
		return DecideOutput{}, fmt.Errorf("policy_decision_append_v21 failed: %w", err)
	}

	return DecideOutput{
		DecisionID: decisionID,
		Decision:   dec,
		DecisionKey: decisionKey,
		InputHash:  inputHash,
	}, nil
}

func mapResult(r opa.DecisionResult, pdpErr error) string {
	// If pdp error happened, we still want to represent fallback decisions as allow/deny,
	// but you can choose to store 'error' when action_class=high_risk and pdpErr != nil.
	// Here we store allow/deny for fallbacks, and 'error' only for unrecognized.
	if r == opa.ResultAllow {
		return "allow"
	}
	if r == opa.ResultDeny {
		return "deny"
	}
	if r == opa.ResultError {
		return "error"
	}
	if pdpErr != nil {
		return "error"
	}
	return "error"
}

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func postgresDecisionKey(projectID, traceID, actionKey, resourceKey, policyVersionStr string) string {
	sum := sha256.Sum256([]byte(projectID + "|" + traceID + "|" + actionKey + "|" + resourceKey + "|" + policyVersionStr))
	return hex.EncodeToString(sum[:])
}