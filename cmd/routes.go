package main

import (
	"concurrent-seat-booking-system/internal/booking"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)


func routes(svc *booking.Service) *chi.Mux {
	h := &APIHandler{svc: svc}
	r := chi.NewRouter()
    r.Use(middleware.Logger)

	r.Get("/healthz", h.healthz)
	r.Get("/users/{id}/bookings", h.listBookings)
	r.Post("/events/{id}/book", h.bookSeat)
	r.Post("/events/{id}/hold", h.holdSeat)
	r.Get("/events/{id}/seats", h.listSeats)
	r.Post("/events/{id}/release", h.releaseSeat)

	return r
}