package usecase

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"example.com/pisag_go/run"
)

// PublishConfirmInput creates (idempotently) a publish commit proposal.
// v4.6: SoT is catalog_publish_commits.
// v4.7: default-deny approval ledger gates confirmation.
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

	// dev/local only:
	// - v4.7 default-deny: even if true, it confirms ONLY when approval is approved.
	AutoConfirm *bool
}

type PublishConfirmOutput struct {
	CommitID      string
	ProjectID     string
	CommitKey     string
	Status        string // proposed/confirmed/failed
	ManifestID    string
	ManifestHash  string
	TraceID       string
	Target        string
	FoundExisting bool

	ErrorCode    *string
	ErrorMessage *string
}

type PublishConfirmUseCase struct {
	PublishRepo  run.PublishRepo
	ApprovalRepo run.ApprovalRepo // v4.7: used for default-deny confirm gate (can be nil in v4.6)
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
		ProjectID:    strings.TrimSpace(in.ProjectID),
		CommitKey:    commitKey,
		ManifestID:   strings.TrimSpace(in.ManifestID),
		ManifestHash: strings.TrimSpace(in.ManifestHash),
		RunID:        in.RunID,
		TraceID:      strings.TrimSpace(in.TraceID),
		Target:       target,
		Status:       run.PublishStatusProposed,
		MetaJSON:     metaJSON,
	}

	// 1) Always create proposed commit (SoT), idempotent.
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

	// 2) v4.7 default-deny:
	// Confirm ONLY if:
	// - AutoConfirm=true (dev/local intent) AND
	// - ApprovalRepo exists AND
	// - approval_requests(project_id, commit_id).status == "approved"
	if boolDefaultFalse(in.AutoConfirm) {
		if uc.ApprovalRepo != nil {
			approved, _ := uc.isApproved(ctx, out.ProjectID, out.CommitID)
			if approved {
				_ = uc.PublishRepo.MarkConfirmed(ctx, out.CommitID)
				out.Status = run.PublishStatusConfirmed
			}
		}
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

func (uc *PublishConfirmUseCase) isApproved(ctx context.Context, projectID string, commitID string) (bool, error) {
	req, err := uc.ApprovalRepo.GetByProjectAndCommit(ctx, projectID, commitID)
	if err != nil {
		// not requested yet => not approved
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return req.Status == run.ApprovalStatusApproved, nil
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