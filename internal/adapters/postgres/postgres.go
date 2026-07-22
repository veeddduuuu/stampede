package postgres

import (
	"context"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// We use the docker-compose credentials you set up
var connStr = "postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable"

func NewPostgresPool() *pgxpool.Pool {
	if envURL := os.Getenv("POSTGRES_URL"); envURL != "" {
		connStr = envURL
	}
	config, err := pgxpool.ParseConfig(connStr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to parse connection string: %v\n", err)
		os.Exit(1)
	}

	// Connection Pooling Configuration
	config.MaxConns = 50
	config.MinConns = 10

	// Force IPv4 dialing to avoid IPv6 unreachable errors in Docker containers
	dialer := &net.Dialer{
		Timeout: 10 * time.Second,
	}
	config.ConnConfig.DialFunc = func(ctx context.Context, network, addr string) (net.Conn, error) {
		return dialer.DialContext(ctx, "tcp4", addr)
	}

	dbpool, err := pgxpool.NewWithConfig(context.Background(), config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to create connection pool: %v\n", err)
		os.Exit(1)
	}

	return dbpool
}
