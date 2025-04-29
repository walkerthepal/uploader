package config

import (
	"encoding/json"
	"fmt"
	"os"

	// Remove "sync" import if no longer needed elsewhere

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

type Credentials struct {
	YouTube struct {
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret"`
	} `json:"youtube"`
	Instagram struct {
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret"`
	} `json:"instagram"`
	TikTok struct {
		ClientKey    string `json:"client_key"`
		ClientSecret string `json:"client_secret"`
	} `json:"tiktok"`
}

// Config holds all configuration for the application
type Config struct {
	Credentials          *Credentials
	YouTubeOAuthConfig   *oauth2.Config
	InstagramOAuthConfig *oauth2.Config
	TikTokOAuthConfig    *oauth2.Config
	RandomState          string
	TemplatesDir         string
}

var (
	// Keep the global config variable if you still want a singleton,
	// but its initialization is now handled differently.
	globalConfig *Config
)

// Load loads configuration from a JSON file and initializes OAuth configs.
// It now potentially updates the global config on each successful call.
func Load(filename string) (*Config, error) {
	creds, err := loadCredentials(filename)
	if err != nil {
		// Return error immediately, don't try to use partially loaded creds
		return nil, err
	}

	// Create a new config instance based on the loaded creds
	cfg := &Config{
		Credentials: creds,
		YouTubeOAuthConfig: &oauth2.Config{
			RedirectURL:  "http://localhost:3000/callback/youtube",
			ClientID:     creds.YouTube.ClientID,
			ClientSecret: creds.YouTube.ClientSecret,
			Scopes:       []string{"https://www.googleapis.com/auth/youtube.upload"},
			Endpoint:     google.Endpoint,
		},
		InstagramOAuthConfig: &oauth2.Config{
			RedirectURL:  "http://localhost:3000/callback/instagram",
			ClientID:     creds.Instagram.ClientID,
			ClientSecret: creds.Instagram.ClientSecret,
			// Update scopes to match what's configured in FB dev portal
			Scopes: []string{"user_profile", "user_media"},
			Endpoint: oauth2.Endpoint{
				// Update to use the Graph API endpoints
				AuthURL:  "https://api.instagram.com/oauth/authorize",
				TokenURL: "https://api.instagram.com/oauth/access_token",
			},
		},
		TikTokOAuthConfig: &oauth2.Config{
			RedirectURL:  "https://2b21-136-56-177-106.ngrok-free.app/callback/tiktok",
			ClientID:     creds.TikTok.ClientKey,
			ClientSecret: creds.TikTok.ClientSecret,
			Scopes:       []string{"user.info.basic", "video.upload", "video.publish"},
			Endpoint: oauth2.Endpoint{
				AuthURL:  "https://www.tiktok.com/v2/auth/authorize/",
				TokenURL: "https://open.tiktokapis.com/v2/oauth/token/",
			},
		},
		RandomState:  "random",    // Consider making this truly random per request
		TemplatesDir: "templates", // Consider making this configurable
	}

	// Update the global config variable upon successful load
	globalConfig = cfg
	return globalConfig, nil // Return the newly created/updated config
}

// loadCredentials remains the same
func loadCredentials(filename string) (*Credentials, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		// Add the filename to the error context for better debugging
		return nil, fmt.Errorf("failed to read credential file '%s': %w", filename, err)
	}

	var creds Credentials
	err = json.Unmarshal(data, &creds)
	if err != nil {
		return nil, fmt.Errorf("failed to parse credential file '%s': %w", filename, err)
	}

	// Validate YouTube credentials
	if creds.YouTube.ClientID == "" || creds.YouTube.ClientSecret == "" {
		return nil, fmt.Errorf("YouTube client ID or client secret is missing in '%s'", filename)
	}

	// Validate Instagram credentials
	if creds.Instagram.ClientID == "" || creds.Instagram.ClientSecret == "" {
		return nil, fmt.Errorf("Instagram client ID or client secret is missing in '%s'", filename)
	}

	return &creds, nil
}

// Get returns the current global configuration
func Get() *Config {
	// Consider adding a check here if globalConfig is nil and returning an error
	// or panicking, depending on how critical config is.
	if globalConfig == nil {
		// This should ideally not happen if Load is called correctly in main
		panic("Configuration accessed before it was successfully loaded")
	}
	return globalConfig
}
