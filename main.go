package main

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/getsentry/sentry-go"

	"link-shortener/internal/server"
	"link-shortener/internal/storage"
)

func main() {
	ctx := context.Background()

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Fatal("DATABASE_URL is required")
	}

	// Мониторинг ошибок: если SENTRY_DSN не задан, просто работаем без него
	if sentryDSN := os.Getenv("SENTRY_DSN"); sentryDSN != "" {
		if err := sentry.Init(sentry.ClientOptions{
			Dsn: sentryDSN,
		}); err != nil {
			log.Fatalf("sentry init: %v", err)
		}
		defer sentry.Flush(2 * time.Second)
	}

	store, err := storage.NewPostgres(ctx, dsn)
	if err != nil {
		log.Fatalf("db connect: %v", err)
	}
	defer store.Close()

	log.Printf("listening on :%s", port)
	if err := server.New(store).Run(":" + port); err != nil {
		log.Fatal(err)
	}
}
