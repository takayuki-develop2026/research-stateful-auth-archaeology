package postgres

import (
	"context"
	"database/sql"
)

type RunTraceRepository struct{ db *sql.DB }

func NewRunTraceRepository(db *sql.DB) *RunTraceRepository { return &RunTraceRepository{db: db} }

func (r *RunTraceRepository) GetTraceID(ctx context.Context, runID string) (string, error) {
	var traceID string
	err := r.db.QueryRowContext(ctx, `SELECT trace_id FROM runs WHERE run_id=$1`, runID).Scan(&traceID)
	return traceID, err
}