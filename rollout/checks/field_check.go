package checks

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/brancz/locutus/client"
	"github.com/brancz/locutus/rollout/types"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
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

type FieldCheck struct {
	logger      log.Logger
	client      *client.Client
	comparisons []*types.FieldComparisonSuccessDefinition
	paths       map[string]*jsonPath
}

func NewFieldCheck(logger log.Logger, client *client.Client, comparisons []*types.FieldComparisonSuccessDefinition) (*FieldCheck, error) {
	paths := map[string]*jsonPath{}
	for _, c := range comparisons {
		jp := jsonpath.New("rollout jsonpath")
		err := jp.Parse(c.Path)
		if err != nil {
			return nil, err
		}

		paths[c.Path] = &jsonPath{jsonpath: jp, defaultValue: c.Default}
		if c.Value != nil && c.Value.Path != "" {
			jp := jsonpath.New("rollout jsonpath")
			err := jp.Parse(c.Value.Path)
			if err != nil {
				return nil, err
			}
			paths[c.Value.Path] = &jsonPath{jsonpath: jp}
		}
	}

	return &FieldCheck{
		logger:      logger,
		client:      client,
		comparisons: comparisons,
		paths:       paths,
	}, nil
}

func (c *FieldCheck) Execute(ctx context.Context, u *unstructured.Unstructured) error {
	level.Debug(c.logger).Log("msg", "starting field comparison success check", "name", u.GetName(), "namespace", u.GetNamespace())
	name := u.GetName()
	namespace := u.GetNamespace()

	rc, err := c.client.ClientForUnstructured(u)
	if err != nil {
		return errors.Wrap(err, "failed to get client for unstructured")
	}

	err = wait.Poll(5*time.Second, time.Hour, wait.ConditionFunc(func() (bool, error) {
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

		err = wait.Poll(5*time.Second, 5*time.Minute, wait.ConditionFunc(func() (bool, error) {
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

			return hasChanged, err
		}))

		return false, err
	}))

	level.Debug(c.logger).Log("msg", "field comparison success check successful", "name", u.GetName(), "namespace", u.GetNamespace())
	return err
}

func (c *FieldCheck) currentValues(ctx context.Context, rc *client.ResourceClient, name string) (map[string]interface{}, error) {
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

func (c *FieldCheck) checkComparisons(values map[string]interface{}) (bool, []*CheckReport) {
	success := true
	reports := []*CheckReport{}

	for _, comp := range c.comparisons {
		succeeded, report := c.checkFieldComparison(comp, values)
		if !succeeded {
			success = false
			reports = append(reports, report)
			continue
		}
		reports = append(reports, report)
	}

	return success, reports
}

func (c *FieldCheck) checkFieldComparison(fieldComparison *types.FieldComparisonSuccessDefinition, values map[string]interface{}) (bool, *CheckReport) {
	report := &CheckReport{
		CheckName: fieldComparison.Name,
	}

	v := values[fieldComparison.Path]
	valuesString := fmt.Sprintf("%s = %#+v equals ", fieldComparison.Path, v)

	// static value check has precedence over path check
	var otherValue interface{}
	if fieldComparison.Value.Static != nil {
		otherValue = fieldComparison.Value.Static
		valuesString += fmt.Sprintf("static %#+v", fieldComparison.Value.Static)
	} else {
		otherValue = values[fieldComparison.Value.Path]
		valuesString += fmt.Sprintf("%s = %#+v", fieldComparison.Value.Path, otherValue)
	}

	eq := reflect.DeepEqual(v, otherValue)
	if eq {
		report.Message = "Field comparison succeeded. " + valuesString
	}
	if !eq {
		report.Message = "Field comparison Failed. " + valuesString
	}

	return eq, report
}
