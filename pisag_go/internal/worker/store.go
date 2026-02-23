package worker

import (
	"database/sql"

	"example.com/pisag_go/postgres"
)

type Store struct {
	RunRepo      *postgres.RunRepository
	ClaimRepo    *postgres.RunInputClaimRepository
	EvidenceRepo *postgres.EvidenceRepository
}

func NewStore(db *sql.DB) *Store {
	return &Store{
		RunRepo:      postgres.NewRunRepository(db),
		ClaimRepo:    postgres.NewRunInputClaimRepository(db),
		EvidenceRepo: postgres.NewEvidenceRepository(db),
	}
}