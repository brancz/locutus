package types

import (
	"flag"

	"github.com/brancz/locutus/rollout/types"
	"github.com/go-kit/kit/log"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type Provider interface {
	RegisterFlags(s *flag.FlagSet)
	NewRenderer(logger log.Logger) Renderer
	Name() string
}

type Renderer interface {
	Render(config []byte) (*Result, error)
}

type Result struct {
	Objects map[string]*unstructured.Unstructured `json:"objects"`
	Rollout *types.Rollout                        `json:"rollout"`
}
