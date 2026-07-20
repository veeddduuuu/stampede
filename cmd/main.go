package main

import (
	"concurrent-seat-booking-system/internal/adapters/postgres"
	"concurrent-seat-booking-system/internal/adapters/redis"
	"concurrent-seat-booking-system/internal/booking"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gorilla/mux"
)

func main() {
	_ = context.Background()
	pool := postgres.NewPostgresPool()
	defer pool.Close()

	rds := redis.NewRedisClient("localhost:6379")
	defer rds.Close()

	repo := booking.NewHybridStore(pool, rds)
	svc := booking.NewService(repo)

	r := mux.NewRouter()

	r.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}).Methods(http.MethodGet)

	r.HandleFunc("/users/{id}/bookings", func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		userID := vars["id"]

		bookings, err := svc.ListBookings(userID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(bookings); err != nil {
			log.Printf("Error encoding response: %v", err)
		}
	}).Methods(http.MethodGet)

	r.HandleFunc("/events/{id}/book", func(w http.ResponseWriter, r *http.Request) {
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

		err := svc.Book(b)
		if err != nil {
			if err.Error() == "seat is already booked for this event" || err.Error() == "seat already booked" {
				http.Error(w, err.Error(), http.StatusConflict)
			} else {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"message": "booking successful"}`))
	}).Methods(http.MethodPost)


	
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
		log.Fatal("Server forced to shutdown:", err)
	}

	log.Println("Server exiting")
}