package usecase

import (
	"context"

	"example.com/pisag_go/run"
)

type LifecycleRunUsecase struct {
	Lifecycle run.LifecycleRepo
}

func NewLifecycleRunUsecase(r run.LifecycleRepo) *LifecycleRunUsecase {
	return &LifecycleRunUsecase{Lifecycle: r}
}

func (uc *LifecycleRunUsecase) Handle(ctx context.Context, in run.LifecycleJobRunInput) (run.LifecycleJobRunResult, error) {
	return uc.Lifecycle.RunJob(ctx, in)
}
