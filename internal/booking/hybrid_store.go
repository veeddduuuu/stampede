package booking

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

const defaultHoldTTL = 3 * time.Minute

type HybridStore struct {
	pool *pgxpool.Pool
	rds  *redis.Client
}

func NewHybridStore(pool *pgxpool.Pool, rds *redis.Client) *HybridStore {
	return &HybridStore{
		pool: pool,
		rds:  rds,
	}
}

func UniqueKey(eventId string, seatId string) string {
	return fmt.Sprintf("seat:%s:%s", eventId, seatId)
}

func (s *HybridStore) Hold(b Booking) (*Booking, error) {
	ctx := context.Background()
	id := uuid.New().String()
	now := time.Now()
	expiresAt := now.Add(defaultHoldTTL)
	val, _ := json.Marshal(b)
	res := s.rds.SetArgs(ctx, UniqueKey(b.EventID, b.SeatID), val, redis.SetArgs{
		Mode: "NX",
		TTL:  defaultHoldTTL,
	})
	ok := res.Val() == "OK"
	if !ok {
		return nil, errors.New("seat already booked")
	}
	return &Booking{
		ID:        id,
		UserID:    b.UserID,
		SeatID:    b.SeatID,
		EventID:   b.EventID,
		Status:    "HELD",
		ExpiresAt: expiresAt,
	}, nil
}

func (s *HybridStore) Book(b Booking) error {
	ctx := context.Background()
	held, err := s.Hold(b)
	if err != nil {
		return fmt.Errorf("unable to hold seat the seat is already booked %w", err)
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		s.rds.Del(ctx, UniqueKey(held.EventID, held.SeatID))
		return fmt.Errorf("unable to begin transaction %w", err)	
	}
	defer tx.Rollback(ctx)
	query := `
		INSERT INTO bookings (id, event_id, seat_id, user_id, status)
		VALUES ($1, $2, $3, $4, $5)
	`
	_, err = tx.Exec(ctx, query, held.ID, held.EventID, held.SeatID, held.UserID, held.Status) 
	if err != nil {
		s.rds.Del(ctx, UniqueKey(held.EventID, held.SeatID))
		// Check if the error is a Postgres Unique Violation (code 23505)
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return errors.New("seat is already booked for this event")
		}
		return err
	}
	err = tx.Commit(ctx)
	if err != nil {
		s.rds.Del(ctx, UniqueKey(held.EventID, held.SeatID))
		return err
	}
	s.rds.Del(ctx, UniqueKey(held.EventID, held.SeatID))
	return nil
}
