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

func main() {
	_ = context.Background()
	pool := postgres.NewPostgresPool()
	defer pool.Close()

	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}
	rds := redis.NewRedisClient(redisAddr)
	defer rds.Close()

	repo := booking.NewHybridStore(pool, rds)
	svc := booking.NewService(repo)

	r := routes(svc)

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
