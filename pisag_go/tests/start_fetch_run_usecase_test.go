package tests

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"testing"

	"example.com/pisag_go/run"
	"example.com/pisag_go/usecase"
)

/* =========================
   helpers
========================= */

func hashHex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

/* =========================
   mem repos
========================= */

type memRunRepo struct {
	mu     sync.Mutex
	byID   map[string]run.Run
	byKey  map[string]string // projectID|runKey -> runID
	failed map[string]struct{ code, msg string }
	done   map[string]bool
}

func newMemRunRepo() *memRunRepo {
	return &memRunRepo{
		byID:   make(map[string]run.Run),
		byKey:  make(map[string]string),
		failed: make(map[string]struct{ code, msg string }),
		done:   make(map[string]bool),
	}
}

func (m *memRunRepo) Create(ctx context.Context, r run.Run) (run.Run, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.byID[r.RunID] = r
	// run_key があれば index
	if r.RunKey != nil && *r.RunKey != "" {
		m.byKey[r.ProjectID+"|"+*r.RunKey] = r.RunID
	}
	return r, nil
}

func (m *memRunRepo) CreateOrGetByRunKey(
	ctx context.Context,
	projectID string,
	runKey string,
	newRun func() run.Run,
) (run.Run, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	k := projectID + "|" + runKey
	if runID, ok := m.byKey[k]; ok {
		ex := m.byID[runID]
		return ex, true, nil
	}

	rr := newRun()
	// 必須の埋め（念のため）
	if rr.ProjectID == "" {
		rr.ProjectID = projectID
	}
	rk := runKey
	rr.RunKey = &rk

	m.byID[rr.RunID] = rr
	m.byKey[k] = rr.RunID
	return rr, false, nil
}

func (m *memRunRepo) MarkDone(ctx context.Context, runID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.done[runID] = true
	return nil
}

func (m *memRunRepo) MarkFailed(ctx context.Context, runID, code, msg string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.failed[runID] = struct{ code, msg string }{code, msg}
	return nil
}

// interface が要求しているなら実装（postgres側にあるため）
func (m *memRunRepo) GetTraceID(ctx context.Context, runID string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if r, ok := m.byID[runID]; ok {
		return r.TraceID, nil
	}
	return "", nil
}

type memRunInputRepo struct {
	mu   sync.Mutex
	byUK map[string]run.RunInput // run_id|enqueue_key -> RunInput
}

func newMemRunInputRepo() *memRunInputRepo {
	return &memRunInputRepo{
		byUK: make(map[string]run.RunInput),
	}
}

func (m *memRunInputRepo) Insert(ctx context.Context, in run.RunInput) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	uk := in.RunID + "|" + in.EnqueueKey
	// ON CONFLICT DO NOTHING 相当
	if _, ok := m.byUK[uk]; ok {
		return nil
	}
	m.byUK[uk] = in
	return nil
}

func (m *memRunInputRepo) CountByRunAndEnqueueKey(runID, enqueueKey string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	uk := runID + "|" + enqueueKey
	if _, ok := m.byUK[uk]; ok {
		return 1
	}
	return 0
}

// ClaimNext はこのテストでは使わない（worker側テストではない）
// ただし過去互換で interface に要求されている場合に備えてダミー実装を置く。
func (m *memRunInputRepo) ClaimNext(ctx context.Context, workerID string) (*run.ClaimedRunInput, error) {
	return nil, nil
}

type memRunEventRepo struct {
	mu     sync.Mutex
	events []run.RunEvent
}

func (m *memRunEventRepo) Append(ctx context.Context, ev run.RunEvent) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, ev)
	return nil
}

/* =========================
   fake fetcher
========================= */

type fakeFetcher struct {
	err error
	res usecase.FetchResult
}

func (f fakeFetcher) Fetch(ctx context.Context, targetURL string) (usecase.FetchResult, error) {
	return f.res, f.err
}

/* =========================
   tests
========================= */

func TestStartFetchRun_ReuseRunDefaultTrue_And_EnqueueIdempotent(t *testing.T) {
	ctx := context.Background()

	rr := newMemRunRepo()
	ir := newMemRunInputRepo()
	er := &memRunEventRepo{}

	uc := usecase.StartFetchRunUseCase{
		Fetcher: fakeFetcher{res: usecase.FetchResult{
			FinalURL:    "https://oracle.singularity.local/pricing_v1.json",
			StatusCode:  200,
			ContentType: "application/json",
			BodySize:    123,
		}},
		RunRepo:      rr,
		RunInputRepo: ir,
		RunEventRepo: er,
	}

	allowKey := "oracle" // ✅ v4 fixed: allowlist_key is required (fail-closed)

	in := usecase.StartFetchRunInput{
		ProjectID:       "p1",
		TargetURL:       "https://oracle.singularity.local/pricing_v1.json",
		PipelineVersion: "v4.1",
		AllowlistKey:    &allowKey, // ✅ 必須
		ImmediateFetch:  false,     // worker主体
		ReuseRun:        nil,       // ✅ デフォルトtrue確認
	}

	out1, err := uc.Handle(ctx, in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out2, err := uc.Handle(ctx, in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// ✅ run reuse
	if out1.RunID != out2.RunID {
		t.Fatalf("expected same RunID (reuse), got %s vs %s", out1.RunID, out2.RunID)
	}

	// ✅ enqueue idempotency
	// enqueueKey の作り方は usecase と一致させる必要がある（ここでは同じ式で再計算）
	nurl, _ := usecase.NormalizeURLForEnqueueKey_ForTest(in.TargetURL)
	method := "GET"
	allow := "oracle"
	enqueueKey := hashHex("fetch|" + method + "|" + allow + "|" + nurl)

	if got := ir.CountByRunAndEnqueueKey(out1.RunID, enqueueKey); got != 1 {
		t.Fatalf("expected run_inputs=1 for (run_id, enqueue_key), got %d", got)
	}

	if out1.Status != "enqueued" || out2.Status != "enqueued" {
		t.Fatalf("expected enqueued, got out1=%s out2=%s", out1.Status, out2.Status)
	}
}