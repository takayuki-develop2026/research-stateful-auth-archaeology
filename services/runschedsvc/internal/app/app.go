package app

import (
	"context"
	"log"
	"time"

	"runschedsvc/internal/dispatcher"
	"runschedsvc/internal/scheduler"
	"runschedsvc/postgres"
)

type Config struct {
	ProjectID string

	TickEvery       time.Duration
	DispatchEvery   time.Duration
	TickLimit       int
	DispatchLimit   int
	CronPreviewLimit int
}

type App struct {
	ticker     *scheduler.Ticker
	dispatcher *dispatcher.Dispatcher
	cfg        Config
}

func New(db *postgres.DB, cfg Config) *App {
	repo := postgres.NewRunSchedRepoV19(db)

	ticker := scheduler.NewTicker(repo, scheduler.TickConfig{
		ProjectID:        cfg.ProjectID,
		LimitSchedules:   cfg.TickLimit,
		CronPreviewLimit: cfg.CronPreviewLimit,
	})

	disp := dispatcher.New(repo, dispatcher.DispatchConfig{
	ProjectID: cfg.ProjectID,
	LimitRuns: cfg.DispatchLimit,
	ActorType: "system",
	ActorID:   "runschedsvc",
})

	if cfg.TickEvery <= 0 {
		cfg.TickEvery = 60 * time.Second
	}
	if cfg.DispatchEvery <= 0 {
		cfg.DispatchEvery = 2 * time.Second
	}
	return &App{ticker: ticker, dispatcher: disp, cfg: cfg}
}

func (a *App) Run(ctx context.Context) error {
	tickT := time.NewTicker(a.cfg.TickEvery)
	defer tickT.Stop()

	dispatchT := time.NewTicker(a.cfg.DispatchEvery)
	defer dispatchT.Stop()

	log.Printf("[runschedsvc] start project_id=%s tick=%s dispatch=%s", a.cfg.ProjectID, a.cfg.TickEvery, a.cfg.DispatchEvery)

	for {
		select {
		case <-ctx.Done():
			log.Printf("[runschedsvc] stop: %v", ctx.Err())
			return ctx.Err()

		case <-tickT.C:
			n, err := a.ticker.TickOnce(ctx)
			if err != nil {
				log.Printf("[runschedsvc][tick] error: %v", err)
				continue
			}
			if n > 0 {
				log.Printf("[runschedsvc][tick] enqueued=%d", n)
			}

		case <-dispatchT.C:
			created, skipped, err := a.dispatcher.DispatchOnce(ctx)
			if err != nil {
				log.Printf("[runschedsvc][dispatch] error: %v", err)
				continue
			}
			if created > 0 || skipped > 0 {
				log.Printf("[runschedsvc][dispatch] created=%d skipped=%d", created, skipped)
			}
		}
	}
}