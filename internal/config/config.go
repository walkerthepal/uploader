package config

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

// Credentials holds the OAuth client IDs and secrets
type Credentials struct {
	YouTube struct {
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret"`
	} `json:"youtube"`
	Instagram struct {
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret"`
	} `json:"instagram"`
}

// Config holds all configuration for the application
type Config struct {
	Credentials        *Credentials
	YouTubeOAuthConfig *oauth2.Config
	InstagramOAuthConfig *oauth2.Config
	RandomState        string
	TemplatesDir       string
}

var (
	config *Config
	once   sync.Once
)

// Load loads configuration from a JSON file and initializes OAuth configs
func Load(filename string) (*Config, error) {
	var err error

	once.Do(func() {
		creds, loadErr := loadCredentials(filename)
		if loadErr != nil {
			err = loadErr
			return
		}

		config = &Config{
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
				Scopes:       []string{"instagram_content_publish"},
				Endpoint: oauth2.Endpoint{
					AuthURL:  "https://api.instagram.com/oauth/authorize",
					TokenURL: "https://api.instagram.com/oauth/access_token",
				},
			},
			RandomState:  "random", // In production, this should be randomly generated per request
			TemplatesDir: "templates",
		}
	})

	if err != nil {
		return nil, err
	}

	return config, nil
}

// loadCredentials loads the OAuth credentials from a JSON file
func loadCredentials(filename string) (*Credentials, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read credential file: %v", err)
	}

	var creds Credentials
	err = json.Unmarshal(data, &creds)
	if err != nil {
		return nil, fmt.Errorf("failed to parse credential file: %v", err)
	}

	// Validate YouTube credentials
	if creds.YouTube.ClientID == "" || creds.YouTube.ClientSecret == "" {
		return nil, fmt.Errorf("YouTube client ID or client secret is missing")
	}

	// Validate Instagram credentials
	if creds.Instagram.ClientID == "" || creds.Instagram.ClientSecret == "" {
		return nil, fmt.Errorf("Instagram client ID or client secret is missing")
	}

	return &creds, nil
}

// Get returns the current configuration
func Get() *Config {
	return config
}
