package scheduler

import (
	"context"
	"fmt"
	"time"

	"github.com/robfig/cron/v3"

	"runschedsvc/postgres"
)

type TickConfig struct {
	ProjectID        string
	LimitSchedules   int
	CronPreviewLimit int
}

type Ticker struct {
	repo *postgres.RunSchedRepoV19
	cfg  TickConfig
}

func NewTicker(repo *postgres.RunSchedRepoV19, cfg TickConfig) *Ticker {
	if cfg.LimitSchedules <= 0 {
		cfg.LimitSchedules = 50
	}
	if cfg.CronPreviewLimit <= 0 {
		cfg.CronPreviewLimit = 200
	}
	return &Ticker{repo: repo, cfg: cfg}
}

// Build next_map for cron schedules due now (best-effort).
// Actual schedule claiming is done inside runsched_tick_enqueue_v19 with SKIP LOCKED.
func (t *Ticker) buildCronNextMap(ctx context.Context, nowUTC time.Time) (map[string]string, error) {
	due, err := t.repo.ListDueCronSchedules(ctx, t.cfg.ProjectID, nowUTC, t.cfg.CronPreviewLimit)
	if err != nil {
		return nil, err
	}

	// 5-field cron: minute hour dom month dow
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)

	nextMap := make(map[string]string, len(due))
	for _, d := range due {
		spec, err := parser.Parse(d.CronExpr)
		if err != nil {
			// don't throw; just skip mapping -> DB fallback moves +1m
			continue
		}
		loc, err := time.LoadLocation(d.TimeZone)
		if err != nil {
			loc = time.UTC
		}
		// compute next in schedule tz, then normalize to UTC
		nowInTZ := nowUTC.In(loc)
		n := spec.Next(nowInTZ).In(time.UTC)
		nextMap[fmt.Sprintf("%d", d.ScheduleID)] = n.Format(time.RFC3339)
	}
	return nextMap, nil
}

func (t *Ticker) TickOnce(ctx context.Context) (int, error) {
	nowUTC := time.Now().UTC()

	nextMap, err := t.buildCronNextMap(ctx, nowUTC)
	if err != nil {
		return 0, err
	}

	rows, err := t.repo.TickEnqueue(ctx, t.cfg.ProjectID, nowUTC, t.cfg.LimitSchedules, nextMap)
	if err != nil {
		return 0, err
	}
	return len(rows), nil
}