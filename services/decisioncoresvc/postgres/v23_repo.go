package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

type V23Repo struct {
	db *sql.DB
}

func NewV23Repo(db *sql.DB) *V23Repo { return &V23Repo{db: db} }

type PolicyEvalUpsertInput struct {
	ProjectID       string
	TraceID         string
	RunID           string // uuid text
	PolicyVersion   string
	PipelineVersion string
	InputHash       string
	PdpMode         string
	Result          string
	ScoreNullable   *float64
	ReasonAssetID   int64
	ObligAssetID    int64
	ProposalAssetID int64 // 0 means NULL
	PolicyDecisionID int64 // 0 means NULL
}

func (r *V23Repo) PolicyEvaluationUpsert(ctx context.Context, in PolicyEvalUpsertInput) (int64, error) {
	var score any = nil
	if in.ScoreNullable != nil {
		score = *in.ScoreNullable
	}
	var id int64
	err := r.db.QueryRowContext(ctx, `
SELECT policy_evaluation_upsert_v23(
  $1,$2,$3::uuid,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13
)`,
		in.ProjectID,
		in.TraceID,
		in.RunID,
		in.PolicyVersion,
		in.PipelineVersion,
		in.InputHash,
		in.PdpMode,
		in.Result,
		score,
		in.ReasonAssetID,
		in.ObligAssetID,
		zeroToNull(in.ProposalAssetID),
		zeroToNull(in.PolicyDecisionID),
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("policy_evaluation_upsert_v23: %w", err)
	}
	return id, nil
}

type DecisionProposeInput struct {
	ProjectID              string
	TraceID                string
	RunID                  string
	SubjectType            string
	SubjectID              string
	SubjectOwnerProjectID  string
	DecisionKey            string
	DecisionScope          string
	PolicyVersion          string
	PipelineVersion        string
	InputHash              string
	InputsAssetID          int64
	ProposalAssetID        int64 // 0 => NULL
	ObligationsAssetID     int64
	PolicyEvaluationID     int64 // 0 => NULL
	DecidedByType          string
	DecidedByID            string
	InitialStatus          string
	ExpiresAtNullable      *string // RFC3339 or empty; keep nil for NULL
}

func (r *V23Repo) DecisionPropose(ctx context.Context, in DecisionProposeInput) (int64, error) {
	var expires any = nil
	if in.ExpiresAtNullable != nil && *in.ExpiresAtNullable != "" {
		expires = *in.ExpiresAtNullable
	}
	var id int64
	err := r.db.QueryRowContext(ctx, `
SELECT decision_propose_v23(
  $1,$2,$3::uuid,
  $4,$5,$6,
  $7,$8,
  $9,$10,$11,
  $12,$13,$14,
  $15,
  $16,$17,
  $18,$19
)`,
		in.ProjectID, in.TraceID, in.RunID,
		in.SubjectType, in.SubjectID, in.SubjectOwnerProjectID,
		in.DecisionKey, in.DecisionScope,
		in.PolicyVersion, in.PipelineVersion, in.InputHash,
		in.InputsAssetID, zeroToNull(in.ProposalAssetID), in.ObligationsAssetID,
		zeroToNull(in.PolicyEvaluationID),
		in.DecidedByType, in.DecidedByID,
		in.InitialStatus,
		expires,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("decision_propose_v23: %w", err)
	}
	return id, nil
}

func (r *V23Repo) DecisionApprove(ctx context.Context, decisionID int64, projectID, decidedByType, decidedByID string, commentAssetID int64) error {
	// We don't rely on return row; we verify state externally if needed.
	_, err := r.db.ExecContext(ctx, `
SELECT decision_approve_v23($1,$2,$3,$4,$5)
`, decisionID, projectID, decidedByType, decidedByID, zeroToNull(commentAssetID))
	if err != nil {
		return fmt.Errorf("decision_approve_v23: %w", err)
	}
	return nil
}

type ActionEnqueueInput struct {
	ProjectID           string
	TraceID             string
	RunID               string
	DecisionLedgerID    int64
	ActionKey           string
	ActionType          string
	ActionScope         string
	TargetHash          string
	TargetAssetID       int64
	PlanAssetID         int64
	BudgetCurrency      string
	BudgetEstimate      int64
	InitialStatus       string
	ErrorAssetID        int64
}

func (r *V23Repo) ActionEnqueue(ctx context.Context, in ActionEnqueueInput) (int64, error) {
	var id int64
	err := r.db.QueryRowContext(ctx, `
SELECT decision_action_enqueue_v23(
  $1,$2,$3::uuid,$4,
  $5,$6,$7,
  $8,$9,$10,
  $11,$12,
  $13,$14
)`,
		in.ProjectID, in.TraceID, in.RunID, in.DecisionLedgerID,
		in.ActionKey, in.ActionType, in.ActionScope,
		in.TargetHash, in.TargetAssetID, in.PlanAssetID,
		in.BudgetCurrency, in.BudgetEstimate,
		in.InitialStatus, zeroToNull(in.ErrorAssetID),
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("decision_action_enqueue_v23: %w", err)
	}
	return id, nil
}

func zeroToNull(v int64) any {
	if v == 0 {
		return nil
	}
	return v
}

// DecisionStatusGet returns status/kind. If no row, status="".
func (r *V23Repo) DecisionStatusGet(ctx context.Context, projectID string, decisionID int64) (status string, kind string, err error) {
	var s sql.NullString
	var k sql.NullString
	err = r.db.QueryRowContext(ctx, `
SELECT status, decision_kind
FROM public.decision_status_get_v23($1,$2)
`, projectID, decisionID).Scan(&s, &k)

	if err != nil {
		// If function returns 0 rows, QueryRow gives no rows.
		if strings.Contains(err.Error(), "no rows") {
			return "", "", nil
		}
		return "", "", fmt.Errorf("decision_status_get_v23: %w", err)
	}
	return s.String, k.String, nil
}