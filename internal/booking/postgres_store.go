package booking

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/pgconn"
)

type PostgresStore struct {
	pool *pgxpool.Pool
}

func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore {
	return &PostgresStore{
		pool: pool,
	}
}

func (s *PostgresStore) Book(b Booking) error {
	ctx := context.Background()

	// Start a transaction
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	
	query := `
		INSERT INTO bookings (id, event_id, seat_id, user_id, status)
		VALUES ($1, $2, $3, $4, $5)
	`
	_, err = tx.Exec(ctx, query, b.ID, b.EventID, b.SeatID, b.UserID, b.Status)
	if err != nil {
		// Check if the error is a Postgres Unique Violation (code 23505)
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return errors.New("seat is already booked for this event")
		}
		return err
	}
	return tx.Commit(ctx)
}

func (s *PostgresStore) ListBookings(id string) []Booking {
	return nil
}
