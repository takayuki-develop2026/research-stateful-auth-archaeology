package worm

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"services/wormexportersvc/internal/postgres"
	"services/wormexportersvc/internal/shared"
)

type Config struct {
	ProjectID string

	Sink   string // localfile (MVP)
	OutDir string

	Limit   int
	RunOnce bool
	Every   time.Duration

	// reclaim / hardening
	StaleAfter    time.Duration
	ReclaimFailed bool

	// IMPORTANT: if true, DO NOT MarkResult (leave started record)
	SkipMark bool

	MarkSummaryMax  int
	ExportSchemaVer string
}

type Exporter struct {
	repo *postgres.Repo
	cfg  Config
	sink Sink
}

func NewExporter(repo *postgres.Repo, cfg Config) (*Exporter, error) {
	if repo == nil {
		return nil, fmt.Errorf("repo required")
	}
	cfg.ProjectID = strings.TrimSpace(cfg.ProjectID)
	if cfg.ProjectID == "" {
		return nil, fmt.Errorf("ProjectID required")
	}

	if cfg.Limit <= 0 {
		cfg.Limit = 50
	}
	if cfg.Every <= 0 {
		cfg.Every = 60 * time.Second
	}
	if strings.TrimSpace(cfg.Sink) == "" {
		cfg.Sink = "localfile"
	}
	if strings.TrimSpace(cfg.OutDir) == "" {
		cfg.OutDir = "/var/wormexporter/out"
	}
	if cfg.StaleAfter <= 0 {
		cfg.StaleAfter = 5 * time.Minute
	}
	if cfg.MarkSummaryMax <= 0 {
		cfg.MarkSummaryMax = 256
	}
	if strings.TrimSpace(cfg.ExportSchemaVer) == "" {
		cfg.ExportSchemaVer = "v21.worm_export.1"
	}

	var sink Sink
	switch cfg.Sink {
	case "localfile":
		if err := os.MkdirAll(cfg.OutDir, 0o755); err != nil {
			return nil, fmt.Errorf("mkdir out_dir failed: %w (out=%s)", err, cfg.OutDir)
		}
		sink = NewLocalFileSink(cfg.OutDir)
	default:
		return nil, fmt.Errorf("unknown sink: %s", cfg.Sink)
	}

	return &Exporter{repo: repo, cfg: cfg, sink: sink}, nil
}

type exportPayloadV21 struct {
	ProjectID              string    `json:"project_id"`
	TraceID                string    `json:"trace_id"`
	EventID                int64     `json:"event_id"`
	EventType              string    `json:"event_type"`
	CreatedAtUTC           time.Time `json:"created_at_utc"`
	EventEvidenceAssetID   int64     `json:"event_evidence_asset_id"`
	PrimaryArtifactAssetID *int64    `json:"primary_artifact_asset_id,omitempty"`
	SchemaVersion          string    `json:"schema_version"`
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func (e *Exporter) Run(ctx context.Context) error {
	log.Printf(
		"[wormexportersvc] start project_id=%s sink=%s out=%s limit=%d once=%t every=%s stale_after=%s reclaim_failed=%t skip_mark=%t mark_summary_max=%d",
		e.cfg.ProjectID, e.cfg.Sink, e.cfg.OutDir, e.cfg.Limit, e.cfg.RunOnce, e.cfg.Every, e.cfg.StaleAfter, e.cfg.ReclaimFailed, e.cfg.SkipMark, e.cfg.MarkSummaryMax,
	)

	// immediate first run
	e.runOnce(ctx)

	if e.cfg.RunOnce {
		log.Printf("[wormexportersvc] done once")
		return nil
	}

	t := time.NewTicker(e.cfg.Every)
	defer t.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Printf("[wormexportersvc] stop: %v", ctx.Err())
			return nil
		case <-t.C:
			e.runOnce(ctx)
		}
	}
}

func (e *Exporter) runOnce(ctx context.Context) {
	claimed, err := e.repo.ClaimBatch(ctx, e.cfg.ProjectID, e.cfg.Limit, e.cfg.Sink, e.cfg.StaleAfter, e.cfg.ReclaimFailed)
	if err != nil {
		log.Printf("[wormexportersvc][claim] error: %v", err)
		return
	}
	log.Printf("[wormexportersvc][claim] claimed=%d", len(claimed))
	if len(claimed) == 0 {
		return
	}

	for _, ev := range claimed {
		objKey := shared.ObjectKeyV21(ev.ProjectID, ev.CreatedAtUTC, ev.ID, ev.TraceID, ev.EventType)

		payload := exportPayloadV21{
			ProjectID:              ev.ProjectID,
			TraceID:                ev.TraceID,
			EventID:                ev.ID,
			EventType:              ev.EventType,
			CreatedAtUTC:           ev.CreatedAtUTC,
			EventEvidenceAssetID:   ev.EventEvidenceAssetID,
			PrimaryArtifactAssetID: ev.PrimaryArtifactID,
			SchemaVersion:          e.cfg.ExportSchemaVer,
		}

		b, jerr := json.Marshal(payload)
		if jerr != nil {
			log.Printf("[wormexportersvc][export] JSON marshal failed event_id=%d trace_id=%s err=%v", ev.ID, ev.TraceID, jerr)

			if e.cfg.SkipMark {
				log.Printf("[wormexportersvc][mark] SKIP event_id=%d trace_id=%s (WORM_SKIP_MARK=true)", ev.ID, ev.TraceID)
				continue
			}

			_ = e.repo.MarkResult(
				ctx,
				ev.ProjectID,
				ev.ID,
				e.cfg.Sink,
				objKey,
				false,
				"json_marshal_failed",
				e.cfg.MarkSummaryMax,
			)
			continue
		}

		req := ExportRequest{
			ObjectKey:   objKey,
			Body:        b,
			ContentType: "application/json",
			Sha256:      sha256Hex(b),
		}

		res, werr := e.sink.Put(ctx, req)
		if werr != nil {
			log.Printf("[wormexportersvc][export] FAILED event_id=%d trace_id=%s sink=%s key=%s err=%v", ev.ID, ev.TraceID, e.cfg.Sink, objKey, werr)

			if e.cfg.SkipMark {
				log.Printf("[wormexportersvc][mark] SKIP event_id=%d trace_id=%s (WORM_SKIP_MARK=true)", ev.ID, ev.TraceID)
				continue
			}

			if merr := e.repo.MarkResult(
				ctx,
				ev.ProjectID,
				ev.ID,
				e.cfg.Sink,
				objKey,
				false,
				"sink_put_failed",
				e.cfg.MarkSummaryMax,
			); merr != nil {
				log.Printf("[wormexportersvc][mark] WARN event_id=%d trace_id=%s err=%v", ev.ID, ev.TraceID, merr)
			}
			continue
		}

		log.Printf(
			"[wormexportersvc][export] OK event_id=%d trace_id=%s sink=%s bytes=%d key=%s sha256=%s",
			ev.ID, ev.TraceID, res.Sink, res.Bytes, res.ObjectKey, res.Sha256,
		)

		// ✅ skipMark を本当に効かせる
		if e.cfg.SkipMark {
			log.Printf("[wormexportersvc][mark] SKIP event_id=%d trace_id=%s (WORM_SKIP_MARK=true)", ev.ID, ev.TraceID)
			continue
		}

		if merr := e.repo.MarkResult(
			ctx,
			ev.ProjectID,
			ev.ID,
			e.cfg.Sink,
			objKey,
			true,
			"exported:"+e.cfg.Sink,
			e.cfg.MarkSummaryMax,
		); merr != nil {
			log.Printf("[wormexportersvc][mark] WARN event_id=%d trace_id=%s err=%v", ev.ID, ev.TraceID, merr)
		}
	}
}