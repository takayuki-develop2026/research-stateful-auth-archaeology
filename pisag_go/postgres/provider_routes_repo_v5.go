package postgres

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"example.com/pisag_go/run"
)

type ProviderRoutesRepoV5 struct{ db *sql.DB }

func NewProviderRoutesRepoV5(db *sql.DB) *ProviderRoutesRepoV5 { return &ProviderRoutesRepoV5{db: db} }

func (r *ProviderRoutesRepoV5) ListCandidates(ctx context.Context, projectID, region, currency, paymentMethod string) ([]run.ProviderRouteRow, error) {
	projectID = strings.TrimSpace(projectID)
	region = strings.TrimSpace(region)
	currency = strings.TrimSpace(currency)
	paymentMethod = strings.TrimSpace(paymentMethod)

	if projectID == "" || region == "" || currency == "" || paymentMethod == "" {
		return nil, errors.New("project_id/region/currency/payment_method are required")
	}

	const q = `
SELECT route_id::text, project_id, provider_id::text, status, priority,
       region, currency, payment_method,
       constraints::text, weights::text, why_policy_ref
FROM public.provider_routes
WHERE project_id=$1
  AND status='active'
  AND region=$2
  AND currency=$3
  AND payment_method=$4
ORDER BY priority ASC, route_id ASC;
`
	rows, err := r.db.QueryContext(ctx, q, projectID, region, currency, paymentMethod)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]run.ProviderRouteRow, 0, 32)
	for rows.Next() {
		var rr run.ProviderRouteRow
		var cStr, wStr string
		if err := rows.Scan(
			&rr.RouteID, &rr.ProjectID, &rr.ProviderID, &rr.Status, &rr.Priority,
			&rr.Region, &rr.Currency, &rr.PaymentMethod,
			&cStr, &wStr, &rr.WhyPolicyRef,
		); err != nil {
			return nil, err
		}
		rr.Constraints = []byte(cStr)
		rr.Weights = []byte(wStr)
		out = append(out, rr)
	}
	return out, rows.Err()
}

func (r *ProviderRoutesRepoV5) GetByID(ctx context.Context, projectID, routeID string) (run.ProviderRouteRow, error) {
	projectID = strings.TrimSpace(projectID)
	routeID = strings.TrimSpace(routeID)
	if projectID == "" || routeID == "" {
		return run.ProviderRouteRow{}, errors.New("project_id and route_id are required")
	}

	const q = `
SELECT route_id::text, project_id, provider_id::text, status, priority,
       region, currency, payment_method,
       constraints::text, weights::text, why_policy_ref
FROM public.provider_routes
WHERE project_id=$1 AND route_id=$2::uuid
LIMIT 1;
`
	var rr run.ProviderRouteRow
	var cStr, wStr string
	if err := r.db.QueryRowContext(ctx, q, projectID, routeID).Scan(
		&rr.RouteID, &rr.ProjectID, &rr.ProviderID, &rr.Status, &rr.Priority,
		&rr.Region, &rr.Currency, &rr.PaymentMethod,
		&cStr, &wStr, &rr.WhyPolicyRef,
	); err != nil {
		return run.ProviderRouteRow{}, err
	}
	rr.Constraints = []byte(cStr)
	rr.Weights = []byte(wStr)
	return rr, nil
}
