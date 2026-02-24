package worker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type EvidenceStore interface {
	SaveFetchBody(ctx context.Context, runID string, r io.Reader, maxBytes int64) (storedRelPath string, sha string, size int, err error)

	// v4.5: generic blob saver (body/meta/headers)
	SaveBlob(ctx context.Context, runID string, kind string, ext string, r io.Reader, maxBytes int64) (storedRelPath string, sha string, size int, err error)
}

type FSEvidenceStore struct {
	BaseDir string // e.g. "./var/evidence"
}

func NewFSEvidenceStore(baseDir string) *FSEvidenceStore {
	return &FSEvidenceStore{BaseDir: baseDir}
}

func (s *FSEvidenceStore) SaveFetchBody(ctx context.Context, runID string, r io.Reader, maxBytes int64) (string, string, int, error) {
	return s.SaveBlob(ctx, runID, "fetch_body", "bin", r, maxBytes)
}

// SaveBlob stores a blob under {BaseDir}/{runID}/{kind}_{sha}.{ext}
// - Computes sha256 while writing
// - Enforces maxBytes (reads at most maxBytes+1 to detect overflow)
// - Uses atomic-ish rename from temp to final name
func (s *FSEvidenceStore) SaveBlob(ctx context.Context, runID string, kind string, ext string, r io.Reader, maxBytes int64) (string, string, int, error) {
	_ = ctx // (将来キャンセル対応するなら Reader を工夫)

	runID = strings.TrimSpace(runID)
	if runID == "" {
		return "", "", 0, fmt.Errorf("runID is required")
	}
	kind = strings.TrimSpace(kind)
	if kind == "" {
		return "", "", 0, fmt.Errorf("kind is required")
	}
	ext = strings.TrimSpace(ext)
	if ext == "" {
		ext = "bin"
	}
	if maxBytes <= 0 {
		maxBytes = 5 << 20
	}

	runDir := filepath.Join(s.BaseDir, runID)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		return "", "", 0, err
	}

	tmpPath := filepath.Join(runDir, fmt.Sprintf("%s.tmp", kind))
	f, err := os.Create(tmpPath)
	if err != nil {
		return "", "", 0, err
	}
	defer f.Close()

	h := sha256.New()
	lr := &io.LimitedReader{R: r, N: maxBytes + 1} // +1 overflow detect
	n64, err := io.Copy(io.MultiWriter(f, h), lr)
	if err != nil {
		_ = os.Remove(tmpPath)
		return "", "", 0, err
	}
	if n64 > maxBytes {
		_ = os.Remove(tmpPath)
		return "", "", int(n64), fmt.Errorf("response too large: %d bytes (limit %d)", n64, maxBytes)
	}

	sum := hex.EncodeToString(h.Sum(nil))
	finalName := fmt.Sprintf("%s_%s.%s", kind, sum, ext)
	finalPath := filepath.Join(runDir, finalName)

	if err := os.Rename(tmpPath, finalPath); err != nil {
		_ = os.Remove(tmpPath)
		return "", "", 0, err
	}

	rel := filepath.ToSlash(filepath.Join(runID, finalName))
	return rel, sum, int(n64), nil
}