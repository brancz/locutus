package crdb

import (
	"context"
	"fmt"
	"time"

	"github.com/cockroachdb/cockroach-go/v2/crdb/crdbpgx"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

type metrics struct {
	txDuration prometheus.ObserverVec
}

type Client struct {
	pool    *pgxpool.Pool
	metrics *metrics
}

func NewClient(ctx context.Context, reg prometheus.Registerer, connString string) (*Client, error) {
	config, err := pgxpool.ParseConfig(connString)
	if err != nil {
		return nil, fmt.Errorf("failed to parse DB string: %w", err)
	}

	pool, err := pgxpool.ConnectConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("failed to create connection pool: %w", err)
	}

	txDuration := promauto.With(reg).NewHistogramVec(prometheus.HistogramOpts{
		Name: "database_transaction_duration_seconds",
		Help: "A histogram of durations of database transactions.",
	}, []string{"result"})

	return &Client{
		pool: pool,
		metrics: &metrics{
			txDuration: txDuration,
		},
	}, nil
}

// Close the underlying db connection(s).
func (c *Client) Close() {
	c.pool.Close()
}

func (c *Client) ExecuteTx(ctx context.Context, txOptions pgx.TxOptions, txFunc func(tx pgx.Tx) error) error {
	now := time.Now()
	result := "success"
	err := crdbpgx.ExecuteTx(ctx, c.pool, txOptions, txFunc)
	if err != nil {
		result = "error"
	}
	c.metrics.txDuration.WithLabelValues(result).Observe(time.Since(now).Seconds())
	return err
}
