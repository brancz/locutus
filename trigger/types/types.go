package types

import (
	"flag"

	"github.com/brancz/locutus/client"
	"github.com/brancz/locutus/rollout"
	"github.com/go-kit/kit/log"
)

type Provider interface {
	NewTrigger(log.Logger, *client.Client) (Trigger, error)
	RegisterFlags(s *flag.FlagSet)
	Name() string
}

type Trigger interface {
	Run(stopc <-chan struct{}) error
	Register(Execution)
}

type Execution interface {
	Execute(*rollout.Config) error
}

type ExecutionRegister struct {
	executions []Execution
}

func (r *ExecutionRegister) Register(execution Execution) {
	if r.executions == nil {
		r.executions = []Execution{}
	}

	r.executions = append(r.executions, execution)
}

func (r *ExecutionRegister) Execute(config *rollout.Config) error {
	for _, e := range r.executions {
		err := e.Execute(config)
		if err != nil {
			return err
		}
	}

	return nil
}
