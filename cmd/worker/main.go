package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/Nurasick/transactional-notification-service/internal/broker"
	"github.com/Nurasick/transactional-notification-service/internal/config"
	"github.com/Nurasick/transactional-notification-service/internal/consumer"
	"github.com/Nurasick/transactional-notification-service/internal/dedup"
	"github.com/Nurasick/transactional-notification-service/internal/notification"
	"github.com/Nurasick/transactional-notification-service/internal/ratelimit"
	"github.com/Nurasick/transactional-notification-service/internal/store"
	tmplstore "github.com/Nurasick/transactional-notification-service/internal/template"
	"github.com/redis/go-redis/v9"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	// RabbitMQ
	conn, err := broker.Connect(cfg.RabbitMQ.URL)
	if err != nil {
		log.Fatalf("rabbitmq: %v", err)
	}
	defer conn.Close()

	// Redis
	redisClient := redis.NewClient(&redis.Options{Addr: cfg.Redis.Addr})

	// PostgreSQL
	notifLog, err := store.New(cfg.Postgres.DSN)
	if err != nil {
		log.Fatalf("postgres: %v", err)
	}

	// MinIO templates
	templates, err := tmplstore.New(
		cfg.Minio.Endpoint,
		cfg.Minio.AccessKey,
		cfg.Minio.SecretAccessKey,
		cfg.Minio.BucketName,
		cfg.Minio.UseSSL,
	)
	if err != nil {
		log.Fatalf("minio: %v", err)
	}

	// Wire up dispatcher
	dispatcher := notification.NewDispatcher(
		dedup.New(redisClient),
		ratelimit.New(redisClient),
		templates,
		notification.NewLogSender(),
		notifLog,
	)

	c := consumer.New(conn.Channel, dispatcher)

	ctx, cancel := context.WithCancel(context.Background())

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-quit
		log.Println("[WORKER] shutting down")
		cancel()
	}()

	if err := c.Run(ctx); err != nil {
		log.Fatalf("consumer: %v", err)
	}
}
