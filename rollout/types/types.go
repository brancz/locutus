package types

import "time"

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
	Object  string               `json:"object"`
	Action  string               `json:"action"`
	Success []*SuccessDefinition `json:"success"`
}

type SuccessDefinition struct {
	FieldComparisons *FieldComparisons    `json:"fieldComparisons"`
	Failure          []*FailureDefinition `json:"failure"`
}

type FieldComparisons struct {
	ExpectedValues  []*ExpectedFieldComparisonValue `json:"expectedValues"`
	Timeout         time.Duration                   `json:"timeout"`
	ProgressTimeout time.Duration                   `json:"progressTimeout"`
	PollInterval    time.Duration                   `json:"pollInterval"`
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
	Path   string      `json:"path"`
	Static interface{} `json:"static"`
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
