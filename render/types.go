package render

import (
	"github.com/brancz/locutus/rollout/types"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type Result struct {
	Objects map[string]*unstructured.Unstructured `json:"objects"`
	Rollout *types.Rollout                        `json:"rollout"`
}
