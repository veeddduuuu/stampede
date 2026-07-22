package booking

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

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

func (s *HybridStore) GetHold(eventID string, seatID string) (*Booking, error) {
	ctx := context.Background()
	key := UniqueKey(eventID, seatID)

	val, err := s.rds.Get(ctx, key).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, ErrHoldExpired
		}
		return nil, fmt.Errorf("error checking hold in redis: %w", err)
	}

	var held Booking
	if err := json.Unmarshal([]byte(val), &held); err != nil {
		return nil, fmt.Errorf("error parsing hold data: %w", err)
	}

	return &held, nil
}

func (s *HybridStore) Hold(b Booking) (*Booking, error) {
	ctx := context.Background()
	ttl := time.Until(b.ExpiresAt)
	if ttl <= 0 {
		ttl = 3 * time.Minute
	}

	val, err := json.Marshal(b)
	if err != nil {
		return nil, fmt.Errorf("error marshaling hold data: %w", err)
	}

	res := s.rds.SetArgs(ctx, UniqueKey(b.EventID, b.SeatID), val, redis.SetArgs{
		Mode: "NX",
		TTL:  ttl,
	})
	if res.Val() != "OK" {
		return nil, ErrSeatAlreadyBooked
	}
	return &b, nil
}

func (s *HybridStore) Book(b Booking) error {
	ctx := context.Background()
	key := UniqueKey(b.EventID, b.SeatID)

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("unable to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	query := `
		INSERT INTO bookings (id, event_id, seat_id, user_id, status)
		VALUES ($1, $2, $3, $4, $5)
	`
	_, err = tx.Exec(ctx, query, b.ID, b.EventID, b.SeatID, b.UserID, "BOOKED")
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return ErrSeatAlreadyBooked
		}
		return err
	}

	if err := tx.Commit(ctx); err != nil {
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

	deleted, err := s.rds.Del(ctx, key).Result()
	if err != nil {
		return nil, err
	}
	if deleted == 0 {
		return nil, ErrSeatNotHeld
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