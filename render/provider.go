package render

import (
	"flag"
	"fmt"

	"github.com/brancz/locutus/render/file"
	"github.com/brancz/locutus/render/jsonnet"
	"github.com/brancz/locutus/render/types"
)

type providerSelection struct {
	providers []types.Provider
}

func Providers() *providerSelection {
	// default providers
	return &providerSelection{
		providers: []types.Provider{
			jsonnet.NewProvider(),
			file.NewProvider(),
		},
	}

}

func (s *providerSelection) RegisterFlags(set *flag.FlagSet) {
	for _, p := range s.providers {
		p.RegisterFlags(set)
	}
}

func (s *providerSelection) Select(providerName string) (types.Provider, error) {
	for _, p := range s.providers {
		if p.Name() == providerName {
			return p, nil
		}
	}

	return nil, fmt.Errorf("provider not found")
}
