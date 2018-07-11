package interval

import (
	"flag"
	"time"

	"github.com/go-kit/kit/log"

	"github.com/brancz/locutus/client"
	"github.com/brancz/locutus/trigger/types"
)

type IntervalProvider struct {
	interval time.Duration
}

func (p *IntervalProvider) RegisterFlags(s *flag.FlagSet) {
	s.DurationVar(&p.interval, "trigger.interval.duration", time.Minute, "Duration of interval in which to trigger.")
}

func (p *IntervalProvider) Name() string {
	return "interval"
}

type IntervalTrigger struct {
	types.ExecutionRegister

	logger log.Logger

	interval time.Duration
}

func (p *IntervalProvider) NewTrigger(logger log.Logger, _ *client.Client) (types.Trigger, error) {
	return &IntervalTrigger{logger: logger, interval: p.interval}, nil
}

func (t *IntervalTrigger) Run(stopc <-chan struct{}) error {
	ticker := time.NewTicker(t.interval)
	defer ticker.Stop()

	for {
		select {
		case <-stopc:
			return nil
		case <-ticker.C:
			t.logger.Log("msg", "interval triggered")
			t.Execute(nil)
		}
	}
}
