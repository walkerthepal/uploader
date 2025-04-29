package handlers

import (
	"bytes"
	"html/template"
	"log"
	"net/http"

	"uploader/internal/models"
	"uploader/internal/services"
)

var templates *template.Template

func init() {
	// Load templates
	var err error
	templates, err = template.ParseGlob("templates/*.html")
	if err != nil {
		log.Fatalf("Failed to parse templates: %v", err)
	}
}

// ShowHomePage displays the home page
func ShowHomePage(w http.ResponseWriter, r *http.Request) {
	templates.ExecuteTemplate(w, "index.html", nil)
}

// ShowTermsPage displays the terms of service page
func ShowTermsPage(w http.ResponseWriter, r *http.Request) {
	templates.ExecuteTemplate(w, "terms.html", nil)
}

// ShowPrivacyPage displays the privacy policy page
func ShowPrivacyPage(w http.ResponseWriter, r *http.Request) {
	templates.ExecuteTemplate(w, "privacy.html", nil)
}

// ShowUploadPage displays the upload form
func ShowUploadPage(w http.ResponseWriter, r *http.Request) {
	templates.ExecuteTemplate(w, "upload.html", nil)
}

// HandleUpload processes the upload form submission
func HandleUpload(w http.ResponseWriter, r *http.Request) {
	result := &models.UploadResult{} // Initialize result struct

	// Parse the multipart form (consider increasing max memory if handling very large uploads simultaneously)
	// Using 32MB here allows form values up to 32MB in memory, file parts are streamed.
	err := r.ParseMultipartForm(32 << 20) // 32MB max memory for non-file parts
	if err != nil {
		log.Printf("Failed to parse multipart form: %v", err)
		http.Error(w, "Failed to parse form (file might be too large or form malformed)", http.StatusBadRequest)
		return
	}

	// Get the platforms selected for upload
	platforms := r.Form["platforms"] // Slice like ["youtube", "tiktok"]
	if len(platforms) == 0 {
		http.Error(w, "No platforms selected for upload", http.StatusBadRequest)
		return
	}

	// Create a map to easily check selected platforms in the template
	selectedPlatformsMap := make(map[string]bool)
	for _, p := range platforms {
		selectedPlatformsMap[p] = true
	}

	// Get the uploaded video file
	file, header, err := r.FormFile("video")
	if err != nil {
		log.Printf("Failed to get video file from form: %v", err)
		http.Error(w, "Failed to get video file: please ensure a valid video file is selected", http.StatusBadRequest)
		return
	}
	defer file.Close() // Ensure the file part is closed

	// Log file details
	log.Printf("Received file: %s, size: %d bytes", header.Filename, header.Size)

	// Get the main caption
	mainCaption := r.FormValue("mainCaption")

	// Get the request context to pass down to service functions
	ctx := r.Context()

	// Handle uploads to selected platforms
	for _, platform := range platforms {
		// Reset file position before each platform upload attempt
		// This is crucial because each service function will read the file.
		if _, err := file.Seek(0, 0); err != nil {
			log.Printf("CRITICAL: Failed to reset file position before uploading to %s: %v", platform, err)
			// Record an error for this platform and skip it
			switch platform {
			case "youtube":
				result.YouTube.Success = false
				result.YouTube.Error = "Internal server error: failed to prepare file for upload"
			case "instagram":
				result.Instagram.Success = false
				result.Instagram.Error = "Internal server error: failed to prepare file for upload"
			case "tiktok":
				result.TikTok.Success = false
				result.TikTok.Error = "Internal server error: failed to prepare file for upload"
			}
			continue // Skip to the next platform
		}

		// Process upload for the current platform
		switch platform {
		case "youtube":
			title := r.FormValue("youtubeTitle")
			if title == "" {
				log.Printf("Skipping YouTube upload: Title is required.")
				result.YouTube.Success = false
				result.YouTube.Error = "YouTube title is required"
				continue // Skip YouTube upload if title missing
			}
			description := r.FormValue("youtubeDescription")
			// Pass context if/when UploadToYoutube supports it
			err := services.UploadToYoutube(file, header, title, description, mainCaption, result)
			if err != nil {
				log.Printf("YouTube upload failed: %v", err)
				// Error details are already set within UploadToYoutube (or should be)
			}
		case "instagram":
			instagramCaption := r.FormValue("instagramCaption")
			// Pass context if/when UploadToInstagram supports it
			err := services.UploadToInstagram(file, header, instagramCaption, mainCaption, result)
			if err != nil {
				log.Printf("Instagram upload failed: %v", err)
				// Error details are already set within UploadToInstagram (or should be)
			}
		case "tiktok":
			tiktokCaption := r.FormValue("tiktokCaption")
			// Pass the request context (ctx) as the first argument
			err := services.UploadToTikTok(ctx, file, header, tiktokCaption, mainCaption, result)
			if err != nil {
				log.Printf("TikTok upload failed: %v", err)
				// Error details are already set within UploadToTikTok (or should be)
			}
		}
	}

	// --- Render the result ---

	// Create a buffer to store the result HTML content
	var buf bytes.Buffer

	// Prepare data structure for the template
	templateData := map[string]interface{}{
		"Result":            result,               // Pass the result struct
		"SelectedPlatforms": selectedPlatformsMap, // Pass the map of selected platforms
	}

	// Execute the result content template
	if err := templates.ExecuteTemplate(&buf, "result_content.html", templateData); err != nil {
		log.Printf("Failed to execute result template: %v", err)
		// Send a generic error response, but log the detailed one
		http.Error(w, "Failed to display upload results", http.StatusInternalServerError)
		return
	}

	// Write the generated HTML fragment to the response
	// This is intended for use with HTMX, replacing the #result div content
	w.Header().Set("Content-Type", "text/html; charset=utf-8") // Set appropriate content type
	w.Write(buf.Bytes())
}
