package db

import (
	"context"
	"fmt"
	"os"

	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/apimachinery/pkg/util/yaml"

	"github.com/brancz/locutus/source/crdb"
)

const (
	DatabaseTypeCockroachDB string = "cockroachdb"
)

type DatabaseConnectionsConfig struct {
	Connections []DatabaseConnectionConfig `json:"connections"`
}

type DatabaseConnectionConfig struct {
	Name        string                               `json:"name"`
	Type        string                               `json:"type"`
	CockroachDB *DatabaseConnectionConfigCockroachdb `json:"cockroachdb,omitempty"`
}

type DatabaseConnectionConfigCockroachdb struct {
	ConnString string `json:"conn_string"`
}

type DatabaseConnections struct {
	Connections map[string]*DatabaseConnection
}

type DatabaseConnection struct {
	Type            string
	CockroachClient *crdb.Client
}

func FromFile(
	ctx context.Context,
	reg prometheus.Registerer,
	file string,
) (*DatabaseConnections, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, errors.Wrap(err, "failed to open config file")
	}
	defer f.Close()

	var config DatabaseConnectionsConfig
	err = yaml.NewYAMLOrJSONDecoder(f, 100).Decode(&config)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse config file")
	}

	names := map[string]struct{}{}
	for _, conn := range config.Connections {
		if _, ok := names[conn.Name]; ok {
			return nil, errors.Errorf("duplicate connection name, connection names must be unique: %s", conn.Name)
		}
		names[conn.Name] = struct{}{}
	}

	connections := map[string]*DatabaseConnection{}
	for _, conn := range config.Connections {
		switch conn.Type {
		case DatabaseTypeCockroachDB:
			client, err := crdb.NewClient(
				ctx,
				reg,
				conn.CockroachDB.ConnString,
			)
			if err != nil {
				return nil, fmt.Errorf("create cockroachdb client: %w", err)
			}

			connections[conn.Name] = &DatabaseConnection{
				Type:            conn.Type,
				CockroachClient: client,
			}
		default:
			return nil, errors.Errorf("unknown connection type: %s", conn.Type)
		}
	}

	return &DatabaseConnections{
		Connections: connections,
	}, nil
}
