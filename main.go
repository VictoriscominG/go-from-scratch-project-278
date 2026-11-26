package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/joho/godotenv"

	"link-shortener/internal/server"
	"link-shortener/internal/storage"
)

type config struct {
	port      string
	baseURL   string
	dsn       string
	sentryDSN string
}

func loadConfig() (config, error) {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	if _, err := strconv.Atoi(port); err != nil {
		return config{}, fmt.Errorf("invalid PORT %q: must be a number", port)
	}

	baseURL := os.Getenv("BASE_URL")
	if baseURL == "" {
		baseURL = "http://localhost:" + port
	}

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return config{}, errors.New("DATABASE_URL is required")
	}

	return config{
		port:      port,
		baseURL:   baseURL,
		dsn:       dsn,
		sentryDSN: os.Getenv("SENTRY_DSN"),
	}, nil
}

func main() {
	_ = godotenv.Load()

	cfg, err := loadConfig()
	if err != nil {
		log.Fatal(err)
	}

	ctx := context.Background()

	if cfg.sentryDSN != "" {
		if err := sentry.Init(sentry.ClientOptions{Dsn: cfg.sentryDSN}); err != nil {
			log.Fatalf("sentry init: %v", err)
		}
		defer sentry.Flush(2 * time.Second)
	}

	store, err := storage.NewPostgres(ctx, cfg.dsn)
	if err != nil {
		log.Fatalf("db connect: %v", err)
	}
	defer store.Close()

	log.Printf("listening on :%s", cfg.port)
	if err := server.New(store, cfg.baseURL).Run(":" + cfg.port); err != nil {
		log.Fatal(err)
	}
}
