package tests

import (
	"context"
	"sync"
	"testing"

	"example.com/pisag_go/run"
	"example.com/pisag_go/usecase"
)

type memRunRepo struct {
	mu     sync.Mutex
	runs   map[string]run.Run
	failed map[string]struct{ code, msg string }
	done   map[string]bool
}

func newMemRunRepo() *memRunRepo {
	return &memRunRepo{
		runs:   make(map[string]run.Run),
		failed: make(map[string]struct{ code, msg string }),
		done:   make(map[string]bool),
	}
}

func (m *memRunRepo) Create(ctx context.Context, r run.Run) (run.Run, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.runs[r.RunID] = r
	return r, nil
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

type memRunInputRepo struct {
	mu     sync.Mutex
	inputs []run.RunInput
}

func (m *memRunInputRepo) Insert(ctx context.Context, in run.RunInput) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.inputs = append(m.inputs, in)
	return nil
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

type fakeFetcher struct {
	err error
	res usecase.FetchResult
}

func (f fakeFetcher) Fetch(ctx context.Context, targetURL string) (usecase.FetchResult, error) {
	return f.res, f.err
}

func TestStartFetchRun_Success(t *testing.T) {
	ctx := context.Background()

	rr := newMemRunRepo()
	ir := &memRunInputRepo{}
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

	out, err := uc.Handle(ctx, usecase.StartFetchRunInput{
		ProjectID:       "p1",
		TargetURL:       "https://oracle.singularity.local/pricing_v1.json",
		PipelineVersion: "v4.1",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != "done" {
		t.Fatalf("expected done, got %s", out.Status)
	}
	if !rr.done[out.RunID] {
		t.Fatalf("expected run marked done")
	}
	if len(er.events) < 2 {
		t.Fatalf("expected >=2 events, got %d", len(er.events))
	}
}

func TestStartFetchRun_Denied_NoThrow(t *testing.T) {
	ctx := context.Background()

	rr := newMemRunRepo()
	ir := &memRunInputRepo{}
	er := &memRunEventRepo{}

	uc := usecase.StartFetchRunUseCase{
		Fetcher:      fakeFetcher{err: usecase.ErrDenied},
		RunRepo:      rr,
		RunInputRepo: ir,
		RunEventRepo: er,
	}

	out, err := uc.Handle(ctx, usecase.StartFetchRunInput{
		ProjectID:       "p1",
		TargetURL:       "https://evil.example.com/",
		PipelineVersion: "v4.1",
	})
	if err != nil {
		t.Fatalf("unexpected infra error: %v", err)
	}
	if out.Status != "failed" {
		t.Fatalf("expected failed, got %s", out.Status)
	}
	f, ok := rr.failed[out.RunID]
	if !ok {
		t.Fatalf("expected run marked failed")
	}
	if f.code != "fetch_denied" {
		t.Fatalf("expected fetch_denied, got %s", f.code)
	}
	if len(er.events) < 2 {
		t.Fatalf("expected >=2 events, got %d", len(er.events))
	}
}
