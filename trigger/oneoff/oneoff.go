package oneoff

import (
	"flag"
	"time"

	"github.com/go-kit/kit/log"

	"github.com/brancz/locutus/client"
	"github.com/brancz/locutus/trigger/types"
)

type OneOffProvider struct {
	interval time.Duration
}

func (p *OneOffProvider) RegisterFlags(s *flag.FlagSet) {
	// nothing to configure with flags
}

func (p *OneOffProvider) Name() string {
	return "oneoff"
}

type OneOffTrigger struct {
	types.ExecutionRegister

	logger log.Logger
}

func (p *OneOffProvider) NewTrigger(logger log.Logger, _ *client.Client) (types.Trigger, error) {
	return &OneOffTrigger{logger: logger}, nil
}

func (t *OneOffTrigger) Run(stopc <-chan struct{}) error {
	return t.Execute(nil)
}
