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
	db   *postgres.DB
	cfg  Config
	sink Sink
}

func NewExporter(repo *postgres.Repo, db *postgres.DB, cfg Config) (*Exporter, error) {
	if repo == nil {
		return nil, fmt.Errorf("repo required")
	}
	if db == nil {
		return nil, fmt.Errorf("db required")
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

	return &Exporter{repo: repo, db: db, cfg: cfg, sink: sink}, nil
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

type exportResultEvidence struct {
	SchemaVersion string `json:"schema_version"`

	ProjectID string `json:"project_id"`
	TraceID   string `json:"trace_id"`
	EventID   int64  `json:"event_id"`
	EventType string `json:"event_type"`

	Sink        string `json:"sink"`
	ObjectKey   string `json:"object_key"`
	ContentType string `json:"content_type"`
	Bytes       int64  `json:"bytes"`
	Sha256      string `json:"sha256"`

	Status    string `json:"status"` // succeeded|failed
	ErrorCode string `json:"error_code,omitempty"`
	ErrorMsg  string `json:"error_message,omitempty"`

	ExportedAtUTC string `json:"exported_at_utc"`
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

			// evidence (failure)
			evAssetID := e.registerExportResultEvidence(ctx, ev, objKey, "application/json", 0, "", "failed", "json_marshal_failed", jerr.Error())
			_ = e.repo.MarkResult(
				ctx,
				ev.ProjectID,
				ev.ID,
				e.cfg.Sink,
				objKey,
				false,
				"json_marshal_failed",
				e.cfg.MarkSummaryMax,
				evAssetID,
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

			evAssetID := e.registerExportResultEvidence(ctx, ev, objKey, req.ContentType, int64(len(b)), req.Sha256, "failed", "sink_put_failed", werr.Error())

			if merr := e.repo.MarkResult(
				ctx,
				ev.ProjectID,
				ev.ID,
				e.cfg.Sink,
				objKey,
				false,
				"sink_put_failed",
				e.cfg.MarkSummaryMax,
				evAssetID,
			); merr != nil {
				log.Printf("[wormexportersvc][mark] WARN event_id=%d trace_id=%s err=%v", ev.ID, ev.TraceID, merr)
			}
			continue
		}

		log.Printf(
			"[wormexportersvc][export] OK event_id=%d trace_id=%s sink=%s bytes=%d key=%s sha256=%s",
			ev.ID, ev.TraceID, res.Sink, res.Bytes, res.ObjectKey, res.Sha256,
		)

		// ✅ skipMark を本当に効かせる（MarkResultもevidence登録もスキップ）
		if e.cfg.SkipMark {
			log.Printf("[wormexportersvc][mark] SKIP event_id=%d trace_id=%s (WORM_SKIP_MARK=true)", ev.ID, ev.TraceID)
			continue
		}

		evAssetID := e.registerExportResultEvidence(ctx, ev, objKey, req.ContentType, res.Bytes, res.Sha256, "succeeded", "", "")

		if merr := e.repo.MarkResult(
			ctx,
			ev.ProjectID,
			ev.ID,
			e.cfg.Sink,
			objKey,
			true,
			"exported:"+e.cfg.Sink,
			e.cfg.MarkSummaryMax,
			evAssetID,
		); merr != nil {
			log.Printf("[wormexportersvc][mark] WARN event_id=%d trace_id=%s err=%v", ev.ID, ev.TraceID, merr)
		}
	}
}

func (e *Exporter) registerExportResultEvidence(
	ctx context.Context,
	ev shared.ComplianceEvent,
	objKey string,
	contentType string,
	bytes int64,
	sha string,
	status string,
	errCode string,
	errMsg string,
) int64 {
	// best-effort: evidence登録に失敗しても throw しない（E条文の理想は満たせないが、処理は継続）
	body := exportResultEvidence{
		SchemaVersion: "v21.worm_export_result.1",
		ProjectID:     ev.ProjectID,
		TraceID:       ev.TraceID,
		EventID:       ev.ID,
		EventType:     ev.EventType,
		Sink:          e.cfg.Sink,
		ObjectKey:     objKey,
		ContentType:   contentType,
		Bytes:         bytes,
		Sha256:        sha,
		Status:        status,
		ErrorCode:     errCode,
		ErrorMsg:      errMsg,
		ExportedAtUTC: time.Now().UTC().Format(time.RFC3339Nano),
	}

	idem := fmt.Sprintf("wormexportersvc:export_result:%s:%d:%s", ev.ProjectID, ev.ID, e.cfg.Sink)
	sourceURI := fmt.Sprintf("wormexportersvc://export_result/%s/%s", e.cfg.Sink, objKey)

	assetID, err := postgres.RegisterTextEvidenceAssetV18(
		ctx,
		e.db,
		ev.ProjectID,
		ev.TraceID,
		"service",
		"wormexportersvc",
		"generated",
		sourceURI,
		body,
		idem,
	)
	if err != nil {
		log.Printf("[wormexportersvc][evidence] WARN export_result evidence_register failed event_id=%d trace_id=%s err=%v", ev.ID, ev.TraceID, err)
		return 0
	}
	return assetID
}