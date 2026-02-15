package httpx

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
)

const TraceHeader = "X-Trace-Id"

type ctxKey string

const traceCtxKey ctxKey = "trace_id"

type ErrorEnvelope struct {
	Error struct {
		Type    string `json:"type"`
		Message string `json:"message"`
		TraceID string `json:"trace_id"`
	} `json:"error"`
}

func NewTraceID() string {
	// 128-bit random hex (32 chars). Simple and header-friendly.
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func SanitizeTraceID(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	// allow [a-zA-Z0-9-_.:] up to 128 chars.
	if len(v) > 128 {
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

func TraceMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		tid := SanitizeTraceID(req.Header.Get(TraceHeader))
		if tid == "" {
			tid = NewTraceID()
		}
		ctx := context.WithValue(req.Context(), traceCtxKey, tid)
		next.ServeHTTP(w, req.WithContext(ctx))
	})
}

func TraceIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(traceCtxKey).(string); ok && v != "" {
		return v
	}
	return ""
}

func WriteJSON(w http.ResponseWriter, status int, body any, traceID string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if traceID != "" {
		w.Header().Set(TraceHeader, traceID)
	}
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func WriteError(w http.ResponseWriter, status int, typ, msg, traceID string) {
	var env ErrorEnvelope
	env.Error.Type = typ
	env.Error.Message = msg
	env.Error.TraceID = traceID
	WriteJSON(w, status, env, traceID)
}

func NullableText(s string) any {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}

func ReadIdempotencyKey(req *http.Request) string {
	requestKey := strings.TrimSpace(req.Header.Get("Idempotency-Key"))
	if requestKey == "" {
		requestKey = strings.TrimSpace(req.Header.Get("X-Idempotency-Key"))
	}
	if len(requestKey) > 256 {
		return ""
	}
	return requestKey
}

func logDBErr(traceID, label string, err error) {
	if err == nil {
		return
	}
	log.Printf("[DB][trace=%s] %s: %v", traceID, label, err)
}

func logPGErr(traceID, label string, err error) {
	if err == nil {
		return
	}
	var pe *pgconn.PgError
	if errors.As(err, &pe) {
		log.Printf(
			"[DB][trace=%s] %s: %s (code=%s) detail=%s where=%s constraint=%s schema=%s table=%s column=%s",
			traceID, label,
			pe.Message, pe.Code, pe.Detail, pe.Where,
			pe.ConstraintName, pe.SchemaName, pe.TableName, pe.ColumnName,
		)
		return
	}
	logDBErr(traceID, label, err)
}
