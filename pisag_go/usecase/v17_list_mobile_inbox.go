package usecase

import (
	"context"
	"fmt"

	run "example.com/pisag_go/run"
)

type ListMobileInboxUseCase struct {
	Inbox run.MobileInboxRepository
}

type ListMobileInboxInput struct {
	ProjectID      string
	AssignedUserID string
	ActorUserID    string
	Status         run.MobileInboxStatus
	ItemKind       run.MobileInboxItemKind
	Priority       run.MobilePriority
	Severity       run.MobileSeverity
	OnlyActionable bool
	Limit          int
	Offset         int
}

type ListMobileInboxOutput struct {
	Items []run.MobileInboxItem
}

func (uc *ListMobileInboxUseCase) Handle(ctx context.Context, in ListMobileInboxInput) (ListMobileInboxOutput, error) {
	if uc.Inbox == nil {
		return ListMobileInboxOutput{}, fmt.Errorf("list mobile inbox: inbox repository is nil")
	}
	if in.ProjectID == "" {
		return ListMobileInboxOutput{}, fmt.Errorf("list mobile inbox: project_id is required")
	}
	if in.Limit <= 0 {
		in.Limit = 20
	}
	if in.Limit > 200 {
		in.Limit = 200
	}
	if in.Offset < 0 {
		in.Offset = 0
	}

	items, err := uc.Inbox.List(ctx, run.ListMobileInboxItemsFilter{
		ProjectID:      in.ProjectID,
		AssignedUserID: in.AssignedUserID,
		ActorUserID:    in.ActorUserID,
		Status:         in.Status,
		ItemKind:       in.ItemKind,
		Priority:       in.Priority,
		Severity:       in.Severity,
		OnlyActionable: in.OnlyActionable,
		Limit:          in.Limit,
		Offset:         in.Offset,
	})
	if err != nil {
		return ListMobileInboxOutput{}, fmt.Errorf("list mobile inbox: %w", err)
	}

	return ListMobileInboxOutput{
		Items: items,
	}, nil
}
