package worm

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type LocalFileSink struct {
	outDir string
}

func NewLocalFileSink(outDir string) *LocalFileSink {
	return &LocalFileSink{outDir: strings.TrimSpace(outDir)}
}

func (s *LocalFileSink) Name() string { return "localfile" }

func (s *LocalFileSink) Put(ctx context.Context, req ExportRequest) (ExportResult, error) {
	// localfile is sync; honor ctx for fast abort
	select {
	case <-ctx.Done():
		return ExportResult{}, ctx.Err()
	default:
	}

	objKey := strings.TrimSpace(req.ObjectKey)
	if objKey == "" {
		return ExportResult{}, fmt.Errorf("object_key required")
	}

	// Normalize: ObjectKey is logical path (slash-separated). Convert for OS.
	rel := filepath.Clean(filepath.FromSlash(objKey))
	rel = strings.TrimLeft(rel, string(os.PathSeparator)) // prevent absolute
	if rel == "." || rel == "" {
		return ExportResult{}, fmt.Errorf("invalid object_key: %q", objKey)
	}
	// Block traversal
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return ExportResult{}, fmt.Errorf("path traversal blocked: %q", objKey)
	}

	outDir := s.outDir
	if outDir == "" {
		outDir = "/var/wormexporter/out"
	}

	// Absolute full path
	full := filepath.Join(outDir, rel)

	// Ensure still under outDir (defense-in-depth)
	outAbs, err := filepath.Abs(outDir)
	if err != nil {
		return ExportResult{}, fmt.Errorf("abs(outDir) failed: %w", err)
	}
	fullAbs, err := filepath.Abs(full)
	if err != nil {
		return ExportResult{}, fmt.Errorf("abs(full) failed: %w", err)
	}
	outAbs = outAbs + string(os.PathSeparator)
	if !strings.HasPrefix(fullAbs+string(os.PathSeparator), outAbs) {
		return ExportResult{}, fmt.Errorf("write escaped outDir: out=%s full=%s", outAbs, fullAbs)
	}

	// Ensure parent dirs
	if err := os.MkdirAll(filepath.Dir(fullAbs), 0o755); err != nil {
		return ExportResult{}, fmt.Errorf("mkdir failed: %w", err)
	}

	// Atomic write: tmp -> rename
	tmp := fullAbs + ".tmp"
	if err := os.WriteFile(tmp, req.Body, 0o644); err != nil {
		return ExportResult{}, fmt.Errorf("write tmp failed: %w", err)
	}
	if err := os.Rename(tmp, fullAbs); err != nil {
		_ = os.Remove(tmp)
		return ExportResult{}, fmt.Errorf("rename failed: %w", err)
	}

	return ExportResult{
		ObjectKey: objKey,              // logical key
		Bytes:     int64(len(req.Body)), // bytes of JSON only
		Sink:      s.Name(),
		Sha256:    req.Sha256, // returned for logging/DB, NOT written to file
	}, nil
}