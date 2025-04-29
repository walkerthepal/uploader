package handlers

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"

	"uploader/internal/config"
)

// HandleInstagramLogin initiates the Instagram OAuth flow
func HandleInstagramLogin(w http.ResponseWriter, r *http.Request) {
	cfg := config.Get()
	url := cfg.InstagramOAuthConfig.AuthCodeURL(cfg.RandomState)
	http.Redirect(w, r, url, http.StatusTemporaryRedirect)
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
		log.Printf("Instagram code exchange failed: %v", err) // Added logging
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
