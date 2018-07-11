package config

import (
	"io/ioutil"

	"github.com/brancz/locutus/rollout"
	"github.com/brancz/locutus/trigger/types"
)

type ConfigPasser struct {
	file     string
	executor types.Execution
}

func NewConfigPasser(file string, executor types.Execution) *ConfigPasser {
	return &ConfigPasser{
		file:     file,
		executor: executor,
	}
}

func (p *ConfigPasser) Execute(rolloutConfig *rollout.Config) error {
	var err error

	if rolloutConfig == nil {
		rolloutConfig = &rollout.Config{}
	}

	rolloutConfig.RawConfig, err = p.readConfig(rolloutConfig)
	if err != nil {
		return err
	}

	return p.executor.Execute(rolloutConfig)
}

func (p *ConfigPasser) readConfig(rolloutConfig *rollout.Config) ([]byte, error) {
	if p.file == "" {
		return rolloutConfig.RawConfig, nil
	}
	return ioutil.ReadFile(p.file)
}
