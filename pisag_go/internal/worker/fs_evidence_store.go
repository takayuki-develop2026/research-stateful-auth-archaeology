package worker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type EvidenceStore interface {
	SaveFetchBody(ctx context.Context, runID string, r io.Reader, maxBytes int64) (storedRelPath string, sha string, size int, err error)
}

type FSEvidenceStore struct {
	BaseDir string // e.g. "./var/evidence"
}

func NewFSEvidenceStore(baseDir string) *FSEvidenceStore {
	return &FSEvidenceStore{BaseDir: baseDir}
}

func (s *FSEvidenceStore) SaveFetchBody(ctx context.Context, runID string, r io.Reader, maxBytes int64) (string, string, int, error) {
	_ = ctx // (将来キャンセル対応するなら Reader を工夫)

	if maxBytes <= 0 {
		maxBytes = 5 << 20
	}

	// limit + hash while writing to temp
	runDir := filepath.Join(s.BaseDir, runID)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		return "", "", 0, err
	}

	tmpPath := filepath.Join(runDir, "fetch_body.tmp")
	f, err := os.Create(tmpPath)
	if err != nil {
		return "", "", 0, err
	}
	defer f.Close()

	h := sha256.New()
	lr := &io.LimitedReader{R: r, N: maxBytes + 1} // +1 overflow detect
	n, err := io.Copy(io.MultiWriter(f, h), lr)
	if err != nil {
		return "", "", 0, err
	}
	if n > maxBytes {
		_ = os.Remove(tmpPath)
		return "", "", int(n), fmt.Errorf("response too large: %d bytes (limit %d)", n, maxBytes)
	}

	sum := hex.EncodeToString(h.Sum(nil))
	finalName := fmt.Sprintf("fetch_body_%s.bin", sum)
	finalPath := filepath.Join(runDir, finalName)

	// atomic-ish rename
	if err := os.Rename(tmpPath, finalPath); err != nil {
		_ = os.Remove(tmpPath)
		return "", "", 0, err
	}

	rel := filepath.ToSlash(filepath.Join(runID, finalName))
	return rel, sum, int(n), nil
}