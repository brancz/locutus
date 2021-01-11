package types

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
	Groups []*RolloutGroup `json:"groups"`
}

type RolloutGroup struct {
	Name  string  `json:"name"`
	Steps []*Step `json:"steps"`
}

type Step struct {
	Object  string               `json:"object"`
	Action  string               `json:"action"`
	Success []*SuccessDefinition `json:"success"`
}

type SuccessDefinition struct {
	FieldCheck            []*FieldCheckDefinition            `json:"fieldCheck"`
	PrometheusMetricCheck []*PrometheusMetricCheckDefinition `json:"prometheusMetricCheck"`
}

type FieldCheckDefinition struct {
	Name    string           `json:"name"`
	Path    string           `json:"path"`
	Default interface{}      `json:"default"`
	Value   *FieldCheckValue `json:"value"`
}

type FieldComparisonValue struct {
	Path   string      `json:"path"`
	Static interface{} `json:"static"`
}

type PrometheusMetricCheckDefinition struct {
	Query        string                               `json:"query"`
	Expectations []*PrometheusInstantQueryExpectation `json:"expectations"`
}

type PrometheusInstantQueryExpectation struct {
	Each *PrometheusVectorEntryExpectation `json:"each"`
}

type PrometheusVectorEntryExpectation struct {
	Type  PrometheusVectorEntryExpectationType `json:"type"`
	Value float64
}

type PrometheusVectorEntryExpectationType string

const (
	PrometheusVectorEntryGTE = "PrometheusVectorEntryGreaterThanOrEqual"
	PrometheusVectorEntryLTE = "PrometheusVectorEntryLessThanOrEqual"
)
