package interval

import (
	"context"
	"time"

	"github.com/brancz/locutus/trigger"
	"github.com/go-kit/kit/log"
)

type Trigger struct {
	trigger.ExecutionRegister

	Logger   log.Logger
	Interval time.Duration
}

func NewTrigger(logger log.Logger, interval time.Duration) *Trigger {
	return &Trigger{Logger: logger, Interval: interval}
}

func (t *Trigger) Run(ctx context.Context) error {
	ticker := time.NewTicker(t.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			t.Logger.Log("msg", "interval triggered")
			t.Execute(ctx, nil)
		}
	}
}
