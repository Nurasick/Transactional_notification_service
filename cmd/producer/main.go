package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/Nurasick/transactional-notification-service/internal/broker"
	"github.com/Nurasick/transactional-notification-service/internal/config"
	"github.com/Nurasick/transactional-notification-service/internal/store"
	"github.com/google/uuid"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	conn, err := broker.Connect(cfg.RabbitMQ.URL)
	if err != nil {
		log.Fatalf("rabbitmq: %v", err)
	}
	defer conn.Close()

	notifLog, err := store.New(cfg.Postgres.DSN)
	if err != nil {
		log.Fatalf("postgres: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/publish", publishHandler(conn))
	mux.HandleFunc("/notifications", listHandler(notifLog))

	srv := &http.Server{Addr: ":8080", Handler: mux}

	go func() {
		log.Println("[PRODUCER] listening on :8080")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("http: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("[PRODUCER] shutting down")
	srv.Shutdown(context.Background())
}

type publishRequest struct {
	EventType string         `json:"event_type"`
	UserID    string         `json:"user_id"`
	Payload   map[string]any `json:"payload"`
}

func publishHandler(conn *broker.Connection) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req publishRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		if req.EventType == "" || req.UserID == "" {
			http.Error(w, "event_type and user_id are required", http.StatusBadRequest)
			return
		}

		event := broker.Event{
			MessageID: uuid.NewString(),
			EventType: req.EventType,
			UserID:    req.UserID,
			Payload:   req.Payload,
		}

		if err := conn.Publish(r.Context(), event); err != nil {
			log.Printf("[PRODUCER] publish error: %v", err)
			http.Error(w, "failed to publish", http.StatusInternalServerError)
			return
		}

		log.Printf("[PRODUCER] published event=%s user=%s message_id=%s", event.EventType, event.UserID, event.MessageID)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"message_id": event.MessageID})
	}
}

func listHandler(notifLog *store.NotificationLog) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		q := r.URL.Query()
		limit, _ := strconv.Atoi(q.Get("limit"))
		offset, _ := strconv.Atoi(q.Get("offset"))

		filter := store.ListFilter{
			UserID:    q.Get("user_id"),
			Status:    q.Get("status"),
			EventType: q.Get("event_type"),
			Limit:     limit,
			Offset:    offset,
		}

		records, err := notifLog.List(r.Context(), filter)
		if err != nil {
			log.Printf("[API] list error: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"data":   records,
			"count":  len(records),
			"offset": offset,
		})
	}
}
