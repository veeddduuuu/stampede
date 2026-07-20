package postgres

import(
	"context"
	"os"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
)

// We use the docker-compose credentials you set up
const connStr = "postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable"

func NewPostgresPool() *pgxpool.Pool {
	config, err := pgxpool.ParseConfig(connStr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to parse connection string: %v\n", err)
		os.Exit(1)
	}

	// Connection Pooling Configuration
	config.MaxConns = 50
	config.MinConns = 10

	dbpool, err := pgxpool.NewWithConfig(context.Background(), config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to create connection pool: %v\n", err)
		os.Exit(1)
	}
	
	return dbpool
}
