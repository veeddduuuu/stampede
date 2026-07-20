package booking

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

const redisStoreTTL = 1200 * time.Millisecond

type RedisStore struct {
	rbd *redis.Client
}

func NewRedisStore(rbd *redis.Client) *RedisStore{
	return &RedisStore{
		rbd:rbd,
	}
}

func SessionKey(id string) string {
	return fmt.Sprintf("session:%s", id)
}

func (s* RedisStore) Book(b Booking) error {
	booking, err := s.hold(b)
	if err != nil {
		return err
	}
	log.Printf("Booking held for user %s: %+v", b.UserID, booking)
	return nil
}

func (s *RedisStore) hold(b Booking) (*Booking, error){
	id:= uuid.New().String()
	now:= time.Now()
	expiresAt:= now.Add(redisStoreTTL)
	ctx := context.Background()
	key:= fmt.Sprintf("seat:%s:%s", b.EventID, b.SeatID)
	b.ID = id
	val, _ := json.Marshal(b)

	res:= s.rbd.SetArgs(ctx, key, val, redis.SetArgs{
		Mode: "NX",
		TTL: redisStoreTTL,
	})

	ok:= res.Val() == "OK"
	if !ok {
		return nil, errors.New("seat already booked")
	}

	return &Booking{
		ID: id,
		UserID: b.UserID,
		SeatID: b.SeatID,
		EventID: b.EventID,
		Status: "HELD",
		ExpiresAt: expiresAt,
	}, nil
}