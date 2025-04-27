package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"html/template"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"

	"uploader/internal/config"
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

// HandleYoutubeLogin initiates the YouTube OAuth flow
func HandleYoutubeLogin(w http.ResponseWriter, r *http.Request) {
	cfg := config.Get()
	url := cfg.YouTubeOAuthConfig.AuthCodeURL(cfg.RandomState)
	http.Redirect(w, r, url, http.StatusTemporaryRedirect)
}

// HandleInstagramLogin initiates the Instagram OAuth flow
func HandleInstagramLogin(w http.ResponseWriter, r *http.Request) {
	cfg := config.Get()
	url := cfg.InstagramOAuthConfig.AuthCodeURL(cfg.RandomState)
	http.Redirect(w, r, url, http.StatusTemporaryRedirect)
}

// HandleTikTokLogin initiates the TikTok OAuth flow
func HandleTikTokLogin(w http.ResponseWriter, r *http.Request) {
	cfg := config.Get()

	// Build the auth URL manually to ensure all params are included
	baseURL := "https://www.tiktok.com/v2/auth/authorize/"
	params := url.Values{}
	params.Add("client_key", cfg.TikTokOAuthConfig.ClientID)
	params.Add("response_type", "code")
	params.Add("scope", strings.Join(cfg.TikTokOAuthConfig.Scopes, ","))
	params.Add("redirect_uri", cfg.TikTokOAuthConfig.RedirectURL)
	params.Add("state", cfg.RandomState)

	authURL := baseURL + "?" + params.Encode()

	http.Redirect(w, r, authURL, http.StatusTemporaryRedirect)
}

// HandleYoutubeCallback processes the YouTube OAuth callback
func HandleYoutubeCallback(w http.ResponseWriter, r *http.Request) {
	cfg := config.Get()
	if r.URL.Query().Get("state") != cfg.RandomState {
		http.Error(w, "Invalid state parameter", http.StatusBadRequest)
		return
	}

	token, err := cfg.YouTubeOAuthConfig.Exchange(context.Background(), r.URL.Query().Get("code"))
	if err != nil {
		http.Error(w, "Code exchange failed", http.StatusInternalServerError)
		return
	}

	tokenFile, err := json.Marshal(token)
	if err != nil {
		log.Printf("Unable to marshal token: %v", err)
		http.Error(w, "Failed to process authentication token", http.StatusInternalServerError)
		return
	}

	err = os.WriteFile("youtube_token.json", tokenFile, 0600)
	if err != nil {
		log.Printf("Unable to write token to file: %v", err)
		http.Error(w, "Failed to save authentication token", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/upload", http.StatusSeeOther)
}

// HandleInstagramCallback processes the Instagram OAuth callback
func HandleInstagramCallback(w http.ResponseWriter, r *http.Request) {
	cfg := config.Get()
	if r.URL.Query().Get("state") != cfg.RandomState {
		http.Error(w, "Invalid state parameter", http.StatusBadRequest)
		return
	}

	token, err := cfg.InstagramOAuthConfig.Exchange(context.Background(), r.URL.Query().Get("code"))
	if err != nil {
		http.Error(w, "Code exchange failed", http.StatusInternalServerError)
		return
	}

	// Save the Instagram token
	tokenFile, err := json.Marshal(token)
	if err != nil {
		log.Printf("Unable to marshal Instagram token: %v", err)
		http.Error(w, "Failed to process authentication token", http.StatusInternalServerError)
		return
	}

	err = os.WriteFile("instagram_token.json", tokenFile, 0600)
	if err != nil {
		log.Printf("Unable to write Instagram token to file: %v", err)
		http.Error(w, "Failed to save authentication token", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/upload", http.StatusSeeOther)
}

// Update HandleTikTokCallback in handlers.go

// HandleTikTokCallback processes the TikTok OAuth callback
func HandleTikTokCallback(w http.ResponseWriter, r *http.Request) {
	cfg := config.Get()
	if r.URL.Query().Get("state") != cfg.RandomState {
		http.Error(w, "Invalid state parameter", http.StatusBadRequest)
		return
	}

	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "No authorization code provided", http.StatusBadRequest)
		return
	}

	// TikTok's V2 API requires a custom token exchange approach
	tokenURL := "https://open.tiktokapis.com/v2/oauth/token/"
	data := url.Values{}
	data.Set("client_key", cfg.TikTokOAuthConfig.ClientID)
	data.Set("client_secret", cfg.TikTokOAuthConfig.ClientSecret)
	data.Set("code", code)
	data.Set("grant_type", "authorization_code")
	data.Set("redirect_uri", cfg.TikTokOAuthConfig.RedirectURL)

	req, err := http.NewRequest("POST", tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		log.Printf("Failed to create token request: %v", err)
		http.Error(w, "Failed to process authentication", http.StatusInternalServerError)
		return
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("Token exchange failed: %v", err)
		http.Error(w, "Code exchange failed", http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		log.Printf("Token exchange returned non-200 status: %d, body: %s", resp.StatusCode, string(bodyBytes))
		http.Error(w, "Code exchange failed", http.StatusInternalServerError)
		return
	}

	var tokenResponse models.TikTokTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResponse); err != nil {
		log.Printf("Failed to decode token response: %v", err)
		http.Error(w, "Failed to process authentication token", http.StatusInternalServerError)
		return
	}

	// Save the token
	tokenFile, err := json.Marshal(tokenResponse)
	if err != nil {
		log.Printf("Unable to marshal TikTok token: %v", err)
		http.Error(w, "Failed to process authentication token", http.StatusInternalServerError)
		return
	}

	err = os.WriteFile("tiktok_token.json", tokenFile, 0600)
	if err != nil {
		log.Printf("Unable to write TikTok token to file: %v", err)
		http.Error(w, "Failed to save authentication token", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/upload", http.StatusSeeOther)
}

// ShowUploadPage displays the upload form
func ShowUploadPage(w http.ResponseWriter, r *http.Request) {
	templates.ExecuteTemplate(w, "upload.html", nil)
}

// HandleUpload processes the upload form submission
func HandleUpload(w http.ResponseWriter, r *http.Request) {
	result := &models.UploadResult{} // Keep this initialization

	// Parse the multipart form
	err := r.ParseMultipartForm(32 << 20) // 32MB max
	if err != nil {
		log.Printf("Failed to parse form: %v", err)
		http.Error(w, "Failed to parse form: file may be too large", http.StatusBadRequest)
		return
	}

	// Get the platforms selected for upload
	platforms := r.Form["platforms"] // This is a slice of strings like ["youtube", "tiktok"]
	if len(platforms) == 0 {
		http.Error(w, "No platforms selected for upload", http.StatusBadRequest)
		return
	}

	// *** NEW: Create a map to easily check selected platforms in the template ***
	selectedPlatformsMap := make(map[string]bool)
	for _, p := range platforms {
		selectedPlatformsMap[p] = true
	}

	file, header, err := r.FormFile("video")
	if err != nil {
		log.Printf("Failed to get video file: %v", err)
		http.Error(w, "Failed to get video file: please ensure a valid video file is selected", http.StatusBadRequest)
		return
	}
	defer file.Close()

	// Log file details for debugging
	log.Printf("Received file: %s, size: %d bytes", header.Filename, header.Size)

	mainCaption := r.FormValue("mainCaption")

	// Handle uploads to selected platforms (Loop remains the same)
	for _, platform := range platforms {
		// Reset file position before each platform upload attempt
		if _, err := file.Seek(0, 0); err != nil {
			log.Printf("CRITICAL: Failed to reset file position before uploading to %s: %v", platform, err)
			// Decide how to handle this - maybe skip this platform and record an error?
			// For now, we'll log and continue, but the subsequent upload might fail or use wrong data.
			// Assigning a specific error might be better:
			switch platform {
			case "youtube":
				result.YouTube.Success = false
				result.YouTube.Error = "Internal server error: failed to prepare file"
			case "instagram":
				result.Instagram.Success = false
				result.Instagram.Error = "Internal server error: failed to prepare file"
			case "tiktok":
				result.TikTok.Success = false
				result.TikTok.Error = "Internal server error: failed to prepare file"
			}
			continue // Skip to the next platform
		}

		switch platform {
		case "youtube":
			title := r.FormValue("youtubeTitle")
			if title == "" {
				result.YouTube.Success = false
				result.YouTube.Error = "YouTube title is required"
				continue // Skip YouTube upload if title missing
			}
			err := services.UploadToYoutube(file, header, title,
				r.FormValue("youtubeDescription"), mainCaption, result)
			if err != nil {
				log.Printf("YouTube upload failed: %v", err)
				result.YouTube.Success = false
				result.YouTube.Error = err.Error()
				// No need to assign result fields here, UploadToYoutube does it
			}
		case "instagram":
			err := services.UploadToInstagram(file, header, r.FormValue("instagramCaption"),
				mainCaption, result)
			if err != nil {
				log.Printf("Instagram upload failed: %v", err)
				// No need to assign result fields here, UploadToInstagram does it
			}
		case "tiktok":
			err := services.UploadToTikTok(file, header, r.FormValue("tiktokCaption"),
				mainCaption, result)
			if err != nil {
				log.Printf("TikTok upload failed: %v", err)
				// No need to assign result fields here, UploadToTikTok does it
			}
		}
	}

	// Create a buffer to store the result HTML
	var buf bytes.Buffer

	// *** NEW: Create data structure to pass both results and selected platforms ***
	templateData := map[string]interface{}{
		"Result":            result,               // Pass the result struct
		"SelectedPlatforms": selectedPlatformsMap, // Pass the map of selected platforms
	}

	// *** UPDATED: Execute template with the combined data ***
	if err := templates.ExecuteTemplate(&buf, "result_content.html", templateData); err != nil {
		log.Printf("Failed to execute result template: %v", err)
		// Send a generic error response, but log the detailed one
		http.Error(w, "Failed to display upload results", http.StatusInternalServerError)
		return
	}

	// Write the result to the response
	w.Write(buf.Bytes())
}
