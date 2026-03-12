package usecase

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	run "example.com/pisag_go/run"
)

type RegisterOCRResultEvidencePort interface {
	RegisterJSONEvidence(ctx context.Context, in run.RegisterRuntimeJSONEvidenceInput) (run.RegisterRuntimeJSONEvidenceOutput, error)
}

type RegisterOCRResultEvidenceUseCase struct {
	Evidence RegisterOCRResultEvidencePort
}

type RegisterOCRResultEvidenceInput struct {
	ProjectID string
	TraceID   string

	// 実OCR本文全文
	FullText string

	// UI/監査向けの短い要約
	SummaryText string

	ConfidenceScore *float64
	Blocks          []map[string]any
	Metadata        map[string]any
}

type RegisterOCRResultEvidenceOutput struct {
	TextEvidenceAssetID       int64
	ConfidenceEvidenceAssetID int64
	BlocksEvidenceAssetID     *int64
}

func (uc *RegisterOCRResultEvidenceUseCase) Handle(ctx context.Context, in RegisterOCRResultEvidenceInput) (RegisterOCRResultEvidenceOutput, error) {
	if uc.Evidence == nil {
		return RegisterOCRResultEvidenceOutput{}, fmt.Errorf("register ocr result evidence: evidence port is nil")
	}
	if strings.TrimSpace(in.ProjectID) == "" {
		return RegisterOCRResultEvidenceOutput{}, fmt.Errorf("register ocr result evidence: project_id is required")
	}
	if strings.TrimSpace(in.TraceID) == "" {
		return RegisterOCRResultEvidenceOutput{}, fmt.Errorf("register ocr result evidence: trace_id is required")
	}

	fullText := strings.TrimSpace(in.FullText)
	summaryText := strings.TrimSpace(in.SummaryText)

	// FullText が空なら SummaryText をフォールバックに使う
	if fullText == "" {
		fullText = summaryText
	}

	textPayload := map[string]any{
		"text":         fullText,
		"summary_text": summaryText,
		"text_length":  len(fullText),
	}
	textJSON, err := json.Marshal(textPayload)
	if err != nil {
		return RegisterOCRResultEvidenceOutput{}, fmt.Errorf("register ocr result evidence text marshal: %w", err)
	}
	textSHA := sha256HexBytes(textJSON)

	textOut, err := uc.Evidence.RegisterJSONEvidence(ctx, run.RegisterRuntimeJSONEvidenceInput{
		ProjectID:   in.ProjectID,
		TraceID:     in.TraceID,
		Kind:        "ocr_text_payload",
		BodyJSON:    string(textJSON),
		SHA256:      textSHA,
		Description: "OCR extracted full text payload",
	})
	if err != nil {
		return RegisterOCRResultEvidenceOutput{}, fmt.Errorf("register ocr result evidence text: %w", err)
	}

	confPayload := map[string]any{
		"confidence_score": in.ConfidenceScore,
		"summary_text":     summaryText,
		"metadata":         defaultAnyMap(in.Metadata),
	}
	confJSON, err := json.Marshal(confPayload)
	if err != nil {
		return RegisterOCRResultEvidenceOutput{}, fmt.Errorf("register ocr result evidence confidence marshal: %w", err)
	}
	confSHA := sha256HexBytes(confJSON)

	confOut, err := uc.Evidence.RegisterJSONEvidence(ctx, run.RegisterRuntimeJSONEvidenceInput{
		ProjectID:   in.ProjectID,
		TraceID:     in.TraceID,
		Kind:        "ocr_confidence_payload",
		BodyJSON:    string(confJSON),
		SHA256:      confSHA,
		Description: "OCR confidence payload",
	})
	if err != nil {
		return RegisterOCRResultEvidenceOutput{}, fmt.Errorf("register ocr result evidence confidence: %w", err)
	}

	var blocksID *int64
	if len(in.Blocks) > 0 {
		blocksPayload := map[string]any{
			"blocks":       in.Blocks,
			"summary_text": summaryText,
			"text_length":  len(fullText),
		}
		blocksJSON, err := json.Marshal(blocksPayload)
		if err != nil {
			return RegisterOCRResultEvidenceOutput{}, fmt.Errorf("register ocr result evidence blocks marshal: %w", err)
		}
		blocksSHA := sha256HexBytes(blocksJSON)

		blocksOut, err := uc.Evidence.RegisterJSONEvidence(ctx, run.RegisterRuntimeJSONEvidenceInput{
			ProjectID:   in.ProjectID,
			TraceID:     in.TraceID,
			Kind:        "ocr_blocks_payload",
			BodyJSON:    string(blocksJSON),
			SHA256:      blocksSHA,
			Description: "OCR blocks payload",
		})
		if err != nil {
			return RegisterOCRResultEvidenceOutput{}, fmt.Errorf("register ocr result evidence blocks: %w", err)
		}
		blocksID = &blocksOut.EvidenceAssetID
	}

	return RegisterOCRResultEvidenceOutput{
		TextEvidenceAssetID:       textOut.EvidenceAssetID,
		ConfidenceEvidenceAssetID: confOut.EvidenceAssetID,
		BlocksEvidenceAssetID:     blocksID,
	}, nil
}

func sha256HexBytes(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func defaultAnyMap(in map[string]any) map[string]any {
	if in == nil {
		return map[string]any{}
	}
	return in
}