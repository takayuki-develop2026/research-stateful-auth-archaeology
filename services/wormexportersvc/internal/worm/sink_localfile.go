package worm

import (
	"context"
	"os"
	"path/filepath"
)

type LocalFileSink struct {
	outDir string
}

func NewLocalFileSink(outDir string) *LocalFileSink {
	return &LocalFileSink{outDir: outDir}
}

func (s *LocalFileSink) Name() string { return "localfile" }

func (s *LocalFileSink) Put(ctx context.Context, req ExportRequest) (ExportResult, error) {
	_ = ctx // localfile is sync; if you want, check ctx.Done() before heavy work

	// ensure parent dirs
	p := filepath.Join(s.outDir, filepath.FromSlash(req.ObjectKey))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return ExportResult{}, err
	}

	if err := os.WriteFile(p, req.Body, 0o644); err != nil {
		return ExportResult{}, err
	}

	return ExportResult{
		ObjectKey: req.ObjectKey,
		Bytes:     int64(len(req.Body)),
		Sink:      s.Name(),
		Sha256:    req.Sha256,
	}, nil
}