package postgres

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"example.com/pisag_go/run"
)

type ProvidersRepoV5 struct{ db *sql.DB }

func NewProvidersRepoV5(db *sql.DB) *ProvidersRepoV5 { return &ProvidersRepoV5{db: db} }

func (r *ProvidersRepoV5) ListActive(ctx context.Context, projectID string) ([]run.ProviderRow, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return nil, errors.New("project_id is required")
	}

	const q = `
SELECT provider_id::text, project_id, provider_key, status
FROM public.providers
WHERE project_id=$1 AND status <> 'blocked'
ORDER BY provider_key ASC;
`
	rows, err := r.db.QueryContext(ctx, q, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]run.ProviderRow, 0, 16)
	for rows.Next() {
		var p run.ProviderRow
		if err := rows.Scan(&p.ProviderID, &p.ProjectID, &p.ProviderKey, &p.Status); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *ProvidersRepoV5) GetByID(ctx context.Context, projectID, providerID string) (run.ProviderRow, error) {
	projectID = strings.TrimSpace(projectID)
	providerID = strings.TrimSpace(providerID)
	if projectID == "" || providerID == "" {
		return run.ProviderRow{}, errors.New("project_id and provider_id are required")
	}

	const q = `
SELECT provider_id::text, project_id, provider_key, status
FROM public.providers
WHERE project_id=$1 AND provider_id=$2::uuid
LIMIT 1;
`
	var p run.ProviderRow
	if err := r.db.QueryRowContext(ctx, q, projectID, providerID).Scan(&p.ProviderID, &p.ProjectID, &p.ProviderKey, &p.Status); err != nil {
		return run.ProviderRow{}, err
	}
	return p, nil
}