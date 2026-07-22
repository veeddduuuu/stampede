package websocket

import (
	"encoding/json"
)

type EventType string

const (
	EventSeatHeld     EventType = "SEAT_HELD"
	EventSeatBooked   EventType = "SEAT_BOOKED"
	EventSeatReleased EventType = "SEAT_RELEASED"
)

type SeatUpdateEvent struct {
	Type       EventType `json:"type"`
	EventID    string    `json:"event_id"`
	SeatID     string    `json:"seat_id"`
	Status     string    `json:"status"`
	HeldBy     string    `json:"held_by,omitempty"`
	TTLSeconds int       `json:"ttl_seconds,omitempty"`
	Timestamp  int64     `json:"timestamp"`
}

func (e *SeatUpdateEvent) ToJSON() ([]byte, error) {
	return json.Marshal(e)
}