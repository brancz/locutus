package trigger

import (
	"context"

	"github.com/brancz/locutus/rollout"
)

type Trigger interface {
	Run(context.Context) error
	Register(Execution)
}

type Execution interface {
	Execute(context.Context, *rollout.Config) error
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

func (r *ExecutionRegister) Execute(ctx context.Context, config *rollout.Config) error {
	for _, e := range r.executions {
		err := e.Execute(ctx, config)
		if err != nil {
			return err
		}
	}

	return nil
}
