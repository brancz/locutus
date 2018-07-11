package trigger

import (
	"flag"
	"fmt"

	"github.com/brancz/locutus/trigger/interval"
	"github.com/brancz/locutus/trigger/oneoff"
	"github.com/brancz/locutus/trigger/resource"
	"github.com/brancz/locutus/trigger/types"
)

type providerSelection struct {
	providers []types.Provider
}

func Providers() *providerSelection {
	return &providerSelection{
		providers: []types.Provider{
			&interval.IntervalProvider{},
			&oneoff.OneOffProvider{},
			&resource.ResourceProvider{},
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
