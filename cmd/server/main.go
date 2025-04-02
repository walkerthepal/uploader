package main

import (
	"log"
	"net/http"

	"uploader/internal/config"
	"uploader/internal/handlers"
	"uploader/internal/middleware"

	"github.com/go-chi/chi/v5"
)

func main() {
	// Initialize the configuration
	_, err := config.Load("creds.json")
	if err != nil {
		// Try looking for credentials in the project root if running from cmd/server
		log.Printf("Failed to load configuration from current directory: %v", err)
		log.Printf("Attempting to load from project root...")
		
		_, err = config.Load("../../creds.json")
		if err != nil {
			log.Fatalf("Failed to load configuration: %v", err)
		}
	}

	// Create a new router
	r := chi.NewRouter()
	
	// Apply middleware
	r.Use(middleware.Logger)

	// Register routes
	r.Get("/", handlers.ShowHomePage)
	r.Get("/login/youtube", handlers.HandleYoutubeLogin)
	r.Get("/login/instagram", handlers.HandleInstagramLogin)
	r.Get("/callback/youtube", handlers.HandleYoutubeCallback)
	r.Get("/callback/instagram", handlers.HandleInstagramCallback)
	r.Get("/upload", handlers.ShowUploadPage)
	r.Post("/upload", handlers.HandleUpload)

	// Start the server
	log.Println("Server is running on :3000")
	http.ListenAndServe(":3000", r)
}
