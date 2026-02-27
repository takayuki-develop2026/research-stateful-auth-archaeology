package postgres

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"example.com/pisag_go/run"
)

type RoutingMetricsRepoV5 struct{ db *sql.DB }

func NewRoutingMetricsRepoV5(db *sql.DB) *RoutingMetricsRepoV5 { return &RoutingMetricsRepoV5{db: db} }

func (r *RoutingMetricsRepoV5) GetLatestForRoute(ctx context.Context, projectID, routeID string) (*run.RoutingMetricSnapshot, error) {
	if projectID == "" || routeID == "" {
		return nil, errors.New("project_id and route_id are required")
	}

	const q = `
SELECT metric_date, success_rate, p95_latency_ms, avg_cost_minor, sample_n
FROM public.routing_metrics_daily
WHERE project_id=$1 AND route_id=$2::uuid
ORDER BY metric_date DESC
LIMIT 1;
`
	var m run.RoutingMetricSnapshot
	var metricDate time.Time
	err := r.db.QueryRowContext(ctx, q, projectID, routeID).Scan(
		&metricDate, &m.SuccessRate, &m.P95LatencyMs, &m.AvgCostMinor, &m.SampleN,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	m.MetricDate = metricDate
	return &m, nil
}