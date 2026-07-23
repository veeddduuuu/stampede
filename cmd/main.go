package main

import (
	"concurrent-seat-booking-system/internal/adapters/postgres"
	"concurrent-seat-booking-system/internal/adapters/redis"
	"concurrent-seat-booking-system/internal/booking"
	"concurrent-seat-booking-system/internal/websocket"
	"context"
	"log"
	"net/http"
	"net/http/pprof"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	goredis "github.com/redis/go-redis/v9"
)

func routes(hub *websocket.Hub, svc *booking.Service, rds *goredis.Client) *chi.Mux {
	h := &APIHandler{svc: svc, rds: rds}
	r := chi.NewRouter()
	r.Use(middleware.Logger)

	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"https://stampede-phi.vercel.app", "https://stampede-go.duckdns.org", "http://localhost:*", "http://localhost:5173", "http://localhost:3000"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
		AllowCredentials: true,
	}))

	// Profiling endpoints
	r.HandleFunc("/debug/pprof/", pprof.Index)
	r.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	r.HandleFunc("/debug/pprof/profile", pprof.Profile)
	r.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	r.HandleFunc("/debug/pprof/trace", pprof.Trace)

	r.Get("/healthz", h.healthz)
	r.Get("/users/{id}/bookings", h.listBookings)
	r.Post("/events/{id}/book", h.bookSeat)
	r.Post("/events/{id}/hold", h.holdSeat)
	r.Get("/events/{id}/seats", h.listSeats)
	r.Post("/events/{id}/release", h.releaseSeat)

	// WebSocket endpoint
	r.Get("/ws", func(w http.ResponseWriter, r *http.Request) {
		websocket.ServeWS(hub, w, r)
	})

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

	// Initialize WebSocket Hub & start event loop BEFORE starting HTTP server
	hub := websocket.NewHub()
	go hub.ListenToRedis(context.Background(), rds)
	go hub.Run()

	r := routes(hub, svc, rds)

	srv := &http.Server{
		Addr:        ":8080",
		Handler:     r,
		ReadTimeout: 10 * time.Second,
		// WriteTimeout must be 0 for long-lived WebSocket connections,
		// as a non-zero WriteTimeout will force-close WebSockets after N seconds.
		WriteTimeout: 0,
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
