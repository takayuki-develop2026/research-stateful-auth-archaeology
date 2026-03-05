package opa

import "time"

type ActionClass string

const (
	HighRisk    ActionClass = "high_risk"
	LowRiskRead ActionClass = "low_risk_read"
	LowRiskWrite ActionClass = "low_risk_write"
)

type DecisionResult string

const (
	ResultAllow          DecisionResult = "allow"
	ResultDeny           DecisionResult = "deny"
	ResultError          DecisionResult = "error" // PDP unreachable/invalid response etc
	ResultReviewRequired DecisionResult = "review_required"
	ResultProposalOnly   DecisionResult = "proposal_only"
)

// Minimal obligations vocabulary (v21 fixed)
type Obligations struct {
	RequireApproval bool   `json:"require_approval"`
	RequireEvidence bool   `json:"require_evidence"`
	MaskRuleKey     string `json:"mask_rule_key"`
}

type Decision struct {
	Result      DecisionResult `json:"result"`
	ReasonCodes []string       `json:"reason_codes"`
	Obligations Obligations    `json:"obligations"`
	// Optional: score/decision_id etc (kept out of SoT; put into evidence if needed)
	Score float64 `json:"score,omitempty"`
}

type ClientConfig struct {
	BaseURL       string
	Timeout       time.Duration
	RetryCount    int
	CacheTTL      time.Duration
	CacheMaxItems int
}