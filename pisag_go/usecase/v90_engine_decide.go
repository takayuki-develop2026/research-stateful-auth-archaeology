package usecase

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	"example.com/pisag_go/postgres"
	"example.com/pisag_go/run"
)

// Principal (minimal for cache safety)
type EnginePrincipal struct {
	ActorType    string   // system|user|service
	ActorID      string   // optional but recommended
	Roles        []string // role names
	AllowlistKey string   // optional (if you want to bind cache to allowlist key)
}

type EngineDecideInput struct {
	ProjectID string
	RunID     string // uuid text
	TraceID   string // uuid text

	TaskType        string
	Mode            string // mode0..mode4 (string)
	PipelineVersion string
	PolicyVersion   string

	IdempotencyKey string // must include scope (v13 style)

	Principal EnginePrincipal
	InputJSON []byte // arbitrary JSON bytes
}

type EngineDecideOutput struct {
	EngineRunID string

	Status   string // succeeded|review_required|failed_recorded|skipped
	CacheHit bool

	DecisionID   string
	DecisionType string

	DecisionEvidenceAssetID int64

	DecisionResultJSON      []byte
	DecisionRationaleJSON   []byte
	DecisionConstraintsJSON []byte

	CacheKey string
}

// Decision builder interface (plug-in point for v9.1+ integrations, e.g. routing_decide_v5)
type EngineDecisionBuilder interface {
	Build(ctx context.Context, in EngineDecideInput, principalHash, inputHash string) (decisionType string, result any, rationale any, constraints any, status string, err error)
}

// Default builder (P0): deterministic, safe, proposal-only for funds task types.
type DefaultEngineDecisionBuilder struct{}

func NewDefaultEngineDecisionBuilder() *DefaultEngineDecisionBuilder {
	return &DefaultEngineDecisionBuilder{}
}

func (b *DefaultEngineDecisionBuilder) Build(ctx context.Context, in EngineDecideInput, principalHash, inputHash string) (string, any, any, any, string, error) {
	_ = ctx
	// funds_* are proposal-only (contract)
	task := strings.ToLower(strings.TrimSpace(in.TaskType))
	isFunds := strings.HasPrefix(task, "funds_")

	constraints := map[string]any{
		"budget":           map[string]any{"allowed": true, "reason": "ok"},
		"role":             map[string]any{"allowed": true, "reason": "ok"},
		"policy":           map[string]any{"allowed": true, "policy_version": in.PolicyVersion},
		"kill_switch":      map[string]any{"blocked": false},
		"regression_guard": map[string]any{"blocked": false},
	}

	if isFunds {
		// proposal-only
		constraints["action_policy"] = map[string]any{"mode": "proposal_only", "reason": "funds_always_proposal_only"}
		result := map[string]any{
			"target_api":        "v15_funds_api",
			"method":            "POST",
			"confirm_required":  true,
			"idempotency_scope": "funds_action_confirm",
			"risk_level":        "high",
			"input_hash":        inputHash,
		}
		rationale := map[string]any{
			"why": "funds task is proposal-only (v15 contract)",
		}
		return "proposal", result, rationale, constraints, "succeeded", nil
	}

	// non-funds: simple deterministic route/plan decision
	constraints["action_policy"] = map[string]any{"mode": "proposal_only", "reason": "default_deny"} // still default deny
	result := map[string]any{
		"plan": []any{
			map[string]any{"step": "noop", "note": "v9 P0 default plan"},
		},
		"input_hash": inputHash,
	}
	rationale := map[string]any{
		"why": "v9 P0 default plan (replace with routing/llm later)",
	}
	return "plan", result, rationale, constraints, "succeeded", nil
}

type EngineDecideUsecaseV9 struct {
	Repo     *postgres.EngineRepoV9
	Evidence run.EvidenceV18Repo
	Builder  EngineDecisionBuilder

	CacheTTL time.Duration
}

func NewEngineDecideUsecaseV9(repo *postgres.EngineRepoV9, evidence run.EvidenceV18Repo, builder EngineDecisionBuilder) *EngineDecideUsecaseV9 {
	if builder == nil {
		builder = NewDefaultEngineDecisionBuilder()
	}
	return &EngineDecideUsecaseV9{
		Repo:     repo,
		Evidence: evidence,
		Builder:  builder,
		CacheTTL: 10 * time.Minute,
	}
}

func (uc *EngineDecideUsecaseV9) Handle(ctx context.Context, in EngineDecideInput) (EngineDecideOutput, error) {
	// trim
	in.ProjectID = strings.TrimSpace(in.ProjectID)
	in.RunID = strings.TrimSpace(in.RunID)
	in.TraceID = strings.TrimSpace(in.TraceID)
	in.TaskType = strings.TrimSpace(in.TaskType)
	in.Mode = strings.TrimSpace(in.Mode)
	in.PipelineVersion = strings.TrimSpace(in.PipelineVersion)
	in.PolicyVersion = strings.TrimSpace(in.PolicyVersion)
	in.IdempotencyKey = strings.TrimSpace(in.IdempotencyKey)

	in.Principal.ActorType = strings.TrimSpace(in.Principal.ActorType)
	in.Principal.ActorID = strings.TrimSpace(in.Principal.ActorID)
	in.Principal.AllowlistKey = strings.TrimSpace(in.Principal.AllowlistKey)

	if in.ProjectID == "" || in.RunID == "" || in.TraceID == "" {
		return EngineDecideOutput{}, errors.New("project_id/run_id/trace_id are required")
	}
	if in.TaskType == "" || in.Mode == "" || in.PipelineVersion == "" || in.PolicyVersion == "" {
		return EngineDecideOutput{}, errors.New("task_type/mode/pipeline_version/policy_version are required")
	}
	if in.IdempotencyKey == "" {
		return EngineDecideOutput{}, errors.New("idempotency_key is required")
	}
	if uc.Repo == nil || uc.Evidence == nil {
		return EngineDecideOutput{}, errors.New("repo and evidence repo are required")
	}

	principalHash := computePrincipalHash(in.ProjectID, in.Principal)
	inputHash, err := computeInputHash(in.InputJSON)
	if err != nil {
		return EngineDecideOutput{}, err
	}

	cacheKey, err := postgres.ComputeEngineCacheKeyV9(
		in.ProjectID, in.TaskType, in.Mode, in.PipelineVersion, in.PolicyVersion, principalHash, inputHash,
	)
	if err != nil {
		return EngineDecideOutput{}, err
	}

	// Upsert stable engine_run row
	up, err := uc.Repo.UpsertStableEngineRun(ctx, postgres.EngineRunUpsertStableInput{
		ProjectID: in.ProjectID,
		RunID:     in.RunID,
		TraceID:   in.TraceID,

		TaskType:        in.TaskType,
		Mode:            in.Mode,
		PipelineVersion: in.PipelineVersion,
		PolicyVersion:   in.PolicyVersion,

		PrincipalHash: principalHash,
		InputHash:     inputHash,

		IdempotencyKey: in.IdempotencyKey,
	})
	if err != nil {
		return EngineDecideOutput{}, err
	}

	// Check cache
	cached, err := uc.Repo.GetCache(ctx, in.ProjectID, cacheKey)
	if err != nil {
		return EngineDecideOutput{}, err
	}
	if cached != nil {
		dec, err := uc.Repo.GetDecision(ctx, cached.DecisionID)
		if err != nil {
			return EngineDecideOutput{}, err
		}
		_ = uc.Repo.MarkEngineRunRunning(ctx, in.ProjectID, up.EngineRunID, &cacheKey)
		_ = uc.Repo.CompleteEngineRun(ctx, in.ProjectID, up.EngineRunID, "succeeded", &dec.DecisionID, &cacheKey, nil, nil, nil)

		return EngineDecideOutput{
			EngineRunID:             up.EngineRunID,
			Status:                  "succeeded",
			CacheHit:                true,
			DecisionID:              dec.DecisionID,
			DecisionType:            dec.DecisionType,
			DecisionEvidenceAssetID: dec.DecisionEvidenceAssetID,
			DecisionResultJSON:      dec.ResultJSON,
			DecisionRationaleJSON:   dec.RationaleJSON,
			DecisionConstraintsJSON: dec.ConstraintsJSON,
			CacheKey:                cacheKey,
		}, nil
	}

	// Cache miss: mark running
	_ = uc.Repo.MarkEngineRunRunning(ctx, in.ProjectID, up.EngineRunID, &cacheKey)

	// Build decision (no-throw design: errors should be turned into failed_recorded in higher layers; here we return error)
	decisionType, resultObj, rationaleObj, constraintsObj, status, err := uc.Builder.Build(ctx, in, principalHash, inputHash)
	if err != nil {
		// record failed_recorded with evidence
		evID, _ := uc.writeDecisionEvidence(ctx, in.ProjectID, in.TraceID, map[string]any{
			"type": "engine_error",
			"err":  err.Error(),
		}, in.IdempotencyKey+":evidence")
		_ = uc.Repo.CompleteEngineRun(ctx, in.ProjectID, up.EngineRunID, "failed_recorded", nil, &cacheKey, ptrStr("engine_error"), ptrStr(short256(err.Error())), ptrI64(evID))
		return EngineDecideOutput{}, err
	}

	// evidence: decision report (heavy)
	evidenceID, err := uc.writeDecisionEvidence(ctx, in.ProjectID, in.TraceID, map[string]any{
		"task_type":        in.TaskType,
		"mode":             in.Mode,
		"policy_version":   in.PolicyVersion,
		"pipeline_version": in.PipelineVersion,
		"principal_hash":   principalHash,
		"input_hash":       inputHash,
		"cache_key":        cacheKey,
		"decision_type":    decisionType,
		"status":           status,
		"result":           resultObj,
		"rationale":        rationaleObj,
		"constraints":      constraintsObj,
	}, in.IdempotencyKey+":decision_evidence")
	if err != nil || evidenceID <= 0 {
		return EngineDecideOutput{}, errors.New("failed to write decision evidence")
	}

	// insert decision ledger
	resultJSON := mustJSONBytes(resultObj)
	rationaleJSON := mustJSONBytes(rationaleObj)
	constraintsJSON := mustJSONBytes(constraintsObj)

	decIns, err := uc.Repo.InsertDecision(ctx, postgres.DecisionInsertInputV9{
		ProjectID:    in.ProjectID,
		EngineRunID:  up.EngineRunID,
		DecisionType: decisionType,

		ResultJSON:      resultJSON,
		RationaleJSON:   rationaleJSON,
		ConstraintsJSON: constraintsJSON,

		DecisionEvidenceAssetID: evidenceID,

		CreatedByType: safeCreatedBy(in.Principal.ActorType),
		CreatedByID:   optStr(in.Principal.ActorID),
		PolicyVersion: in.PolicyVersion,
	})
	if err != nil {
		return EngineDecideOutput{}, err
	}

	// complete engine run
	_ = uc.Repo.CompleteEngineRun(ctx, in.ProjectID, up.EngineRunID, statusFromEngine(status), &decIns.DecisionID, &cacheKey, nil, nil, nil)

	// cache store only if succeeded
	if statusFromEngine(status) == "succeeded" {
		_ = uc.Repo.UpsertCache(ctx, in.ProjectID, cacheKey, up.EngineRunID, decIns.DecisionID, time.Now().Add(uc.CacheTTL))
	}

	return EngineDecideOutput{
		EngineRunID:             up.EngineRunID,
		Status:                  statusFromEngine(status),
		CacheHit:                false,
		DecisionID:              decIns.DecisionID,
		DecisionType:            decisionType,
		DecisionEvidenceAssetID: evidenceID,
		DecisionResultJSON:      resultJSON,
		DecisionRationaleJSON:   rationaleJSON,
		DecisionConstraintsJSON: constraintsJSON,
		CacheKey:                cacheKey,
	}, nil
}

// ----------------------------- helpers -----------------------------

func statusFromEngine(status string) string {
	s := strings.ToLower(strings.TrimSpace(status))
	switch s {
	case "succeeded":
		return "succeeded"
	case "review_required":
		return "review_required"
	case "failed_recorded":
		return "failed_recorded"
	case "skipped":
		return "skipped"
	default:
		// default to succeeded if builder returns unknown (safe for P0)
		return "succeeded"
	}
}

func safeCreatedBy(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	if v == "" {
		return "system"
	}
	if v != "system" && v != "user" && v != "service" {
		return "system"
	}
	return v
}

func computePrincipalHash(projectID string, p EnginePrincipal) string {
	roles := append([]string{}, p.Roles...)
	for i := range roles {
		roles[i] = strings.TrimSpace(roles[i])
	}
	sort.Strings(roles)
	s := strings.Join([]string{
		strings.TrimSpace(projectID),
		strings.TrimSpace(p.ActorType),
		strings.TrimSpace(p.ActorID),
		strings.Join(roles, ","),
		strings.TrimSpace(p.AllowlistKey),
	}, "|")
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func computeInputHash(raw []byte) (string, error) {
	if len(raw) == 0 {
		raw = []byte(`{}`)
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return "", err
	}
	normalized, err := json.Marshal(v) // encoding/json sorts keys deterministically
	if err != nil {
		return "", err
	}
	h := sha256.Sum256(normalized)
	return hex.EncodeToString(h[:]), nil
}

func mustJSONBytes(v any) []byte {
	if v == nil {
		return []byte(`{}`)
	}
	b, err := json.Marshal(v)
	if err != nil {
		return []byte(`{}`)
	}
	return b
}

func (uc *EngineDecideUsecaseV9) writeDecisionEvidence(ctx context.Context, projectID, traceID string, payload any, idem string) (int64, error) {
	b := mustJSONBytes(payload)
	sum := sha256.Sum256(b)
	sha := hex.EncodeToString(sum[:])

	res, err := uc.Evidence.Register(ctx, run.EvidenceRegisterInput{
		ProjectID: projectID,
		TraceID:   traceID,
		ActorType: "system",
		ActorID:   nil,

		MediaType:     "text",
		MimeType:      "application/json",
		SourceKind:    "generated",
		SourceURI:     optStr("engine://decision"),
		ContentSHA256: sha,
		ContentLength: int64(len(b)),

		Language:        nil,
		RetentionPolicy: "standard",
		ExpiresAtUTC:    nil,

		IdempotencyKey: idem,
	})
	if err != nil {
		return 0, err
	}
	return uc.Repo.ResolveEvidenceAssetIDByRef(ctx, projectID, res.EvidenceRef)
}

func short256(s string) string {
	s = strings.TrimSpace(s)
	if len(s) <= 256 {
		return s
	}
	return s[:256]
}

func optStr(s string) *string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return &s
}

func ptrStr(s string) *string { return &s }

func ptrI64(v int64) *int64 {
	if v <= 0 {
		return nil
	}
	return &v
}
