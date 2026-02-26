package usecase

import (
	"context"
	"errors"
	"strings"

	"example.com/pisag_go/postgres"
)

type V13CompatContractInsertInput struct {
	ProjectID string
	ContractType string
	ContractVersion string
	ChecksumSha256 string

	ArtifactRef *string
	DiffSummary *string
	DetailEvidenceAssetID *int64
}

type V13CompatContractUseCase struct {
	V13Repo *postgres.V13Repository
}

func (uc *V13CompatContractUseCase) Insert(ctx context.Context, in V13CompatContractInsertInput) (int64, error) {
	if strings.TrimSpace(in.ProjectID) == "" {
		return 0, errors.New("project_id is required")
	}
	if strings.TrimSpace(in.ContractType) == "" {
		return 0, errors.New("contract_type is required")
	}
	if strings.TrimSpace(in.ContractVersion) == "" {
		return 0, errors.New("contract_version is required")
	}
	if strings.TrimSpace(in.ChecksumSha256) == "" {
		return 0, errors.New("checksum_sha256 is required")
	}
	return uc.V13Repo.CompatContractInsert(ctx,
		strings.TrimSpace(in.ProjectID),
		strings.TrimSpace(in.ContractType),
		strings.TrimSpace(in.ContractVersion),
		strings.TrimSpace(in.ChecksumSha256),
		in.ArtifactRef, in.DiffSummary, in.DetailEvidenceAssetID,
	)
}