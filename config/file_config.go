package config

import (
	"context"
	"io/ioutil"

	"github.com/brancz/locutus/rollout"
	"github.com/brancz/locutus/trigger"
)

type FileConfigPasser struct {
	file     string
	executor trigger.Execution
}

func NewFileConfigPasser(file string, executor trigger.Execution) *FileConfigPasser {
	return &FileConfigPasser{
		file:     file,
		executor: executor,
	}
}

func (p *FileConfigPasser) Execute(ctx context.Context, rolloutConfig *rollout.Config) error {
	var err error

	if rolloutConfig == nil {
		rolloutConfig = &rollout.Config{}
	}

	rolloutConfig.RawConfig, err = p.readConfig(rolloutConfig)
	if err != nil {
		return err
	}

	return p.executor.Execute(ctx, rolloutConfig)
}

func (p *FileConfigPasser) readConfig(rolloutConfig *rollout.Config) ([]byte, error) {
	if p.file == "" {
		return rolloutConfig.RawConfig, nil
	}
	return ioutil.ReadFile(p.file)
}
