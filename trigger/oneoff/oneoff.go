package oneoff

import (
	"context"

	"github.com/brancz/locutus/trigger"
	"github.com/go-kit/kit/log"
)

type Trigger struct {
	trigger.ExecutionRegister
	logger log.Logger
}

func NewTrigger(logger log.Logger) trigger.Trigger {
	return &Trigger{logger: logger}
}

func (t *Trigger) Run(ctx context.Context) error {
	return t.Execute(ctx, nil)
}
