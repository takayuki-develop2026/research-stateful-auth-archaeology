package usecase

import (
	"context"
	"fmt"
	"strings"
	"time"

	run "example.com/pisag_go/run"

	"github.com/google/uuid"
)

type DevSeedMobileInboxUseCase struct {
	Inbox run.MobileInboxRepository
}

type DevSeedMobileInboxInput struct {
	ProjectID       string
	ActorUserID     string
	AssignedUserID  string
	ItemKind        run.MobileInboxItemKind
	SourceType      string
	SourceID        string
	RunID           string
	Priority        run.MobilePriority
	Severity        run.MobileSeverity
	Title           string
	Summary         string
	StepUpRequired  bool
	CommentRequired bool
	CanApprove      bool
	CanReject       bool
	CanAck          bool
	DueAt           *time.Time
	TraceID         string
}

type DevSeedMobileInboxOutput struct {
	Item run.MobileInboxItem
}

func (uc *DevSeedMobileInboxUseCase) Handle(ctx context.Context, in DevSeedMobileInboxInput) (DevSeedMobileInboxOutput, error) {
	if uc.Inbox == nil {
		return DevSeedMobileInboxOutput{}, fmt.Errorf("dev seed mobile inbox: inbox repository is nil")
	}
	if strings.TrimSpace(in.ProjectID) == "" {
		return DevSeedMobileInboxOutput{}, fmt.Errorf("dev seed mobile inbox: project_id is required")
	}
	if strings.TrimSpace(in.Title) == "" {
		return DevSeedMobileInboxOutput{}, fmt.Errorf("dev seed mobile inbox: title is required")
	}

	traceID := ensureV17TraceID(in.TraceID)

	itemKind := in.ItemKind
	if itemKind == "" {
		itemKind = run.MobileInboxItemApprovalRequest
	}

	sourceType := strings.TrimSpace(in.SourceType)
	if sourceType == "" {
		sourceType = string(itemKind)
	}

	sourceID := strings.TrimSpace(in.SourceID)
	if sourceID == "" {
		sourceID = "src_" + uuid.NewString()
	}

	runID := strings.TrimSpace(in.RunID)
	if runID == "" {
		runID = "run_" + uuid.NewString()
	}

	priority := in.Priority
	if priority == "" {
		priority = run.MobilePriorityNormal
	}

	severity := in.Severity
	if severity == "" {
		severity = run.MobileSeverityInfo
	}

	assignedUserID := strings.TrimSpace(in.AssignedUserID)
	if assignedUserID == "" {
		assignedUserID = strings.TrimSpace(in.ActorUserID)
	}

	item, err := uc.Inbox.Create(ctx, run.CreateMobileInboxItemInput{
		PublicID:               "minbox_" + uuid.NewString(),
		ProjectID:              in.ProjectID,
		InboxStatus:            run.MobileInboxStatusPending,
		ItemKind:               itemKind,
		SourceType:             sourceType,
		SourceID:               sourceID,
		RunID:                  runID,
		TraceID:                traceID,
		ActorUserID:            strings.TrimSpace(in.ActorUserID),
		AssignedUserID:         assignedUserID,
		Priority:               priority,
		Severity:               severity,
		Title:                  strings.TrimSpace(in.Title),
		Summary:                strings.TrimSpace(in.Summary),
		ActionRequired:         true,
		StepUpRequired:         in.StepUpRequired,
		CommentRequired:        in.CommentRequired,
		AvailableActionApprove: in.CanApprove,
		AvailableActionReject:  in.CanReject,
		AvailableActionAck:     in.CanAck,
		TerminalReasonCode:     "",
		SourceOccurredAt:       ptrTimeV17(time.Now().UTC()),
		FirstPresentedAt:       ptrTimeV17(time.Now().UTC()),
		DueAt:                  in.DueAt,
	})
	if err != nil {
		return DevSeedMobileInboxOutput{}, fmt.Errorf("dev seed mobile inbox create: %w", err)
	}

	return DevSeedMobileInboxOutput{
		Item: item,
	}, nil
}

func ptrTimeV17(t time.Time) *time.Time {
	return &t
}