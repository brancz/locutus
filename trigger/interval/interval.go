package interval

import (
	"context"
	"time"

	"github.com/brancz/locutus/trigger"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
)

type Trigger struct {
	trigger.ExecutionRegister

	logger   log.Logger
	interval time.Duration
}

func NewTrigger(logger log.Logger, interval time.Duration) *Trigger {
	return &Trigger{logger: logger, interval: interval}
}

func (t *Trigger) Run(ctx context.Context) error {
	ticker := time.NewTicker(t.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			level.Debug(t.logger).Log("msg", "interval triggered")
			if err := t.Execute(ctx, nil); err != nil {
				level.Error(t.logger).Log("msg", "execution failed", "err", err)
			}
		}
	}
}
