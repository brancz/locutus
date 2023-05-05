package source

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/go-kit/log"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v4"
	"github.com/ory/dockertest/v3"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/brancz/locutus/source/crdb"
)

func TestCockroachdbSource(t *testing.T) {
	t.Parallel()

	pool, err := dockertest.NewPool("")
	if err != nil {
		t.Fatalf("Could not connect to docker: %s", err)
	}

	resource, err := pool.RunWithOptions(&dockertest.RunOptions{Repository: "cockroachdb/cockroach", Tag: "v21.1.2", Cmd: []string{"start-single-node", "--insecure"}})
	if err != nil {
		t.Fatalf("Could not start resource: %s", err)
	}
	defer func() {
		if err = pool.Purge(resource); err != nil {
			t.Fatalf("Could not purge resource: %s", err)
		}
	}()

	if err = pool.Retry(func() error {
		m, err := migrate.New(
			"file://migrations",
			// Migrate needs the explicit cockroachdb protocol specified here.
			fmt.Sprintf("cockroachdb://root@localhost:%s/defaultdb?sslmode=disable", resource.GetPort("26257/tcp")),
		)
		if err != nil {
			t.Logf("failed to instantiate migrations, retrying ... err: %s", err)
			return err
		}
		if err := m.Up(); err != nil && err.Error() != "no change" {
			t.Logf("failed to run migrations, retrying ... err: %s", err)
			return err
		}

		return nil
	}); err != nil {
		t.Fatalf("Could not connect to docker: %s", err)
	}

	ctx := context.Background()
	crdbClient, err := crdb.NewClient(
		ctx,
		prometheus.NewRegistry(),
		fmt.Sprintf("postgresql://root@localhost:%s/defaultdb?sslmode=disable", resource.GetPort("26257/tcp")),
	)
	if err != nil {
		t.Fatalf("failed to create crdb client: %s", err)
	}

	rows := []struct {
		name    string
		enabled bool
	}{
		{
			name:    "test1",
			enabled: true,
		},
		{
			name:    "test2",
			enabled: false,
		},
	}

	if err := crdbClient.ExecuteTx(ctx, pgx.TxOptions{}, func(tx pgx.Tx) error {
		for _, row := range rows {
			if _, err := tx.Exec(ctx, "INSERT INTO test (name, enabled) VALUES ($1, $2)", row.name, row.enabled); err != nil {
				return err
			}
		}

		return nil
	}); err != nil {
		t.Fatalf("Could not insert data: %s", err)
	}

	sources := (&CockroachdbSource{
		logger: log.NewLogfmtLogger(os.Stdout),
		crdb:   crdbClient,
		config: CockroachdbSourceConfig{
			Queries: []CockroachdbSourceConfigQuery{{
				Key:   "test",
				Query: `SELECT name, enabled FROM test ORDER BY name;`,
			}},
		},
	}).InputSources()

	payload, err := sources["test"](ctx)
	if err != nil {
		t.Fatalf("Unexpected error: %s", err)
	}

	resultRows := []map[string]any{}
	if err := json.Unmarshal(payload, &resultRows); err != nil {
		t.Fatalf("Unexpected error: %s", err)
	}

	expectedRows := []map[string]any{{
		"name":    "test1",
		"enabled": true,
	}, {
		"name":    "test2",
		"enabled": false,
	}}

	if len(resultRows) != len(expectedRows) {
		t.Fatalf("Unexpected number of rows: %d", len(resultRows))
	}

	if resultRows[0]["name"] != expectedRows[0]["name"] ||
		resultRows[0]["enabled"] != expectedRows[0]["enabled"] {
		t.Fatalf("Unexpected row 1; got %v, expected %v", resultRows[0], expectedRows[0])
	}
	if resultRows[1]["name"] != expectedRows[1]["name"] ||
		resultRows[1]["enabled"] != expectedRows[1]["enabled"] {
		t.Fatalf("Unexpected row 1; got %v, expected %v", resultRows[0], expectedRows[0])
	}
}
