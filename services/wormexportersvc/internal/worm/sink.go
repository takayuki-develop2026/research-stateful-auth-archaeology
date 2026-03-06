package worm

import "context"

// ExportRequest is the sink write input.
type ExportRequest struct {
	ObjectKey   string
	Body        []byte
	ContentType string
	Sha256      string
}

// ExportResult is the sink write output metadata.
// This is runtime output only (NOT DB SoT).
type ExportResult struct {
	ObjectKey string
	Bytes     int64
	Sink      string
	Sha256    string
}

type Sink interface {
	Put(ctx context.Context, req ExportRequest) (ExportResult, error)
	Name() string
}