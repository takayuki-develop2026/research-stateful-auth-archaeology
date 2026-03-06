package adminapi

import (
	"net/http"
	"time"

	run "example.com/pisag_go/run"
	"example.com/pisag_go/usecase"
)

type V17MobileDevHandler struct {
	SeedInboxUC *usecase.DevSeedMobileInboxUseCase
}

type devSeedInboxRequest struct {
	ProjectID       string `json:"project_id"`
	ActorUserID     string `json:"actor_user_id"`
	AssignedUserID  string `json:"assigned_user_id"`
	ItemKind        string `json:"item_kind"`
	SourceType      string `json:"source_type"`
	SourceID        string `json:"source_id"`
	RunID           string `json:"run_id"`
	Priority        string `json:"priority"`
	Severity        string `json:"severity"`
	Title           string `json:"title"`
	Summary         string `json:"summary"`
	StepUpRequired  bool   `json:"stepup_required"`
	CommentRequired bool   `json:"comment_required"`
	CanApprove      bool   `json:"can_approve"`
	CanReject       bool   `json:"can_reject"`
	CanAck          bool   `json:"can_ack"`
	DueAtRFC3339    string `json:"due_at_rfc3339"`
}

func (h *V17MobileDevHandler) SeedInbox(w http.ResponseWriter, r *http.Request) {
	traceID := ensureTraceIDFromHeader(r)
	if h.SeedInboxUC == nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "seed inbox usecase is nil", traceID)
		return
	}

	var req devSeedInboxRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), traceID)
		return
	}

	var dueAt *time.Time
	if req.DueAtRFC3339 != "" {
		t, err := time.Parse(time.RFC3339, req.DueAtRFC3339)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_due_at", err.Error(), traceID)
			return
		}
		dueAt = &t
	}

	out, err := h.SeedInboxUC.Handle(r.Context(), usecase.DevSeedMobileInboxInput{
		ProjectID:       req.ProjectID,
		ActorUserID:     req.ActorUserID,
		AssignedUserID:  req.AssignedUserID,
		ItemKind:        run.MobileInboxItemKind(req.ItemKind),
		SourceType:      req.SourceType,
		SourceID:        req.SourceID,
		RunID:           req.RunID,
		Priority:        run.MobilePriority(req.Priority),
		Severity:        run.MobileSeverity(req.Severity),
		Title:           req.Title,
		Summary:         req.Summary,
		StepUpRequired:  req.StepUpRequired,
		CommentRequired: req.CommentRequired,
		CanApprove:      req.CanApprove,
		CanReject:       req.CanReject,
		CanAck:          req.CanAck,
		DueAt:           dueAt,
		TraceID:         traceID,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, "seed_inbox_failed", err.Error(), traceID)
		return
	}

	writeJSON(w, http.StatusCreated, traceID, map[string]any{
		"item": out.Item,
	})
}