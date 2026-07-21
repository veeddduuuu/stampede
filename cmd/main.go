package main

import (
	"concurrent-seat-booking-system/internal/adapters/postgres"
	"concurrent-seat-booking-system/internal/adapters/redis"
	"concurrent-seat-booking-system/internal/booking"
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gorilla/mux"
)

func setupRouter(svc *booking.Service) *mux.Router {
	h := &APIHandler{svc: svc}
	r := mux.NewRouter()

	r.HandleFunc("/healthz", h.healthz).Methods(http.MethodGet)
	r.HandleFunc("/users/{id}/bookings", h.listBookings).Methods(http.MethodGet)
	r.HandleFunc("/events/{id}/book", h.bookSeat).Methods(http.MethodPost)
	r.HandleFunc("/events/{id}/hold", h.holdSeat).Methods(http.MethodPost)
	r.HandleFunc("/events/{id}/seats", h.listSeats).Methods(http.MethodGet)

	return r
}

func main() {
	_ = context.Background()
	pool := postgres.NewPostgresPool()
	defer pool.Close()

	rds := redis.NewRedisClient("localhost:6379")
	defer rds.Close()

	repo := booking.NewHybridStore(pool, rds)
	svc := booking.NewService(repo)

	r := setupRouter(svc)
	srv := &http.Server{
		Addr:         ":8080",
		Handler:      r,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		log.Println("Starting API server on :8080")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed to start: %v", err)
		}
	}()

	// Wait for interrupt signal to gracefully shutdown the server
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down server...")

	// The context is used to inform the server it has 10 seconds to finish
	// the request it is currently handling
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("Server forced to shutdown: %v", err)
	}

	log.Println("Server exiting")
}