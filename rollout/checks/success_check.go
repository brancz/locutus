package checks

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	"github.com/brancz/locutus/client"
	"github.com/brancz/locutus/db"
	"github.com/brancz/locutus/rollout/types"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/jackc/pgx/v4"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/jsonpath"
)

type jsonPath struct {
	jsonpath     *jsonpath.JSONPath
	defaultValue interface{}
}

func (j *jsonPath) findResult(data map[string]interface{}) (interface{}, error) {
	jsonPathResult, err := j.jsonpath.FindResults(data)
	if err != nil && !strings.HasSuffix(err.Error(), " is not found") {
		return nil, err
	}

	if len(jsonPathResult) == 1 && len(jsonPathResult[0]) == 1 {
		return jsonPathResult[0][0].Interface(), nil
	}

	if len(jsonPathResult) == 0 && j.defaultValue != nil {
		return j.defaultValue, nil
	}

	return nil, fmt.Errorf("Expected 1 result but found different amount.")
}

type Check interface {
	Execute(ctx context.Context, client *client.Client, u *unstructured.Unstructured) error
	Name() string
	IsFailedError(err error) bool
}

type CheckRunner struct {
	logger              log.Logger
	client              *client.Client
	def                 *types.SuccessDefinition
	paths               map[string]*jsonPath
	databaseConnections *db.Connections
	knownChecks         map[string]Check
}

func NewCheckRunner(
	logger log.Logger,
	client *client.Client,
	def *types.SuccessDefinition,
	databaseConnections *db.Connections,
	knownChecks map[string]Check,
) (*CheckRunner, error) {
	paths := map[string]*jsonPath{}
	for _, ev := range def.FieldComparisons.ExpectedValues {
		jp := jsonpath.New("rollout jsonpath")
		err := jp.Parse(ev.Path)
		if err != nil {
			return nil, err
		}

		paths[ev.Path] = &jsonPath{jsonpath: jp, defaultValue: ev.Default}
		if ev.Value != nil && ev.Value.Path != "" {
			jp := jsonpath.New("rollout jsonpath")
			err := jp.Parse(ev.Value.Path)
			if err != nil {
				return nil, err
			}
			paths[ev.Value.Path] = &jsonPath{jsonpath: jp}
		}
	}

	return &CheckRunner{
		logger:              logger,
		client:              client,
		def:                 def,
		paths:               paths,
		databaseConnections: databaseConnections,
		knownChecks:         knownChecks,
	}, nil
}

func (c *CheckRunner) Execute(ctx context.Context, u *unstructured.Unstructured) error {
	level.Debug(c.logger).Log("msg", "starting field comparison success check", "name", u.GetName(), "namespace", u.GetNamespace())
	name := u.GetName()
	namespace := u.GetNamespace()

	rc, err := c.client.ClientForUnstructured(u)
	if err != nil {
		return errors.Wrap(err, "failed to get client for unstructured")
	}

	err = wait.Poll(c.def.FieldComparisons.PollInterval.Duration, c.def.FieldComparisons.Timeout.Duration, wait.ConditionFunc(func() (bool, error) {
		level.Debug(c.logger).Log("msg", "starting poll", "name", name, "namespace", namespace)
		outerValues, err := c.currentValues(ctx, rc, name)
		if err != nil {
			return false, errors.Wrap(err, "failed to extract periodic status information")
		}

		success, checkReports := c.checkComparisons(outerValues)
		for _, checkReport := range checkReports {
			level.Debug(c.logger).Log("name", name, "namespace", namespace, "check-name", checkReport.CheckName, "check-message", checkReport.Message)
		}

		if success {
			return true, nil
		}

		if err := c.checkFailed(ctx, u); err != nil {
			return false, fmt.Errorf("check if rollout failed: %w", err)
		}

		err = wait.Poll(c.def.FieldComparisons.PollInterval.Duration, c.def.FieldComparisons.ProgressTimeout.Duration, wait.ConditionFunc(func() (bool, error) {
			level.Debug(c.logger).Log("msg", "check whether fields have changed", "name", name, "namespace", namespace)
			innerValues, err := c.currentValues(ctx, rc, name)
			if err != nil {
				return false, errors.Wrap(err, "failed to extract updated status information")
			}

			hasChanged := !reflect.DeepEqual(outerValues, innerValues)
			level.Debug(c.logger).Log("msg", "finished checking whether fields have changed", "name", name, "namespace", namespace, "hasChanged", hasChanged)

			success, checkReports := c.checkComparisons(outerValues)
			for _, checkReport := range checkReports {
				level.Debug(c.logger).Log("name", name, "namespace", namespace, "check-name", checkReport.CheckName, "check-message", checkReport.Message)
			}

			if success {
				return true, nil
			}

			if err := c.checkFailed(ctx, u); err != nil {
				return false, fmt.Errorf("check if rollout failed: %w", err)
			}

			return hasChanged, err
		}))

		return false, err
	}))
	if err == wait.ErrWaitTimeout && c.def.FieldComparisons.ReportTimeout != nil {
		err = c.reportTimeout(ctx, u)
	}

	level.Debug(c.logger).Log("msg", "field comparison success check successful", "name", u.GetName(), "namespace", u.GetNamespace())
	return err
}

func (c *CheckRunner) checkFailed(ctx context.Context, u *unstructured.Unstructured) error {
	for _, fd := range c.def.Failure {
		check, ok := c.knownChecks[fd.CheckName]
		if !ok {
			return fmt.Errorf("unknown failure check %q", fd.CheckName)
		}

		level.Debug(c.logger).Log("msg", "running failure check", "name", u.GetName(), "namespace", u.GetNamespace(), "check-name", fd.CheckName)
		if err := check.Execute(ctx, c.client, u); err != nil {
			level.Debug(c.logger).Log("msg", "failure check failed", "name", u.GetName(), "namespace", u.GetNamespace(), "check-name", fd.CheckName, "err", err)
			if fd.Report != nil && check.IsFailedError(err) {
				if rerr := c.report(ctx, u, fd.Report); rerr != nil {
					return fmt.Errorf("failed to report failure: %w", rerr)
				}

				return nil
			}
			return fmt.Errorf("run failed check %q: %w", fd.CheckName, err)
		}
	}

	return nil
}

func (c *CheckRunner) reportTimeout(ctx context.Context, u *unstructured.Unstructured) error {
	return c.report(ctx, u, c.def.FieldComparisons.ReportTimeout)
}

func (c *CheckRunner) report(ctx context.Context, u *unstructured.Unstructured, report *types.ReportConfig) error {
	if report.Database != nil {
		return c.reportDatabase(ctx, u, report)
	}

	return errors.New("no reporting configured")
}

func (c *CheckRunner) reportDatabase(ctx context.Context, u *unstructured.Unstructured, report *types.ReportConfig) error {
	conn, ok := c.databaseConnections.Connections[report.Database.DatabaseName]
	if !ok {
		return fmt.Errorf("database connection %s not found", report.Database.DatabaseName)
	}

	switch {
	case conn.Type == db.TypeCockroachDB:
		if err := conn.CockroachClient.ExecuteTx(ctx, pgx.TxOptions{}, func(tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, report.Database.Query.Stmt); err != nil {
				return err
			}

			return nil
		}); err != nil {
			return err
		}

		return nil
	default:
		return fmt.Errorf("database type %s not supported", conn.Type)
	}
}

func (c *CheckRunner) currentValues(ctx context.Context, rc *client.ResourceClient, name string) (map[string]interface{}, error) {
	u, err := rc.Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	res := map[string]interface{}{}
	for pathString, p := range c.paths {
		jsonPathResult, err := p.findResult(u.Object)
		if err != nil {
			return nil, err
		}
		res[pathString] = jsonPathResult
	}

	return res, nil
}

func (c *CheckRunner) checkComparisons(values map[string]interface{}) (bool, []*CheckReport) {
	success := true
	reports := []*CheckReport{}

	for _, ev := range c.def.FieldComparisons.ExpectedValues {
		succeeded, report := c.checkFieldComparison(ev, values)
		if !succeeded {
			success = false
		}
		reports = append(reports, report)
	}

	return success, reports
}

func (c *CheckRunner) checkFieldComparison(expectedValue *types.ExpectedFieldComparisonValue, values map[string]interface{}) (bool, *CheckReport) {
	report := &CheckReport{
		CheckName: expectedValue.Name,
	}

	v := values[expectedValue.Path]
	valuesString := fmt.Sprintf("observed %s = %#+v (type: %T); expected ", expectedValue.Path, v, v)

	// static value check has precedence over path check
	var expected interface{}
	if expectedValue.Value.Path != "" {
		expected = values[expectedValue.Value.Path]
		valuesString += fmt.Sprintf("dynamic value of %s = %#+v (type: %T)", expectedValue.Value.Path, expected, expected)
	} else {
		if expectedValue.Value.Static == nil {
			expected = expectedValue.Value.StaticInt64
		} else {
			expected = expectedValue.Value.Static
		}
		valuesString += fmt.Sprintf("static value of %#+v (type: %T)", expected, expected)
	}

	eq := reflect.DeepEqual(v, expected)
	if eq {
		report.Message = "field comparison succeeded: " + valuesString
	}
	if !eq {
		report.Message = "field comparison failed: " + valuesString
	}

	return eq, report
}
