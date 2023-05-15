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
	TypeCockroachDB string = "cockroachdb"
)

type ConnectionsConfig struct {
	Connections []ConnectionConfig `json:"connections"`
}

type ConnectionConfig struct {
	Name        string                       `json:"name"`
	Type        string                       `json:"type"`
	CockroachDB *ConnectionConfigCockroachdb `json:"cockroachdb,omitempty"`
}

type ConnectionConfigCockroachdb struct {
	ConnString string `json:"conn_string"`
}

type Connections struct {
	Connections map[string]*Connection
}

type Connection struct {
	Type            string
	CockroachClient *crdb.Client
}

func FromFile(
	ctx context.Context,
	reg prometheus.Registerer,
	file string,
) (*Connections, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, errors.Wrap(err, "failed to open config file")
	}
	defer f.Close()

	var config ConnectionsConfig
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

	connections := map[string]*Connection{}
	for _, conn := range config.Connections {
		switch conn.Type {
		case TypeCockroachDB:
			client, err := crdb.NewClient(
				ctx,
				reg,
				conn.CockroachDB.ConnString,
			)
			if err != nil {
				return nil, fmt.Errorf("create cockroachdb client: %w", err)
			}

			connections[conn.Name] = &Connection{
				Type:            conn.Type,
				CockroachClient: client,
			}
		default:
			return nil, errors.Errorf("unknown connection type: %s", conn.Type)
		}
	}

	return &Connections{
		Connections: connections,
	}, nil
}
