package rollout

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/brancz/locutus/render"
	"github.com/go-kit/kit/log"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/brancz/locutus/client"
	"github.com/brancz/locutus/feedback"
	"github.com/brancz/locutus/rollout/checks"
	"github.com/brancz/locutus/rollout/types"
)

type Renderer interface {
	Render(config []byte) (*render.Result, error)
}

type rolloutMetrics struct {
	executionDuration prometheus.Summary
	executions        prometheus.Counter
	executionsFailed  prometheus.Counter
}

type Runner struct {
	logger     log.Logger
	client     *client.Client
	actions    map[string]ObjectAction
	checks     *checks.SuccessChecks
	provider   Renderer
	renderOnly bool
	metrics    *rolloutMetrics
}

func NewRunner(r prometheus.Registerer, logger log.Logger, client *client.Client, renderer Renderer, checks *checks.SuccessChecks, renderOnly bool) *Runner {
	m := &rolloutMetrics{
		executionDuration: prometheus.NewSummary(prometheus.SummaryOpts{
			Name: "rollout_execution_duration_seconds",
			Help: "Time executions are taking.",
		}),
		executions: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "rollout_executions_total",
			Help: "Total number of times rollouts have been executed.",
		}),
		executionsFailed: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "rollout_executions_failed_total",
			Help: "Total number of times rollouts failed.",
		}),
	}

	if r != nil {
		r.MustRegister(m.executionDuration)
		r.MustRegister(m.executions)
		r.MustRegister(m.executionsFailed)
	}

	return &Runner{
		logger:     logger,
		client:     client,
		actions:    map[string]ObjectAction{},
		checks:     checks,
		provider:   renderer,
		renderOnly: renderOnly,
		metrics:    m,
	}
}

func (r *Runner) SetObjectActions(actions []ObjectAction) {
	for _, a := range actions {
		r.actions[a.Name()] = a
	}
}

type Config struct {
	RawConfig []byte
	Feedback  feedback.Feedback
}

func (r *Runner) Execute(rolloutConfig *Config) (err error) {
	var rawConfig []byte = nil
	if rolloutConfig != nil {
		rawConfig = rolloutConfig.RawConfig
	}

	begin := time.Now()
	defer func() {
		r.metrics.executionDuration.Observe(time.Since(begin).Seconds())
		r.metrics.executions.Inc()
		if err != nil {
			r.metrics.executionsFailed.Inc()
		}
	}()

	var res *render.Result
	res, err = r.provider.Render(rawConfig)
	if err != nil {
		return fmt.Errorf("failed to render: %v", err)
	}

	if r.renderOnly {
		return json.NewEncoder(os.Stdout).Encode(res)
	}

	if rolloutConfig != nil && rolloutConfig.Feedback != nil {

		groups := []string{}
		for _, g := range res.Rollout.Spec.Groups {
			groups = append(groups, g.Name)
		}

		err := rolloutConfig.Feedback.Initialize(groups)
		if err != nil {
			return err
		}
	}

	for _, group := range res.Rollout.Spec.Groups {
		for _, step := range group.Steps {
			object, found := res.Objects[step.Object]
			if !found {
				return fmt.Errorf("Could not find object named %q", step.Object)
			}

			err := r.runStep(step, object)
			if err != nil {
				return errors.Wrap(err, "failed to run step")
			}
		}
		if rolloutConfig != nil && rolloutConfig.Feedback != nil {
			err := rolloutConfig.Feedback.SetCondition(group.Name, feedback.StatusConditionFinished)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (r *Runner) runStep(step *types.Step, object *unstructured.Unstructured) error {
	err := r.executeAction(step.Action, object)
	if err != nil {
		return fmt.Errorf("failed to execute action (%s): %v", step.Action, err)
	}

	return r.checks.RunChecks(step.Success, object)
}

func (r *Runner) executeAction(actionName string, u *unstructured.Unstructured) error {
	isList := u.IsList()
	if isList {
		return u.EachListItem(func(o runtime.Object) error {
			u := o.(*unstructured.Unstructured)

			return r.executeAction(actionName, u)
		})
	}

	return r.executeSingleAction(actionName, u)
}

func (r *Runner) executeSingleAction(actionName string, unstructured *unstructured.Unstructured) error {
	action := r.actions[actionName]
	rc, err := r.client.ClientForUnstructured(unstructured)
	if err != nil {
		return err
	}

	return action.Execute(rc, unstructured)
}
