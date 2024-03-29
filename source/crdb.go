package source

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/go-kit/log"
	"github.com/jackc/pgx/v4"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/util/yaml"

	"github.com/brancz/locutus/db"
	"github.com/brancz/locutus/source/crdb"
)

type DatabaseSources struct {
	logger log.Logger
	conns  *db.Connections
	config DatabaseSourceConfig
}

type DatabaseSourceConfig struct {
	Queries []DatabaseSourceConfigQuery `json:"queries"`
}

type DatabaseSourceConfigQuery struct {
	Name         string `json:"name"`
	DatabaseName string `json:"databaseName"`
	Query        string `json:"query"`
}

func NewDatabaseSources(
	logger log.Logger,
	conns *db.Connections,
	file string,
) (*DatabaseSources, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, fmt.Errorf("open config file: %w", err)
	}
	defer f.Close()

	var config DatabaseSourceConfig
	err = yaml.NewYAMLOrJSONDecoder(f, 100).Decode(&config)
	if err != nil {
		return nil, fmt.Errorf("parse config file: %w", err)
	}

	return &DatabaseSources{
		logger: logger,
		conns:  conns,
		config: config,
	}, nil
}

func (s *DatabaseSources) InputSources() (map[string]func(context.Context) ([]byte, error), error) {
	res := map[string]func(context.Context) ([]byte, error){}

	var err error
	for _, query := range s.config.Queries {
		res[query.Name], err = s.sourceForQuery(query)
		if err != nil {
			return nil, err
		}
	}

	return res, nil
}

func (s *DatabaseSources) sourceForQuery(q DatabaseSourceConfigQuery) (func(context.Context) ([]byte, error), error) {
	conn, ok := s.conns.Connections[q.DatabaseName]
	if !ok {
		return nil, errors.Errorf("no connection for database %q", q.DatabaseName)
	}

	switch conn.Type {
	case db.TypeCockroachDB:
		return cockroachSource(conn.CockroachClient, q.Query), nil
	default:
		return nil, errors.Errorf("unsupported database type %q", conn.Type)
	}
}

func cockroachSource(conn *crdb.Client, query string) func(context.Context) ([]byte, error) {
	return func(ctx context.Context) ([]byte, error) {
		res := []map[string]any{}

		if err := conn.ExecuteTx(ctx, pgx.TxOptions{}, func(tx pgx.Tx) error {
			rows, err := tx.Query(ctx, query)
			if err != nil {
				return err
			}
			defer rows.Close()

			fields := rows.FieldDescriptions()
			columns := make([]string, 0, len(fields))

			for _, field := range fields {
				columns = append(columns, string(field.Name))
			}

			for rows.Next() {
				row := make(map[string]any, len(columns))

				values, err := rows.Values()
				if err != nil {
					return err
				}

				for i, column := range columns {
					row[column] = values[i]
				}

				res = append(res, row)
			}

			return nil
		}); err != nil {
			return nil, err
		}

		return json.Marshal(res)
	}
}
