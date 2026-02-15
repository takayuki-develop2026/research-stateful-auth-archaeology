package worker

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	PolicyAllowAll  = "allow_all"
	PolicyDenyAll   = "deny_all"
	PolicyAllowList = "allow_list"

	// RunArtifacts contract (DDL-aligned)
	// CHECK (schema_version = '1.0') が勝つので、必ず "1.0"
	RunArtifactSchemaVersion = "1.0"
)

func NormalizeRunID(s string) string {
	return strings.TrimSpace(s)
}

func IsValidRunID(s string) bool {
	s = strings.TrimSpace(s)
	return len(s) == 26
}

func NowRFC3339Nano() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}

func NormalizePolicy(s string) string {
	return strings.TrimSpace(strings.ToLower(s))
}

func MarshalJSONOrEmptyMap(payload map[string]any) string {
	if payload == nil {
		return "{}"
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return "{}"
	}
	return string(b)
}

func MustEnv(k string) string {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		log.Fatalf("%s is required", k)
	}
	return v
}

func GetenvDefault(k, def string) string {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		return def
	}
	return v
}

func GetenvDurationMS(k string, def time.Duration) time.Duration {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return def
	}
	return time.Duration(n) * time.Millisecond
}

func GetenvInt64(k string, def int64) int64 {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		return def
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n <= 0 {
		return def
	}
	return n
}

func HostnameFallback() string {
	h, _ := os.Hostname()
	if strings.TrimSpace(h) == "" {
		return "worker-unknown"
	}
	return h
}

func NewTraceID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		now := time.Now().UTC().UnixNano()
		return fmt.Sprintf("trace-%d", now)
	}
	return hex.EncodeToString(b)
}

func ContainsInt(xs []int, v int) bool {
	for _, x := range xs {
		if x == v {
			return true
		}
	}
	return false
}

func ContainsStr(xs []string, v string) bool {
	for _, x := range xs {
		if x == v {
			return true
		}
	}
	return false
}