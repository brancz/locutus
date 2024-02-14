package database

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/log/level"
	"github.com/jackc/pgx/v4"
	"github.com/oklog/run"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/util/yaml"

	"github.com/brancz/locutus/db"
	"github.com/brancz/locutus/rollout"
	"github.com/brancz/locutus/source/crdb"
	"github.com/brancz/locutus/trigger"
)

type TriggerConfigs struct {
	Triggers []TriggerConfig `json:"triggers"`
}

type TriggerConfig struct {
	Name              string `json:"name"`
	DatabaseName      string `json:"databaseName"`
	Query             string `json:"query"`
	Key               string `json:"key"`
	GroupsRowsToArray bool   `json:"groupsRowsToArray"`
}

type TriggerRunner struct {
	trigger.ExecutionRegister

	db     *db.Connections
	logger log.Logger

	config TriggerConfigs

	activeTriggers map[string]map[string]*TriggerRun
}

type TriggerRun struct {
	logger log.Logger
	runner *TriggerRunner

	key string

	mtx  *sync.Mutex
	done bool
}

func (t *TriggerRun) Done() bool {
	t.mtx.Lock()
	defer t.mtx.Unlock()
	return t.done
}

func (t *TriggerRun) Run(ctx context.Context, payload []byte) {
	if err := t.run(ctx, payload); err != nil {
		level.Error(t.logger).Log("msg", "error running", "err", err)
	}
	t.mtx.Lock()
	t.done = true
	t.mtx.Unlock()
}

func (t *TriggerRun) run(ctx context.Context, payload []byte) error {
	level.Debug(t.logger).Log("msg", "triggered", "key", t.key)
	return t.runner.Execute(ctx, &rollout.Config{
		RawConfig: payload,
	})
}

func NewTrigger(
	logger log.Logger,
	db *db.Connections,
	configFile string,
) (*TriggerRunner, error) {
	t := &TriggerRunner{
		logger:         logger,
		db:             db,
		activeTriggers: map[string]map[string]*TriggerRun{},
	}

	f, err := os.Open(configFile)
	if err != nil {
		return nil, fmt.Errorf("open trigger config file: %w", err)
	}
	defer f.Close()

	if err := yaml.NewYAMLOrJSONDecoder(f, 100).Decode(&t.config); err != nil {
		return nil, fmt.Errorf("parse config file: %w", err)
	}

	for _, triggerConfig := range t.config.Triggers {
		t.activeTriggers[triggerConfig.Name] = map[string]*TriggerRun{}
	}

	return t, nil
}

func (t *TriggerRunner) Run(ctx context.Context) error {
	g := run.Group{}
	for _, triggerConfig := range t.config.Triggers {
		triggerConfig := triggerConfig
		ctx, cancel := context.WithCancel(ctx)
		g.Add(func() error {
			t.runTrigger(ctx, triggerConfig)
			return nil
		}, func(error) {
			cancel()
		})
	}
	return g.Run()
}

func (t *TriggerRunner) runTrigger(ctx context.Context, triggerConfig TriggerConfig) {
	ticker := time.NewTicker(10 * time.Second)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := t.checkTrigger(ctx, triggerConfig); err != nil {
				level.Error(t.logger).Log("msg", "error checking trigger", "err", err)
			}
		}
	}
}

func (t *TriggerRunner) checkTrigger(ctx context.Context, c TriggerConfig) error {
	for key, trigger := range t.activeTriggers[c.Name] {
		if trigger.Done() {
			delete(t.activeTriggers, key)
		}
	}

	conn, ok := t.db.Connections[c.DatabaseName]
	if !ok {
		return errors.Errorf("no connection for database %q", c.DatabaseName)
	}

	switch conn.Type {
	case db.TypeCockroachDB:
		return t.cockroachTrigger(ctx, conn.CockroachClient, c)
	default:
		return errors.Errorf("unsupported database type %q", conn.Type)
	}
}

func (t *TriggerRunner) ScheduleTriggerRun(ctx context.Context, triggerName, key string, payload []byte) {
	if _, ok := t.activeTriggers[key]; !ok {
		run := &TriggerRun{
			logger: t.logger,
			key:    key,
			runner: t,
			mtx:    &sync.Mutex{},
		}
		t.activeTriggers[triggerName][key] = run

		go run.Run(ctx, payload)
	}
}

func (t *TriggerRunner) cockroachTrigger(ctx context.Context, conn *crdb.Client, c TriggerConfig) error {
	if err := conn.ExecuteTx(ctx, pgx.TxOptions{}, func(tx pgx.Tx) error {
		level.Debug(t.logger).Log("msg", "executing trigger query", "name", c.Name)
		rows, err := tx.Query(ctx, c.Query)
		if err != nil {
			return err
		}
		defer rows.Close()

		fields := rows.FieldDescriptions()
		columns := make([]string, 0, len(fields))

		for _, field := range fields {
			columns = append(columns, string(field.Name))
		}

		rowsArray := make([]map[string]any, 0)
		groupTriggerKey := ""
		for rows.Next() {
			row := make(map[string]any, len(columns))

			values, err := rows.Values()
			if err != nil {
				return err
			}

			for i, column := range columns {
				row[column] = values[i]
			}

			key, ok := row[c.Key]
			if !ok {
				return errors.Errorf("key column %q not in result", c.Key)
			}

			var triggerKey string
			switch key := key.(type) {
			case string:
				triggerKey = key
			case []byte:
				triggerKey = fmt.Sprintf("%x", key)
			default:
				triggerKey = fmt.Sprintf("%v", key)
			}
			groupTriggerKey += triggerKey

			if !c.GroupsRowsToArray {
				payload, err := json.Marshal(row)
				if err != nil {
					return err
				}

				t.ScheduleTriggerRun(ctx, c.Name, triggerKey, payload)
			} else {
				rowsArray = append(rowsArray, row)
			}
		}

		if c.GroupsRowsToArray {
			payload, err := json.Marshal(rowsArray)
			if err != nil {
				return err
			}

			t.ScheduleTriggerRun(ctx, c.Name, groupTriggerKey, payload)
		}

		return nil
	}); err != nil {
		return fmt.Errorf("execute tx: %w", err)
	}

	return nil
}
