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
	key := UniqueKey(b.EventID, b.SeatID)

	val, err := s.rds.Get(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return errors.New("seat hold expired or does not exist")
		}
		return fmt.Errorf("error checking hold in redis: %w", err)
	}

	var held Booking
	if err := json.Unmarshal([]byte(val), &held); err != nil {
		return fmt.Errorf("error parsing hold data: %w", err)
	}

	if held.UserID != b.UserID {
		return errors.New("seat is held by another user")
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("unable to begin transaction %w", err)	
	}
	defer tx.Rollback(ctx)
	query := `
		INSERT INTO bookings (id, event_id, seat_id, user_id, status)
		VALUES ($1, $2, $3, $4, $5)
	`
	_, err = tx.Exec(ctx, query, held.ID, held.EventID, held.SeatID, held.UserID, "BOOKED") 
	if err != nil {
		// Check if the error is a Postgres Unique Violation (code 23505)
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return errors.New("seat is already booked for this event")
		}
		return err
	}
	err = tx.Commit(ctx)
	if err != nil {
		return err
	}
	s.rds.Del(ctx, key)
	return nil
}

func (s *HybridStore) ListBookings(userID string) ([]Booking, error) {
	ctx := context.Background()
	query := `
		SELECT id, event_id, seat_id, user_id, status
		FROM bookings
		WHERE user_id = $1
	`
	rows, err := s.pool.Query(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var bookings []Booking
	for rows.Next() {
		var b Booking
		if err := rows.Scan(&b.ID, &b.EventID, &b.SeatID, &b.UserID, &b.Status); err != nil {
			return nil, err
		}
		bookings = append(bookings, b)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return bookings, nil
}

func (s *HybridStore) ListEventBookings(eventID string) ([]Booking, error) {
	ctx := context.Background()
	var bookings []Booking

	// 1. Fetch BOOKED seats from Postgres
	query := `
		SELECT id, event_id, seat_id, user_id, status
		FROM bookings
		WHERE event_id = $1
	`
	rows, err := s.pool.Query(ctx, query, eventID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var b Booking
		if err := rows.Scan(&b.ID, &b.EventID, &b.SeatID, &b.UserID, &b.Status); err != nil {
			return nil, err
		}
		bookings = append(bookings, b)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// 2. Fetch HELD seats from Redis
	// Using KEYS for simplicity, though SCAN is better for production
	pattern := fmt.Sprintf("seat:%s:*", eventID)
	keys, err := s.rds.Keys(ctx, pattern).Result()
	if err != nil && err != redis.Nil {
		return nil, err
	}

	if len(keys) > 0 {
		values, err := s.rds.MGet(ctx, keys...).Result()
		if err != nil && err != redis.Nil {
			return nil, err
		}
		for _, val := range values {
			if val == nil {
				continue
			}
			strVal, ok := val.(string)
			if !ok {
				continue
			}
			var b Booking
			if err := json.Unmarshal([]byte(strVal), &b); err == nil {
				// We don't overwrite Status here because Hold already sets it to "HELD" before marshaling,
				// but let's ensure it is set correctly just in case.
				b.Status = "HELD"
				bookings = append(bookings, b)
			}
		}
	}

	return bookings, nil
}

func (s *HybridStore) Release(b Booking) (*Booking, error) {
	ctx := context.Background()
	key := UniqueKey(b.EventID, b.SeatID)
	val, err := s.rds.Get(ctx, key).Result()
	if err != nil {
		return nil, err
	}
	if val == "" {
		return nil, errors.New("seat is not on hold")
	}
	err = s.rds.Del(ctx, key).Err()
	if err != nil {
		return nil, err
	}
	return &Booking{
		ID:        b.ID,
		EventID:   b.EventID,
		SeatID:    b.SeatID,
		UserID:    b.UserID,
		Status:    "AVAILABLE",
		ExpiresAt: b.ExpiresAt,
	}, nil
}