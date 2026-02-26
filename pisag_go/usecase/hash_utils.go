package usecase

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

func shortHash(s string) string {
	h := sha256.Sum256([]byte(strings.TrimSpace(s)))
	return hex.EncodeToString(h[:])[:16]
}

func sha256Hex(s string) string {
	h := sha256.Sum256([]byte(strings.TrimSpace(s)))
	return hex.EncodeToString(h[:])
}