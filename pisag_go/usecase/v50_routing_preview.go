package usecase

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"strconv"
	"strings"

	"example.com/pisag_go/run"
)

type RoutingPreviewUsecaseV5 struct {
	Providers run.ProvidersRepoV5
	Routes    run.ProviderRoutesRepoV5
	Metrics   run.RoutingMetricsRepoV5
	Evidence  run.EvidenceV18Repo // v18 registry -> returns EvidenceRef(uuid)
}

func NewRoutingPreviewUsecaseV5(
	providers run.ProvidersRepoV5,
	routes run.ProviderRoutesRepoV5,
	metrics run.RoutingMetricsRepoV5,
	evidence run.EvidenceV18Repo,
) *RoutingPreviewUsecaseV5 {
	return &RoutingPreviewUsecaseV5{
		Providers: providers,
		Routes:    routes,
		Metrics:   metrics,
		Evidence:  evidence,
	}
}

func (uc *RoutingPreviewUsecaseV5) Handle(ctx context.Context, in run.RoutingPreviewInput) (run.RoutingPreviewResult, error) {
	// trim & defaults
	in.ProjectID = strings.TrimSpace(in.ProjectID)
	in.SubjectType = strings.TrimSpace(in.SubjectType)
	in.SubjectInternalID = strings.TrimSpace(in.SubjectInternalID)
	in.Region = strings.TrimSpace(in.Region)
	in.Currency = strings.TrimSpace(in.Currency)
	in.PaymentMethod = strings.TrimSpace(in.PaymentMethod)
	in.PolicyVersion = strings.TrimSpace(in.PolicyVersion)
	in.PipelineVersion = strings.TrimSpace(in.PipelineVersion)
	in.RoutingVersion = strings.TrimSpace(in.RoutingVersion)
	in.TraceID = strings.TrimSpace(in.TraceID)
	in.RunID = strings.TrimSpace(in.RunID)

	if in.RoutingVersion == "" {
		in.RoutingVersion = "v5"
	}

	// required
	if in.ProjectID == "" || in.SubjectType == "" || in.SubjectInternalID == "" {
		return run.RoutingPreviewResult{}, errors.New("project_id/subject_type/subject_internal_id are required")
	}
	if in.Region == "" || in.Currency == "" || in.PaymentMethod == "" {
		return run.RoutingPreviewResult{}, errors.New("region/currency/payment_method are required")
	}
	if in.PolicyVersion == "" || in.PipelineVersion == "" || in.TraceID == "" {
		return run.RoutingPreviewResult{}, errors.New("policy_version/pipeline_version/trace_id are required")
	}

	fp := v50ComputeRoutingFingerprint(in.RoutingInput)

	// load providers map (non-blocked)
	providers, err := uc.Providers.ListActive(ctx, in.ProjectID)
	if err != nil {
		return run.RoutingPreviewResult{}, err
	}
	pmap := map[string]run.ProviderRow{}
	for _, p := range providers {
		pmap[p.ProviderID] = p
	}

	// load candidate routes (repo already filters status=active)
	routes, err := uc.Routes.ListCandidates(ctx, in.ProjectID, in.Region, in.Currency, in.PaymentMethod)
	if err != nil {
		return run.RoutingPreviewResult{}, err
	}

	cands := make([]run.RoutingCandidate, 0, len(routes))
	for _, r := range routes {
		p, ok := pmap[r.ProviderID]
		if !ok || strings.ToLower(strings.TrimSpace(p.Status)) == "blocked" {
			cands = append(cands, run.RoutingCandidate{
				RouteID:        r.RouteID,
				ProviderID:     r.ProviderID,
				ProviderKey:    p.ProviderKey,
				Priority:       r.Priority,
				Excluded:       true,
				ExcludeReasons: []string{"provider_blocked_or_missing"},
				Score:          run.RoutingCandidateScore{TotalScore: -1},
			})
			continue
		}

		// metric snapshot (optional)
		ms, _ := uc.Metrics.GetLatestForRoute(ctx, in.ProjectID, r.RouteID)

		// priors
		successRate := 0.5
		p95ms := 1500
		costMinor := int64(100)

		if ms != nil {
			if ms.SuccessRate >= 0 && ms.SuccessRate <= 1 {
				successRate = ms.SuccessRate
			}
			if ms.P95LatencyMs > 0 {
				p95ms = ms.P95LatencyMs
			}
			if ms.AvgCostMinor >= 0 {
				costMinor = ms.AvgCostMinor
			}
		}

		successScore := successRate
		latencyScore := 1.0 - v50Clamp01(float64(p95ms)/5000.0)
		costScore := 1.0 - v50Clamp01(float64(costMinor)/500.0)

		wSuccess, wCost, wLatency := v50ParseWeights3(r.Weights)
		total := wSuccess*successScore + wCost*costScore + wLatency*latencyScore

		cands = append(cands, run.RoutingCandidate{
			RouteID:        r.RouteID,
			ProviderID:     r.ProviderID,
			ProviderKey:    p.ProviderKey,
			Priority:       r.Priority,
			Excluded:       false,
			ExcludeReasons: []string{},
			Score: run.RoutingCandidateScore{
				SuccessScore: successScore,
				CostScore:    costScore,
				LatencyScore: latencyScore,
				TotalScore:   total,
			},
		})
	}

	// deterministic tie-break
	sort.Slice(cands, func(i, j int) bool {
		if cands[i].Excluded != cands[j].Excluded {
			return !cands[i].Excluded
		}
		if cands[i].Score.TotalScore != cands[j].Score.TotalScore {
			return cands[i].Score.TotalScore > cands[j].Score.TotalScore
		}
		if cands[i].Priority != cands[j].Priority {
			return cands[i].Priority < cands[j].Priority
		}
		if cands[i].ProviderKey != cands[j].ProviderKey {
			return cands[i].ProviderKey < cands[j].ProviderKey
		}
		return cands[i].RouteID < cands[j].RouteID
	})

	var suggestedRoute, suggestedProv *string
	status := "suggested"
	var deniedReason *string

	for _, c := range cands {
		if c.Excluded {
			continue
		}
		rid := c.RouteID
		pid := c.ProviderID
		suggestedRoute = &rid
		suggestedProv = &pid
		break
	}
	if suggestedRoute == nil {
		status = "denied"
		dr := "no_candidates"
		deniedReason = &dr
	}

	whyReport := map[string]any{
		"input_fingerprint": fp,
		"input": map[string]any{
			"project_id":          in.ProjectID,
			"subject_type":        in.SubjectType,
			"subject_internal_id": in.SubjectInternalID,
			"region":              in.Region,
			"currency":            in.Currency,
			"payment_method":      in.PaymentMethod,
			"amount_minor":        in.AmountMinor,
			"policy_version":      in.PolicyVersion,
			"pipeline_version":    in.PipelineVersion,
			"routing_version":     in.RoutingVersion,
		},
		"candidates": cands,
		"suggested": map[string]any{
			"route_id":    suggestedRoute,
			"provider_id": suggestedProv,
		},
		"status":        status,
		"denied_reason": deniedReason,
	}

	whyBytes, _ := json.Marshal(whyReport)
	whyRef, err := uc.v50RegisterEvidenceJSON(ctx, in.ProjectID, in.TraceID, whyBytes, "routing_why_report", fp, "preview")
	if err != nil {
		return run.RoutingPreviewResult{}, err
	}

	return run.RoutingPreviewResult{
		Status:              status,
		InputFingerprint:    fp,
		Candidates:          cands,
		SuggestedRouteID:    suggestedRoute,
		SuggestedProviderID: suggestedProv,
		DeniedReason:        deniedReason,
		WhyEvidenceRef:      whyRef,
		TraceID:             in.TraceID,
		RunID:               in.RunID,
	}, nil
}

func (uc *RoutingPreviewUsecaseV5) v50RegisterEvidenceJSON(
	ctx context.Context,
	projectID, traceID string,
	b []byte,
	schema string,
	fingerprint string,
	stage string,
) (string, error) {
	sum := sha256.Sum256(b)
	sha := hex.EncodeToString(sum[:])

	idem := "v5:routing:" + schema + ":" + stage + ":" + fingerprint

	res, err := uc.Evidence.Register(ctx, run.EvidenceRegisterInput{
		ProjectID: projectID,
		TraceID:   traceID,

		ActorType: "system",
		ActorID:   nil,

		MediaType:     "text",
		MimeType:      "application/json",
		SourceKind:    "generated",
		SourceURI:     v50Ptr("routing://" + schema + "/" + stage),
		ContentSHA256: sha,
		ContentLength: int64(len(b)),

		Language:        nil,
		RetentionPolicy: "standard",
		ExpiresAtUTC:    nil,

		IdempotencyKey: idem,
	})
	if err != nil {
		return "", err
	}
	return res.EvidenceRef, nil
}

// ---- v50 helpers (namespaced to avoid collisions in package usecase) ----

func v50Ptr(s string) *string { return &s }

func v50Clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// weights json example: {"success":0.5,"cost":0.3,"latency":0.2}
func v50ParseWeights3(raw []byte) (wSuccess, wCost, wLatency float64) {
	wSuccess, wCost, wLatency = 0.5, 0.3, 0.2

	if len(raw) == 0 {
		return
	}

	var m map[string]any
	_ = json.Unmarshal(raw, &m)
	if m == nil {
		return
	}

	if v, ok := m["success"].(float64); ok {
		wSuccess = v
	}
	if v, ok := m["cost"].(float64); ok {
		wCost = v
	}
	if v, ok := m["latency"].(float64); ok {
		wLatency = v
	}

	sum := wSuccess + wCost + wLatency
	if sum <= 0 {
		return 0.5, 0.3, 0.2
	}
	wSuccess /= sum
	wCost /= sum
	wLatency /= sum
	return
}

// deterministic fingerprint: sha256(stable string)
// constraints_json is included via sha256(trimmed bytes)
func v50ComputeRoutingFingerprint(in run.RoutingInput) string {
	consSum := sha256.Sum256(v50BytesTrim(in.ConstraintsJSON))
	cons := hex.EncodeToString(consSum[:])

	s := strings.Join([]string{
		strings.TrimSpace(in.ProjectID),
		strings.TrimSpace(in.SubjectType),
		strings.TrimSpace(in.SubjectInternalID),
		strings.TrimSpace(in.Region),
		strings.TrimSpace(in.Currency),
		strings.TrimSpace(in.PaymentMethod),
		strconv.FormatInt(in.AmountMinor, 10),
		cons,
		strings.TrimSpace(in.PolicyVersion),
		strings.TrimSpace(in.PipelineVersion),
		strings.TrimSpace(in.RoutingVersion),
	}, "|")

	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func v50BytesTrim(b []byte) []byte {
	s := strings.TrimSpace(string(b))
	return []byte(s)
}
