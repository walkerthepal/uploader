package handlers

import (
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
	result := &models.UploadResult{}

	// Parse the multipart form
	err := r.ParseMultipartForm(32 << 20) // 32MB max
	if err != nil {
		http.Error(w, "Failed to parse form", http.StatusBadRequest)
		return
	}

	// Get the platforms selected for upload
	platforms := r.Form["platforms"]

	file, header, err := r.FormFile("video")
	if err != nil {
		http.Error(w, "Failed to get video file", http.StatusBadRequest)
		return
	}
	defer file.Close()

	mainCaption := r.FormValue("mainCaption")

	// Handle uploads to selected platforms
	for _, platform := range platforms {
		switch platform {
		case "youtube":
			err := services.UploadToYoutube(file, header, r.FormValue("youtubeTitle"),
				r.FormValue("youtubeDescription"), mainCaption, result)
			if err != nil {
				result.YouTube.Success = false
				result.YouTube.Error = err.Error()
			}
		case "instagram":
			err := services.UploadToInstagram(file, header, r.FormValue("instagramCaption"),
				mainCaption, result)
			if err != nil {
				result.Instagram.Success = false
				result.Instagram.Error = err.Error()
			}
		case "tiktok":
			err := services.UploadToTikTok(file, header, r.FormValue("tiktokCaption"),
				mainCaption, result)
			if err != nil {
				result.TikTok.Success = false
				result.TikTok.Error = err.Error()
			}
		}
	}

	templates.ExecuteTemplate(w, "result.html", result)
}
