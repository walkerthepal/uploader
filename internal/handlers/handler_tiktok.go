package handlers

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"

	"uploader/internal/config"
	"uploader/internal/models"
)

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

	// Use request's context for the token exchange request
	req, err := http.NewRequestWithContext(r.Context(), "POST", tokenURL, strings.NewReader(data.Encode()))
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
