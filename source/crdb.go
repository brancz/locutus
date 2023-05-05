package source

import (
	"context"
	"encoding/json"
	"os"

	"github.com/go-kit/log"
	_ "github.com/golang-migrate/migrate/v4/database/cockroachdb"
	"github.com/jackc/pgx/v4"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/util/yaml"

	"github.com/brancz/locutus/source/crdb"
)

type CockroachdbSource struct {
	logger log.Logger
	crdb   *crdb.CockroachClient
	config CockroachdbSourceConfig
}

type CockroachdbSourceConfig struct {
	Queries []CockroachdbSourceConfigQuery `json:"queries"`
}

type CockroachdbSourceConfigQuery struct {
	Key   string `json:"key"`
	Query string `json:"query"`
}

func NewCockroachdbSource(
	logger log.Logger,
	crdb *crdb.CockroachClient,
	file string,
) (*CockroachdbSource, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, errors.Wrap(err, "failed to open config file")
	}
	defer f.Close()

	var config CockroachdbSourceConfig
	err = yaml.NewYAMLOrJSONDecoder(f, 100).Decode(&config)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse config file")
	}

	return &CockroachdbSource{
		logger: logger,
		crdb:   crdb,
		config: config,
	}, nil
}

func (s *CockroachdbSource) InputSources() map[string]func(context.Context) ([]byte, error) {
	res := map[string]func(context.Context) ([]byte, error){}

	for _, query := range s.config.Queries {
		res[query.Key] = func(ctx context.Context) ([]byte, error) {
			res := []map[string]any{}

			if err := s.crdb.ExecuteTx(ctx, pgx.TxOptions{}, func(tx pgx.Tx) error {
				rows, err := tx.Query(ctx, query.Query)
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

	return res
}
