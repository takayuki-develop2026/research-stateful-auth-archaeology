package usecase

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"example.com/pisag_go/postgres"
)

type V13IdempotencyStartInput struct {
	ProjectID string
	Scope     string
	Key       string

	// any stable string you want to hash (request body, params, etc.)
	RequestCanonical string
}

type V13IdempotencyStartOutput struct {
	IdempotencyID int64
	FoundExisting bool
	RequestHash   string // 64 hex
}

type V13IdempotencyUseCase struct {
	V13Repo *postgres.V13Repository
}

func (uc *V13IdempotencyUseCase) Start(ctx context.Context, in V13IdempotencyStartInput) (V13IdempotencyStartOutput, error) {
	if strings.TrimSpace(in.ProjectID) == "" {
		return V13IdempotencyStartOutput{}, errors.New("project_id is required")
	}
	if strings.TrimSpace(in.Scope) == "" {
		return V13IdempotencyStartOutput{}, errors.New("scope is required")
	}
	if strings.TrimSpace(in.Key) == "" {
		return V13IdempotencyStartOutput{}, errors.New("key is required")
	}

	sum := sha256.Sum256([]byte(strings.TrimSpace(in.RequestCanonical)))
	hash := hex.EncodeToString(sum[:])

	res, err := uc.V13Repo.IdempotencyStart(ctx, strings.TrimSpace(in.ProjectID), strings.TrimSpace(in.Scope), strings.TrimSpace(in.Key), hash)
	if err != nil {
		return V13IdempotencyStartOutput{}, err
	}
	return V13IdempotencyStartOutput{
		IdempotencyID: res.IdempotencyID,
		FoundExisting: res.FoundExisting,
		RequestHash:   hash,
	}, nil
}

type V13IdempotencyFinishInput struct {
	ProjectID             string
	Id                    int64
	Status                string // succeeded|review_required|failed
	Summary               *string
	ResultEvidenceAssetID *int64
}

func (uc *V13IdempotencyUseCase) Finish(ctx context.Context, in V13IdempotencyFinishInput) error {
	if strings.TrimSpace(in.ProjectID) == "" {
		return errors.New("project_id is required")
	}
	if in.Id <= 0 {
		return errors.New("id is required")
	}
	if strings.TrimSpace(in.Status) == "" {
		return errors.New("status is required")
	}
	return uc.V13Repo.IdempotencyFinish(ctx, strings.TrimSpace(in.ProjectID), in.Id, strings.TrimSpace(in.Status), in.Summary, in.ResultEvidenceAssetID, time.Now().UTC())
}
