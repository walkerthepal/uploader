package main

import (
	"log"
	"net/http"
	"os"
	"path/filepath"

	"uploader/internal/config"
	"uploader/internal/handlers"
	"uploader/internal/middleware"

	"github.com/go-chi/chi/v5"
)

func main() {
	// --- Enhanced Debugging ---
	cwd, err := os.Getwd()
	if err != nil {
		log.Fatalf("FATAL: Failed to get current working directory: %v", err)
	}
	log.Printf("DEBUG: Current Working Directory (CWD): %s", cwd)

	// Explicitly define the relative path we expect to work
	relativePath := "creds.json"
	log.Printf("DEBUG: Relative path being checked: %s", relativePath)

	// Resolve the absolute path based on the CWD
	absPath, err := filepath.Abs(relativePath)
	if err != nil {
		log.Printf("WARN: Could not resolve absolute path for '%s': %v", relativePath, err)
		absPath = "[unknown]" // Assign a placeholder if Abs fails
	}
	log.Printf("DEBUG: Expected absolute path: %s", absPath)

	// Use os.Stat to check file existence and permissions directly
	log.Printf("DEBUG: Running os.Stat on relative path '%s'...", relativePath)
	fileInfo, statErr := os.Stat(relativePath)

	if statErr != nil {
		log.Printf("DEBUG: os.Stat FAILED: %v", statErr) // Log the specific error from os.Stat

		// Check the type of error
		if os.IsNotExist(statErr) {
			log.Printf("DEBUG: os.Stat confirms file does NOT exist at relative path '%s'.", relativePath)
		} else if os.IsPermission(statErr) {
			log.Printf("DEBUG: os.Stat confirms PERMISSION DENIED for relative path '%s'.", relativePath)
		} else {
			log.Printf("DEBUG: os.Stat failed with an unexpected error type for relative path '%s'.", relativePath)
		}

		// Also try stating the absolute path IF we could resolve it
		if absPath != "[unknown]" {
			log.Printf("DEBUG: Running os.Stat on absolute path '%s'...", absPath)
			_, statErrAbs := os.Stat(absPath)
			if statErrAbs != nil {
				log.Printf("DEBUG: os.Stat on absolute path FAILED: %v", statErrAbs)
			} else {
				log.Printf("DEBUG: os.Stat on absolute path SUCCEEDED.") // This would be very strange if relative failed
			}
		}

	} else {
		// This case should NOT happen based on your error, but good to have
		log.Printf("DEBUG: os.Stat SUCCEEDED for relative path '%s'. File size: %d, Mode: %s", relativePath, fileInfo.Size(), fileInfo.Mode())
	}
	// --- End Enhanced Debugging ---

	// Now, attempt the load again (which we expect to fail based on prior runs)
	log.Printf("Attempting to load configuration directly from: %s", relativePath)
	_, err = config.Load(relativePath) // Use the relativePath variable
	if err != nil {
		// Log the final failure reason
		log.Fatalf("Failed to load configuration from '%s': %v", relativePath, err)
	}

	// Code below here likely won't be reached
	log.Println("Successfully loaded configuration.")

	// Create a new router
	r := chi.NewRouter()

	// Apply middleware
	r.Use(middleware.Logger)

	// Register routes
	r.Get("/", handlers.ShowHomePage)

	// Legal pages
	r.Get("/terms", handlers.ShowTermsPage)
	r.Get("/privacy", handlers.ShowPrivacyPage)
	r.Get("/data-removal", handlers.ShowDataRemovalPage)

	// Authentication routes
	r.Get("/login/youtube", handlers.HandleYoutubeLogin)
	r.Get("/login/instagram", handlers.HandleInstagramLogin)
	r.Get("/login/tiktok", handlers.HandleTikTokLogin)
	r.Get("/callback/youtube", handlers.HandleYoutubeCallback)
	r.Get("/callback/instagram", handlers.HandleInstagramCallback)
	r.Get("/callback/tiktok", handlers.HandleTikTokCallback)

	// Upload routes
	r.Get("/upload", handlers.ShowUploadPage)
	r.Post("/upload", handlers.HandleUpload)

	// Serve static files
	fileServer := http.FileServer(http.Dir("./static"))
	r.Handle("/static/*", http.StripPrefix("/static/", fileServer))

	// Start the server
	log.Println("Server is running on :3000")
	http.ListenAndServe(":3000", r)
}
