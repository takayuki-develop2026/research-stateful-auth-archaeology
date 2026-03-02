package httpx

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
)

const TraceHeader = "X-Trace-Id"

type ErrorEnvelope struct {
	Error struct {
		Type    string `json:"type"`
		Message string `json:"message"`
		TraceID string `json:"trace_id"`
	} `json:"error"`
}

func NewTraceID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func SanitizeTraceID(v string) string {
	v = strings.TrimSpace(v)
	if v == "" || len(v) > 128 {
		return ""
	}
	for _, r := range v {
		if (r >= 'a' && r <= 'z') ||
			(r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') ||
			r == '-' || r == '_' || r == '.' || r == ':' {
			continue
		}
		return ""
	}
	return v
}

func WriteJSON(w http.ResponseWriter, status int, v any, traceID string) {
	w.Header().Set("Content-Type", "application/json")
	if traceID != "" {
		w.Header().Set(TraceHeader, traceID)
	}
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func WriteError(w http.ResponseWriter, status int, typ, msg, traceID string) {
	var env ErrorEnvelope
	env.Error.Type = typ
	env.Error.Message = msg
	env.Error.TraceID = traceID
	WriteJSON(w, status, env, traceID)
}