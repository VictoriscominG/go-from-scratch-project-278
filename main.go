package main

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/joho/godotenv"

	"link-shortener/internal/server"
	"link-shortener/internal/storage"
)

func main() {
	_ = godotenv.Load() // локально подхватит .env, в проде файла нет — и ладно

	ctx := context.Background()

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	baseURL := os.Getenv("BASE_URL")
	if baseURL == "" {
		baseURL = "http://localhost:" + port
	}

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Fatal("DATABASE_URL is required")
	}

	if sentryDSN := os.Getenv("SENTRY_DSN"); sentryDSN != "" {
		if err := sentry.Init(sentry.ClientOptions{Dsn: sentryDSN}); err != nil {
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
	if err := server.New(store, baseURL).Run(":" + port); err != nil {
		log.Fatal(err)
	}
}
