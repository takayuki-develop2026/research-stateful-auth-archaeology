package usecase

import (
	"context"
	"errors"
	"fmt"
	"time"

	"ledgersvc/postgres"
)



type V14CreatePosting struct {
	repo LedgerRepo
}

func NewV14CreatePosting(repo LedgerRepo) *V14CreatePosting {
	return &V14CreatePosting{repo: repo}
}

type V14CreatePostingInput struct {
	ProjectID       string
	PostingKey      string
	SourceEventKey  string
	PostingType     string
	Currency        string
	PostedAt        time.Time
	RunID           string
	TraceID         string
	PolicyVersionID string

	Lines             []postgres.EntryInput
	EvidenceRefs       []string
	AppendEvidenceRefs []string
}

type V14CreatePostingOutput struct {
	PostingID   string
	Status      string // posted | already_exists_posted
	DebitTotal  int64
	CreditTotal int64
}

// fail-closed:
// - 例外や予期しない不整合は error を返す（呼び出し側が run_events / alert / DLQ に落とす）
// - finalize が posted でない場合は必ず error（会計SoTとしては「成功扱い」にしない）
func (uc *V14CreatePosting) Handle(ctx context.Context, in V14CreatePostingInput) (V14CreatePostingOutput, error) {
	if err := validate(in); err != nil {
		return V14CreatePostingOutput{}, err
	}

	cr, err := uc.repo.CreatePosting(ctx, postgres.PostingCreateParams{
		ProjectID:       in.ProjectID,
		PostingKey:      in.PostingKey,
		SourceEventKey:  in.SourceEventKey,
		PostingType:     in.PostingType,
		Currency:        in.Currency,
		PostedAt:        in.PostedAt,
		RunID:           in.RunID,
		TraceID:         in.TraceID,
		PolicyVersionID: in.PolicyVersionID,
		EvidenceRefs:    in.EvidenceRefs,
	})
	if err != nil {
		return V14CreatePostingOutput{}, fmt.Errorf("create_posting: %w", err)
	}

	if err := uc.repo.InsertEntries(ctx, cr.PostingID, in.Lines); err != nil {
		return V14CreatePostingOutput{}, fmt.Errorf("insert_entries: %w", err)
	}

	fr, err := uc.repo.FinalizePosting(ctx, cr.PostingID, in.AppendEvidenceRefs)
	if err != nil {
		return V14CreatePostingOutput{}, fmt.Errorf("finalize_posting: %w", err)
	}

	if fr.Status != "posted" {
		return V14CreatePostingOutput{}, fmt.Errorf("%w: posting_id=%s status=%s debit=%d credit=%d",
			ErrPostingNotPosted, fr.PostingID, fr.Status, fr.DebitTotal, fr.CreditTotal)
	}

	status := "posted"
	if cr.Status == "already_exists" {
		status = "already_exists_posted"
	}

	return V14CreatePostingOutput{
		PostingID:   fr.PostingID,
		Status:      status,
		DebitTotal:  fr.DebitTotal,
		CreditTotal: fr.CreditTotal,
	}, nil
}

var ErrPostingNotPosted = errors.New("posting_not_posted")

func validate(in V14CreatePostingInput) error {
	if in.ProjectID == "" || in.PostingKey == "" || in.SourceEventKey == "" || in.PostingType == "" ||
		in.Currency == "" || in.RunID == "" || in.TraceID == "" || in.PolicyVersionID == "" {
		return fmt.Errorf("%w: required field missing", postgres.ErrInvalidArgument)
	}
	if in.PostedAt.IsZero() {
		return fmt.Errorf("%w: posted_at is required", postgres.ErrInvalidArgument)
	}
	if len(in.Lines) == 0 {
		return fmt.Errorf("%w: lines is required", postgres.ErrInvalidArgument)
	}
	for i := range in.Lines {
		l := in.Lines[i]
		if l.AccountKey == "" || l.EntryKey == "" || l.Direction == "" || l.Currency == "" {
			return fmt.Errorf("%w: line[%d] missing required field", postgres.ErrInvalidArgument, i)
		}
		if l.Currency != in.Currency {
			return fmt.Errorf("%w: line[%d] currency must match header currency", postgres.ErrInvalidArgument, i)
		}
		if l.Amount < 0 {
			return fmt.Errorf("%w: line[%d] amount must be >=0", postgres.ErrInvalidArgument, i)
		}
	}
	return nil
}

func IsPermissionDenied(err error) bool { return errors.Is(err, postgres.ErrPermissionDenied) }
func IsUnknownAccount(err error) bool   { return errors.Is(err, postgres.ErrUnknownAccount) }