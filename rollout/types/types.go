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
	FieldComparisons []*FieldComparisonSuccessDefinition `json:"fieldComparisons"`
}

type FieldComparisonSuccessDefinition struct {
	Name    string                `json:"name"`
	Path    string                `json:"path"`
	Default interface{}           `json:"default"`
	Value   *FieldComparisonValue `json:"value"`
}

type FieldComparisonValue struct {
	Path   string      `json:"path"`
	Static interface{} `json:"static"`
}
