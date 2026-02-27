package httpx

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

func DecodeJSON(r *http.Request, dst any) error {
	raw, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		return err
	}
	_ = r.Body.Close()

	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return fmt.Errorf("decode json: %w", err)
	}
	return nil
}

func LogPGErr(traceID, label string, err error) {
	if err == nil {
		return
	}
	var pe *pgconn.PgError
	if errors.As(err, &pe) {
		log.Printf("[DB][trace=%s] %s: %s (code=%s) detail=%s where=%s constraint=%s table=%s",
			traceID, label, pe.Message, pe.Code, pe.Detail, pe.Where, pe.ConstraintName, pe.TableName)
		return
	}
	log.Printf("[DB][trace=%s] %s: %v", traceID, label, err)
}