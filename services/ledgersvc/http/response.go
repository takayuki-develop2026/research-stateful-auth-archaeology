package httpapi

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

func GetOrCreateTraceID(r *http.Request) string {
	v := strings.TrimSpace(r.Header.Get(TraceHeader))
	if v != "" && len(v) <= 128 {
		return v
	}
	return NewTraceID()
}

func WriteJSON(w http.ResponseWriter, traceID string, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set(TraceHeader, traceID)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func WriteError(w http.ResponseWriter, traceID string, status int, typ, msg string) {
	env := ErrorEnvelope{}
	env.Error.Type = typ
	env.Error.Message = msg
	env.Error.TraceID = traceID
	WriteJSON(w, traceID, status, env)
}