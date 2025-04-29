package handlers

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"

	"uploader/internal/config"
)

// HandleYoutubeLogin initiates the YouTube OAuth flow
func HandleYoutubeLogin(w http.ResponseWriter, r *http.Request) {
	cfg := config.Get()
	url := cfg.YouTubeOAuthConfig.AuthCodeURL(cfg.RandomState)
	http.Redirect(w, r, url, http.StatusTemporaryRedirect)
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
		log.Printf("YouTube code exchange failed: %v", err) // Added logging
		http.Error(w, "Code exchange failed", http.StatusInternalServerError)
		return
	}

	tokenFile, err := json.Marshal(token)
	if err != nil {
		log.Printf("Unable to marshal YouTube token: %v", err)
		http.Error(w, "Failed to process authentication token", http.StatusInternalServerError)
		return
	}

	err = os.WriteFile("youtube_token.json", tokenFile, 0600)
	if err != nil {
		log.Printf("Unable to write YouTube token to file: %v", err)
		http.Error(w, "Failed to save authentication token", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/upload", http.StatusSeeOther)
}
