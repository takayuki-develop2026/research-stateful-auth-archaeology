package worker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"example.com/pisag_go/postgres"
	"example.com/pisag_go/run"
)

// ManifestBuilder builds EvidenceManifest + Links as SoT.
// v4.5: "ListLinks from DB and hash that canonical set" is the professional approach.
type ManifestBuilder struct {
	ManifestRepo *postgres.EvidenceManifestRepository
}

// NewManifestBuilder creates a builder.
func NewManifestBuilder(repo *postgres.EvidenceManifestRepository) *ManifestBuilder {
	return &ManifestBuilder{ManifestRepo: repo}
}

// FetchMeta is the minimal canonical metadata we want to capture for audit.
// This is stored as fetch_meta evidence (JSON) and linked in the manifest.
type FetchMeta struct {
	Kind          string `json:"kind"` // always "fetch_meta"
	TargetURL     string `json:"target_url"`
	FinalURL      string `json:"final_url"`
	StatusCode    int    `json:"status_code"`
	ContentType   string `json:"content_type"`
	BodyBytes     int    `json:"body_bytes"`
	BodySHA256    string `json:"body_sha256"`
	StoredBodyRel string `json:"stored_body_rel"`

	FetchedAtUTC string `json:"fetched_at_utc"`
}

// BuildAndComplete ensures a manifest exists for the run, appends links idempotently,
// then reads links back from DB to compute canonical hash, and marks the manifest complete.
//
// Inputs are the assets you already stored (body/headers/meta). For v4.5 minimal:
// - body is required
// - headers/meta can be optional (but recommended).
func (b *ManifestBuilder) BuildAndComplete(
	ctx context.Context,
	runID string,
	traceID string,
	assets []run.EvidenceAsset,
) (manifest run.EvidenceManifest, manifestHash string, err error) {
	if b == nil || b.ManifestRepo == nil {
		return run.EvidenceManifest{}, "", errors.New("manifest repo is required")
	}
	if strings.TrimSpace(runID) == "" {
		return run.EvidenceManifest{}, "", errors.New("run_id is required")
	}
	if strings.TrimSpace(traceID) == "" {
		return run.EvidenceManifest{}, "", errors.New("trace_id is required")
	}
	if len(assets) == 0 {
		return run.EvidenceManifest{}, "", errors.New("assets are required")
	}

	// 1) create/get manifest (building or complete)
	m, err := b.ManifestRepo.CreateOrGetBuilding(ctx, runID, traceID)
	if err != nil {
		return run.EvidenceManifest{}, "", err
	}

	// If already complete, return existing hash (idempotent).
	if m.Status == "complete" && m.ManifestHash != nil && *m.ManifestHash != "" {
		return m, *m.ManifestHash, nil
	}

	// 2) append links idempotently
	for _, a := range assets {
		if strings.TrimSpace(a.RunID) == "" {
			a.RunID = runID
		}
		if strings.TrimSpace(a.TraceID) == "" {
			a.TraceID = traceID
		}
		if strings.TrimSpace(a.Kind) == "" {
			return run.EvidenceManifest{}, "", fmt.Errorf("asset.kind is required")
		}
		if strings.TrimSpace(a.SHA256) == "" {
			return run.EvidenceManifest{}, "", fmt.Errorf("asset.sha256 is required")
		}
		if a.ByteSize <= 0 {
			return run.EvidenceManifest{}, "", fmt.Errorf("asset.byte_size is required")
		}
		if strings.TrimSpace(a.FinalURL) == "" {
			return run.EvidenceManifest{}, "", fmt.Errorf("asset.final_url is required")
		}
		if strings.TrimSpace(a.StoredPath) == "" {
			return run.EvidenceManifest{}, "", fmt.Errorf("asset.stored_path is required")
		}

		link := run.EvidenceLink{
			ManifestID:  m.ManifestID,
			Kind:        a.Kind,
			AssetSHA256: a.SHA256,
			ContentType: a.ContentType,
			ByteSize:    a.ByteSize,
			FinalURL:    a.FinalURL,
			StoredPath:  a.StoredPath,
		}
		if err := b.ManifestRepo.AppendLink(ctx, link); err != nil {
			return run.EvidenceManifest{}, "", err
		}
	}

	// 3) read links back from DB (single source of truth)
	links, err := b.ManifestRepo.ListLinks(ctx, m.ManifestID)
	if err != nil {
		return run.EvidenceManifest{}, "", err
	}
	if len(links) == 0 {
		return run.EvidenceManifest{}, "", errors.New("no links found for manifest (unexpected)")
	}

	// 4) canonical hash of links set (stable order, stable serialization)
	h := hashLinksCanonical(links)

	// 5) mark complete (idempotent)
	if err := b.ManifestRepo.MarkComplete(ctx, m.ManifestID, h); err != nil {
		return run.EvidenceManifest{}, "", err
	}

	// refresh manifest (optional): we can return m with ManifestHash set
	mh := h
	m.Status = "complete"
	m.ManifestHash = &mh
	m.UpdatedAt = time.Now().UTC()

	return m, h, nil
}

// hashLinksCanonical produces sha256 hex over canonical JSON of links.
// This must be stable across languages/runs.
// Canonicalization rules:
// - sort by (kind, asset_sha256, stored_path, final_url) to be extra stable
// - include only fields that define evidence identity and audit payload
func hashLinksCanonical(links []run.EvidenceLink) string {
	type canon struct {
		Kind        string  `json:"kind"`
		SHA256      string  `json:"sha256"`
		ContentType *string `json:"content_type,omitempty"`
		ByteSize    int     `json:"byte_size"`
		FinalURL    string  `json:"final_url"`
		StoredPath  string  `json:"stored_path"`
	}

	// copy + sort defensively
	cp := make([]run.EvidenceLink, len(links))
	copy(cp, links)

	sort.Slice(cp, func(i, j int) bool {
		if cp[i].Kind != cp[j].Kind {
			return cp[i].Kind < cp[j].Kind
		}
		if cp[i].AssetSHA256 != cp[j].AssetSHA256 {
			return cp[i].AssetSHA256 < cp[j].AssetSHA256
		}
		if cp[i].StoredPath != cp[j].StoredPath {
			return cp[i].StoredPath < cp[j].StoredPath
		}
		return cp[i].FinalURL < cp[j].FinalURL
	})

	out := make([]canon, 0, len(cp))
	for _, l := range cp {
		out = append(out, canon{
			Kind:        l.Kind,
			SHA256:      l.AssetSHA256,
			ContentType: l.ContentType,
			ByteSize:    l.ByteSize,
			FinalURL:    l.FinalURL,
			StoredPath:  l.StoredPath,
		})
	}

	// JSON is stable because:
	// - struct field order is stable
	// - slice order is stable after sort
	b, _ := json.Marshal(out)

	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
