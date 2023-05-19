package types

import (
	"encoding/json"
	"errors"
	"time"
)

type Rollout struct {
	APIVersion string       `json:"apiVersion"`
	Kind       string       `json:"kind"`
	Metadata   *Metadata    `json:"metadata"`
	Spec       *RolloutSpec `json:"spec"`
}

type Metadata struct {
	Name string `json:"name"`
}

type RolloutSpec struct {
	Parallel bool            `json:"parallel"`
	Groups   []*RolloutGroup `json:"groups"`
}

type RolloutGroup struct {
	Name     string  `json:"name"`
	Parallel bool    `json:"parallel"`
	Steps    []*Step `json:"steps"`
}

type Step struct {
	Name            string               `json:"name"`
	Object          string               `json:"object"`
	Action          string               `json:"action"`
	Success         []*SuccessDefinition `json:"success"`
	ContinueOnError bool                 `json:"continueOnError"`
}

type SuccessDefinition struct {
	FieldComparisons *FieldComparisons    `json:"fieldComparisons"`
	Failure          []*FailureDefinition `json:"failure"`
}

type FieldComparisons struct {
	ExpectedValues  []*ExpectedFieldComparisonValue `json:"expectedValues"`
	Timeout         Duration                        `json:"timeout"`
	ProgressTimeout Duration                        `json:"progressTimeout"`
	PollInterval    Duration                        `json:"pollInterval"`
	ReportTimeout   *ReportConfig                   `json:"reportTimeout"`
	Failure         []*FailureDefinition            `json:"failure"`
}

type ExpectedFieldComparisonValue struct {
	Name    string                `json:"name"`
	Path    string                `json:"path"`
	Default interface{}           `json:"default"`
	Value   *FieldComparisonValue `json:"value"`
}

type FieldComparisonValue struct {
	Path        string      `json:"path"`
	Static      interface{} `json:"static"`
	StaticInt64 int64       `json:"staticInt64"`
}

type ReportConfig struct {
	Database *DatabaseReportConfig `json:"database"`
}

type DatabaseReportConfig struct {
	DatabaseName string              `json:"name"`
	Query        DatabaseReportQuery `json:"query"`
}

type DatabaseReportQuery struct {
	Stmt string `json:"stmt"`
}

type FailureDefinition struct {
	CheckName string        `json:"checkName"`
	Report    *ReportConfig `json:"report"`
}

type Duration struct {
	time.Duration
}

func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(d.String())
}

func (d *Duration) UnmarshalJSON(b []byte) error {
	var v interface{}
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	switch value := v.(type) {
	case float64:
		d.Duration = time.Duration(value)
		return nil
	case string:
		var err error
		d.Duration, err = time.ParseDuration(value)
		if err != nil {
			return err
		}
		return nil
	default:
		return errors.New("invalid duration")
	}
}
