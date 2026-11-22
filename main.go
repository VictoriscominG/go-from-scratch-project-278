package main

import (
	"link-shortener/internal/server"
	"log"
	"os"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	if err := server.New().Run(":" + port); err != nil {
		log.Fatal(err)
	}
}
