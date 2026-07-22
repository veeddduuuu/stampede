package main

import (
	"concurrent-seat-booking-system/internal/booking"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/mux"
)

type APIHandler struct {
	svc *booking.Service
}

func (h *APIHandler) healthz(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

func (h *APIHandler) listBookings(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	userID := vars["id"]

	bookings, err := h.svc.ListBookings(userID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(bookings); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

func (h *APIHandler) bookSeat(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	eventID := vars["id"]

	var req struct {
		SeatID string `json:"seat_id"`
		UserID string `json:"user_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request payload", http.StatusBadRequest)
		return
	}

	b := booking.Booking{
		EventID: eventID,
		SeatID:  req.SeatID,
		UserID:  req.UserID,
	}

	err := h.svc.Book(b)
	if err != nil {
		if errors.Is(err, booking.ErrSeatAlreadyBooked) || errors.Is(err, booking.ErrHoldExpired) || errors.Is(err, booking.ErrUnauthorizedHold) {
			http.Error(w, err.Error(), http.StatusConflict)
		} else {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	w.Write([]byte(`{"message": "booking successful"}`))
}

func (h *APIHandler) holdSeat(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	eventID := vars["id"]

	var req struct {
		SeatID string `json:"seat_id"`
		UserID string `json:"user_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request payload", http.StatusBadRequest)
		return
	}

	b := booking.Booking{
		EventID: eventID,
		SeatID:  req.SeatID,
		UserID:  req.UserID,
	}

	held, err := h.svc.Hold(b)
	if err != nil {
		if errors.Is(err, booking.ErrSeatAlreadyBooked) {
			http.Error(w, err.Error(), http.StatusConflict)
		} else {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)

	resp := struct {
		Message   string `json:"message"`
		ExpiresAt string `json:"expires_at"`
	}{
		Message:   "seat held successfully",
		ExpiresAt: held.ExpiresAt.Format(time.RFC3339),
	}

	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

func (h *APIHandler) listSeats(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	eventID := vars["id"]

	seats, err := h.svc.ListSeats(eventID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(seats); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

func (h *APIHandler) releaseSeat(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	eventID := vars["id"]

	var req struct {
		SeatID string `json:"seat_id"`
		UserID string `json:"user_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request payload", http.StatusBadRequest)
		return
	}

	b := booking.Booking{
		EventID: eventID,
		SeatID:  req.SeatID,
		UserID:  req.UserID,
	}

	held, err := h.svc.Release(b)
	if err != nil {
		if errors.Is(err, booking.ErrSeatNotHeld) || errors.Is(err, booking.ErrUnauthorizedHold) {
			http.Error(w, err.Error(), http.StatusConflict)
		} else {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)

	resp := struct {
		Message   string `json:"message"`
		ExpiresAt string `json:"expires_at"`
	}{
		Message:   "seat released successfully",
		ExpiresAt: held.ExpiresAt.Format(time.RFC3339),
	}

	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}