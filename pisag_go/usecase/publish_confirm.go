package usecase

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"example.com/pisag_go/run"
)

// PublishConfirmInput creates (idempotently) a publish commit proposal.
// v4.6: SoT is catalog_publish_commits.
// v4.7: approval ledger will gate confirmation.
type PublishConfirmInput struct {
	ProjectID string

	// evidence input
	ManifestID   string
	ManifestHash string

	// traceability
	TraceID string
	RunID   *string

	Target string // default "catalog_v1"

	// Idempotency key (recommended). If empty, generated from inputs.
	CommitKey *string

	// Optional meta snapshot (non-SoT, audit convenience)
	Meta map[string]any

	// dev/local only: allow immediate confirm (default false)
	AutoConfirm *bool
}

type PublishConfirmOutput struct {
	CommitID     string
	ProjectID    string
	CommitKey    string
	Status       string // proposed/confirmed/failed
	ManifestID   string
	ManifestHash string
	TraceID      string
	Target       string
	FoundExisting bool

	ErrorCode    *string
	ErrorMessage *string
}

type PublishConfirmUseCase struct {
	PublishRepo run.PublishRepo
}

func (uc *PublishConfirmUseCase) Handle(ctx context.Context, in PublishConfirmInput) (PublishConfirmOutput, error) {
	if strings.TrimSpace(in.ProjectID) == "" {
		return PublishConfirmOutput{}, errors.New("project_id is required")
	}
	if strings.TrimSpace(in.ManifestID) == "" {
		return PublishConfirmOutput{}, errors.New("manifest_id is required")
	}
	if strings.TrimSpace(in.ManifestHash) == "" {
		return PublishConfirmOutput{}, errors.New("manifest_hash is required")
	}
	if strings.TrimSpace(in.TraceID) == "" {
		return PublishConfirmOutput{}, errors.New("trace_id is required")
	}
	target := strings.TrimSpace(in.Target)
	if target == "" {
		target = "catalog_v1"
	}

	commitKey := ""
	if in.CommitKey != nil {
		commitKey = strings.TrimSpace(*in.CommitKey)
	}
	if commitKey == "" {
		commitKey = buildCommitKey(in.ProjectID, target, in.ManifestHash)
	}

	metaJSON := []byte(`{}`)
	if in.Meta != nil {
		// inject a minimal timestamp for audit convenience (does not affect idempotency)
		if _, ok := in.Meta["ts_utc"]; !ok {
			in.Meta["ts_utc"] = time.Now().UTC().Format(time.RFC3339Nano)
		}
		b, err := json.Marshal(in.Meta)
		if err == nil && len(b) > 0 {
			metaJSON = b
		}
	}

	pc := run.PublishCommit{
		ProjectID:     strings.TrimSpace(in.ProjectID),
		CommitKey:     commitKey,
		ManifestID:    strings.TrimSpace(in.ManifestID),
		ManifestHash:  strings.TrimSpace(in.ManifestHash),
		RunID:         in.RunID,
		TraceID:       strings.TrimSpace(in.TraceID),
		Target:        target,
		Status:        run.PublishStatusProposed,
		MetaJSON:      metaJSON,
	}

	out, found, err := uc.PublishRepo.CreateProposed(ctx, pc)
	if err != nil {
		code := "publish_propose_failed"
		msg := err.Error()
		return PublishConfirmOutput{
			CommitID:      "",
			ProjectID:     pc.ProjectID,
			CommitKey:     pc.CommitKey,
			Status:        run.PublishStatusFailed,
			ManifestID:    pc.ManifestID,
			ManifestHash:  pc.ManifestHash,
			TraceID:       pc.TraceID,
			Target:        pc.Target,
			FoundExisting: false,
			ErrorCode:     &code,
			ErrorMessage:  &msg,
		}, nil
	}

	// dev/local only: optional auto confirm (default false)
	if boolDefaultFalse(in.AutoConfirm) {
		_ = uc.PublishRepo.MarkConfirmed(ctx, out.CommitID)
		// read back for status (optional). To keep minimal, reflect confirmed here.
		out.Status = run.PublishStatusConfirmed
	}

	return PublishConfirmOutput{
		CommitID:      out.CommitID,
		ProjectID:     out.ProjectID,
		CommitKey:     out.CommitKey,
		Status:        out.Status,
		ManifestID:    out.ManifestID,
		ManifestHash:  out.ManifestHash,
		TraceID:       out.TraceID,
		Target:        out.Target,
		FoundExisting: found,
		ErrorCode:     out.ErrorCode,
		ErrorMessage:  out.ErrorMessage,
	}, nil
}

// buildCommitKey MUST NOT include run_id.
// Recommended: sha256("publish|project|target|manifest_hash")
func buildCommitKey(projectID, target, manifestHash string) string {
	s := "publish|" + strings.TrimSpace(projectID) + "|" + strings.TrimSpace(target) + "|" + strings.TrimSpace(manifestHash)
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func boolDefaultFalse(p *bool) bool {
	if p == nil {
		return false
	}
	return *p
}