package adminapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	run "example.com/pisag_go/run"
	"example.com/pisag_go/usecase"
)

type V17MobileHandler struct {
	RegisterDeviceUC *usecase.RegisterMobileDeviceUseCase
	RequestStepUpUC  *usecase.RequestMobileStepUpUseCase
	VerifyStepUpUC   *usecase.VerifyMobileStepUpUseCase
	ListInboxUC      *usecase.ListMobileInboxUseCase
	AckItemUC        *usecase.AckMobileInboxItemUseCase
	ApproveItemUC    *usecase.ApproveMobileInboxItemUseCase
	RejectItemUC     *usecase.RejectMobileInboxItemUseCase
}

type errorEnvelope struct {
	Error struct {
		Type    string `json:"type"`
		Message string `json:"message"`
		TraceID string `json:"trace_id"`
	} `json:"error"`
}

type registerMobileDeviceRequest struct {
	ProjectID           string `json:"project_id"`
	ActorUserID         string `json:"actor_user_id"`
	DeviceLabel         string `json:"device_label"`
	PlatformType        string `json:"platform_type"`
	AppChannel          string `json:"app_channel"`
	DeviceFingerprint   string `json:"device_fingerprint"`
	DeviceKeyID         string `json:"device_key_id"`
	DevicePublicKeyPEM  string `json:"device_public_key_pem"`
	AttestationFormat   string `json:"attestation_format"`
	AttestationSubject  string `json:"attestation_subject"`
	CreatedRunID        string `json:"created_run_id"`
	ActivateImmediately bool   `json:"activate_immediately"`
}

type requestStepUpRequest struct {
	ProjectID            string `json:"project_id"`
	ActorUserID          string `json:"actor_user_id"`
	DevicePublicID       string `json:"device_public_id"`
	ActionKind           string `json:"action_kind"`
	InboxItemPublicID    string `json:"inbox_item_public_id"`
	TargetSourceType     string `json:"target_source_type"`
	TargetSourceID       string `json:"target_source_id"`
	RunID                string `json:"run_id"`
	StepUpMethod         string `json:"stepup_method"`
	TTLSeconds           int    `json:"ttl_seconds"`
	ProvidedChallengeVal string `json:"provided_challenge_value"`
}

type verifyStepUpRequest struct {
	ProjectID         string `json:"project_id"`
	ActorUserID       string `json:"actor_user_id"`
	DevicePublicID    string `json:"device_public_id"`
	ChallengePublicID string `json:"challenge_public_id"`
	ActionKind        string `json:"action_kind"`
	VerificationValue string `json:"verification_value"`
}

type ackInboxRequest struct {
	ProjectID         string `json:"project_id"`
	ActorUserID       string `json:"actor_user_id"`
	DevicePublicID    string `json:"device_public_id"`
	StepUpChallengeID string `json:"stepup_challenge_id"`
	IdempotencyKey    string `json:"idempotency_key"`
	CommentText       string `json:"comment_text"`
}

type approveInboxRequest struct {
	ProjectID         string `json:"project_id"`
	ActorUserID       string `json:"actor_user_id"`
	DevicePublicID    string `json:"device_public_id"`
	StepUpChallengeID string `json:"stepup_challenge_id"`
	IdempotencyKey    string `json:"idempotency_key"`
	CommentText       string `json:"comment_text"`
}

type rejectInboxRequest struct {
	ProjectID         string `json:"project_id"`
	ActorUserID       string `json:"actor_user_id"`
	DevicePublicID    string `json:"device_public_id"`
	StepUpChallengeID string `json:"stepup_challenge_id"`
	IdempotencyKey    string `json:"idempotency_key"`
	CommentText       string `json:"comment_text"`
}

func (h *V17MobileHandler) RegisterDevice(w http.ResponseWriter, r *http.Request) {
	traceID := ensureTraceIDFromHeader(r)
	if h.RegisterDeviceUC == nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "register device usecase is nil", traceID)
		return
	}

	var req registerMobileDeviceRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), traceID)
		return
	}

	out, err := h.RegisterDeviceUC.Handle(r.Context(), usecase.RegisterMobileDeviceInput{
		ProjectID:           req.ProjectID,
		ActorUserID:         req.ActorUserID,
		DeviceLabel:         req.DeviceLabel,
		PlatformType:        run.MobilePlatformType(req.PlatformType),
		AppChannel:          run.MobileAppChannel(req.AppChannel),
		DeviceFingerprint:   req.DeviceFingerprint,
		DeviceKeyID:         req.DeviceKeyID,
		DevicePublicKeyPEM:  req.DevicePublicKeyPEM,
		AttestationFormat:   req.AttestationFormat,
		AttestationSubject:  req.AttestationSubject,
		CreatedRunID:        req.CreatedRunID,
		TraceID:             traceID,
		ActivateImmediately: req.ActivateImmediately,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, "register_failed", err.Error(), traceID)
		return
	}

	writeJSON(w, http.StatusCreated, traceID, map[string]any{
		"device": out.Device,
	})
}

func (h *V17MobileHandler) RequestStepUp(w http.ResponseWriter, r *http.Request) {
	traceID := ensureTraceIDFromHeader(r)
	if h.RequestStepUpUC == nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "request stepup usecase is nil", traceID)
		return
	}

	var req requestStepUpRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), traceID)
		return
	}

	out, err := h.RequestStepUpUC.Handle(r.Context(), usecase.RequestMobileStepUpInput{
		ProjectID:              req.ProjectID,
		ActorUserID:            req.ActorUserID,
		DevicePublicID:         req.DevicePublicID,
		ActionKind:             run.MobileActionKind(req.ActionKind),
		InboxItemPublicID:      req.InboxItemPublicID,
		TargetSourceType:       req.TargetSourceType,
		TargetSourceID:         req.TargetSourceID,
		RunID:                  req.RunID,
		TraceID:                traceID,
		StepUpMethod:           run.MobileStepUpMethod(req.StepUpMethod),
		TTL:                    secondsToDuration(req.TTLSeconds),
		ProvidedChallengeValue: req.ProvidedChallengeVal,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, "stepup_request_failed", err.Error(), traceID)
		return
	}

	writeJSON(w, http.StatusOK, traceID, map[string]any{
		"challenge":             out.Challenge,
		"plain_challenge_value": out.PlainChallengeValue,
		"challenge_nonce":       out.ChallengeNonce,
		"expires_at":            out.ExpiresAt,
	})
}

func (h *V17MobileHandler) VerifyStepUp(w http.ResponseWriter, r *http.Request) {
	traceID := ensureTraceIDFromHeader(r)
	if h.VerifyStepUpUC == nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "verify stepup usecase is nil", traceID)
		return
	}

	var req verifyStepUpRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), traceID)
		return
	}

	out, err := h.VerifyStepUpUC.Handle(r.Context(), usecase.VerifyMobileStepUpInput{
		ProjectID:         req.ProjectID,
		ActorUserID:       req.ActorUserID,
		DevicePublicID:    req.DevicePublicID,
		ChallengePublicID: req.ChallengePublicID,
		ActionKind:        run.MobileActionKind(req.ActionKind),
		VerificationValue: req.VerificationValue,
		TraceID:           traceID,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, "stepup_verify_failed", err.Error(), traceID)
		return
	}

	writeJSON(w, http.StatusOK, traceID, map[string]any{
		"challenge": out.Challenge,
	})
}

func (h *V17MobileHandler) ListInbox(w http.ResponseWriter, r *http.Request) {
	traceID := ensureTraceIDFromHeader(r)
	if h.ListInboxUC == nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "list inbox usecase is nil", traceID)
		return
	}

	q := r.URL.Query()
	limit := parseIntDefault(q.Get("limit"), 20)
	offset := parseIntDefault(q.Get("offset"), 0)

	out, err := h.ListInboxUC.Handle(r.Context(), usecase.ListMobileInboxInput{
		ProjectID:      q.Get("project_id"),
		AssignedUserID: q.Get("assigned_user_id"),
		ActorUserID:    q.Get("actor_user_id"),
		Status:         run.MobileInboxStatus(q.Get("status")),
		ItemKind:       run.MobileInboxItemKind(q.Get("item_kind")),
		Priority:       run.MobilePriority(q.Get("priority")),
		Severity:       run.MobileSeverity(q.Get("severity")),
		OnlyActionable: parseBoolDefault(q.Get("only_actionable"), false),
		Limit:          limit,
		Offset:         offset,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, "list_inbox_failed", err.Error(), traceID)
		return
	}

	items := out.Items
	if items == nil {
		items = []run.MobileInboxItem{}
	}

	writeJSON(w, http.StatusOK, traceID, map[string]any{
		"items": items,
		"count": len(items),
	})
}

func (h *V17MobileHandler) AckInboxItem(w http.ResponseWriter, r *http.Request) {
	traceID := ensureTraceIDFromHeader(r)
	if h.AckItemUC == nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "ack usecase is nil", traceID)
		return
	}

	inboxPublicID, ok := extractInboxPublicIDFromPath(r.URL.Path, "/v17/mobile/inbox/", "/ack")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_path", "invalid inbox path", traceID)
		return
	}

	var req ackInboxRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), traceID)
		return
	}

	out, err := h.AckItemUC.Handle(r.Context(), usecase.AckMobileInboxItemInput{
		ProjectID:         req.ProjectID,
		ActorUserID:       req.ActorUserID,
		DevicePublicID:    req.DevicePublicID,
		InboxItemPublicID: inboxPublicID,
		StepUpChallengeID: req.StepUpChallengeID,
		IdempotencyKey:    req.IdempotencyKey,
		CommentText:       req.CommentText,
		TraceID:           traceID,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, "ack_failed", err.Error(), traceID)
		return
	}

	writeJSON(w, http.StatusOK, traceID, map[string]any{
		"item":      out.Item,
		"receipt":   out.Receipt,
		"challenge": out.Challenge,
	})
}

func (h *V17MobileHandler) ApproveInboxItem(w http.ResponseWriter, r *http.Request) {
	traceID := ensureTraceIDFromHeader(r)
	if h.ApproveItemUC == nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "approve usecase is nil", traceID)
		return
	}

	inboxPublicID, ok := extractInboxPublicIDFromPath(r.URL.Path, "/v17/mobile/inbox/", "/approve")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_path", "invalid inbox path", traceID)
		return
	}

	var req approveInboxRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), traceID)
		return
	}

	out, err := h.ApproveItemUC.Handle(r.Context(), usecase.ApproveMobileInboxItemInput{
		ProjectID:         req.ProjectID,
		ActorUserID:       req.ActorUserID,
		DevicePublicID:    req.DevicePublicID,
		InboxItemPublicID: inboxPublicID,
		StepUpChallengeID: req.StepUpChallengeID,
		IdempotencyKey:    req.IdempotencyKey,
		CommentText:       req.CommentText,
		TraceID:           traceID,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, "approve_failed", err.Error(), traceID)
		return
	}

	writeJSON(w, http.StatusOK, traceID, map[string]any{
		"item":      out.Item,
		"receipt":   out.Receipt,
		"challenge": out.Challenge,
	})
}

func (h *V17MobileHandler) RejectInboxItem(w http.ResponseWriter, r *http.Request) {
	traceID := ensureTraceIDFromHeader(r)
	if h.RejectItemUC == nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "reject usecase is nil", traceID)
		return
	}

	inboxPublicID, ok := extractInboxPublicIDFromPath(r.URL.Path, "/v17/mobile/inbox/", "/reject")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_path", "invalid inbox path", traceID)
		return
	}

	var req rejectInboxRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), traceID)
		return
	}

	out, err := h.RejectItemUC.Handle(r.Context(), usecase.RejectMobileInboxItemInput{
		ProjectID:         req.ProjectID,
		ActorUserID:       req.ActorUserID,
		DevicePublicID:    req.DevicePublicID,
		InboxItemPublicID: inboxPublicID,
		StepUpChallengeID: req.StepUpChallengeID,
		IdempotencyKey:    req.IdempotencyKey,
		CommentText:       req.CommentText,
		TraceID:           traceID,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, "reject_failed", err.Error(), traceID)
		return
	}

	writeJSON(w, http.StatusOK, traceID, map[string]any{
		"item":      out.Item,
		"receipt":   out.Receipt,
		"challenge": out.Challenge,
	})
}

func decodeJSON(r *http.Request, v any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

func writeJSON(w http.ResponseWriter, status int, traceID string, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Trace-Id", traceID)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, typ, msg, traceID string) {
	var env errorEnvelope
	env.Error.Type = typ
	env.Error.Message = msg
	env.Error.TraceID = traceID
	writeJSON(w, status, traceID, env)
}

func ensureTraceIDFromHeader(r *http.Request) string {
	v := strings.TrimSpace(r.Header.Get("X-Trace-Id"))
	if v != "" {
		return v
	}
	return usecaseTraceIDFallback(r.Context())
}

func usecaseTraceIDFallback(_ context.Context) string {
	return "trace_http_" + strconv.FormatInt(time.Now().UTC().UnixNano(), 10)
}

func secondsToDuration(v int) time.Duration {
	if v <= 0 {
		return 0
	}
	return time.Duration(v) * time.Second
}

func parseIntDefault(v string, d int) int {
	if strings.TrimSpace(v) == "" {
		return d
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return d
	}
	return n
}

func parseBoolDefault(v string, d bool) bool {
	if strings.TrimSpace(v) == "" {
		return d
	}
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return d
	}
}

func extractInboxPublicIDFromPath(path, prefix, suffix string) (string, bool) {
	if !strings.HasPrefix(path, prefix) || !strings.HasSuffix(path, suffix) {
		return "", false
	}
	mid := strings.TrimPrefix(path, prefix)
	mid = strings.TrimSuffix(mid, suffix)
	mid = strings.Trim(mid, "/")
	if mid == "" || strings.Contains(mid, "/") {
		return "", false
	}
	return mid, true
}
