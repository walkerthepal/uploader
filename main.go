package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/option"
	"google.golang.org/api/youtube/v3"
)

var (
	youtubeOauthConfig   *oauth2.Config
	instagramOauthConfig *oauth2.Config
	randomState          = "random" // In production, this should be randomly generated per request
	templates            *template.Template
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
}

type UploadResult struct {
	YouTube struct {
		Success bool   `json:"success"`
		VideoID string `json:"videoId,omitempty"`
		Error   string `json:"error,omitempty"`
	} `json:"youtube"`
	Instagram struct {
		Success bool   `json:"success"`
		ReelID  string `json:"reelId,omitempty"`
		Error   string `json:"error,omitempty"`
	} `json:"instagram"`
}

// Instagram API response structures
type InstagramTokenResponse struct {
	AccessToken string `json:"access_token"`
	UserID      string `json:"user_id"`
}

type InstagramMediaResponse struct {
	ID      string `json:"id"`
	Status  string `json:"status"`
	MediaID string `json:"media_id"`
}

func init() {
	creds, err := loadCredentials("creds.json")
	if err != nil {
		log.Fatalf("Failed to load credentials: %v", err)
	}

	youtubeOauthConfig = &oauth2.Config{
		RedirectURL:  "http://localhost:3000/callback/youtube",
		ClientID:     creds.YouTube.ClientID,
		ClientSecret: creds.YouTube.ClientSecret,
		Scopes:       []string{"https://www.googleapis.com/auth/youtube.upload"},
		Endpoint:     google.Endpoint,
	}

	instagramOauthConfig = &oauth2.Config{
		RedirectURL:  "http://localhost:3000/callback/instagram",
		ClientID:     creds.Instagram.ClientID,
		ClientSecret: creds.Instagram.ClientSecret,
		Scopes:       []string{"instagram_content_publish"},
		Endpoint: oauth2.Endpoint{
			AuthURL:  "https://api.instagram.com/oauth/authorize",
			TokenURL: "https://api.instagram.com/oauth/access_token",
		},
	}

	templates = template.Must(template.ParseGlob("templates/*.html"))
}

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

func main() {
	r := chi.NewRouter()
	r.Use(middleware.Logger)

	r.Get("/", showHomePage)
	r.Get("/login/youtube", handleYoutubeLogin)
	r.Get("/login/instagram", handleInstagramLogin)
	r.Get("/callback/youtube", handleYoutubeCallback)
	r.Get("/callback/instagram", handleInstagramCallback)
	r.Get("/upload", showUploadPage)
	r.Post("/upload", handleUpload)

	log.Println("Server is running on :3000")
	http.ListenAndServe(":3000", r)
}

func showHomePage(w http.ResponseWriter, r *http.Request) {
	templates.ExecuteTemplate(w, "index.html", nil)
}

func handleYoutubeLogin(w http.ResponseWriter, r *http.Request) {
	url := youtubeOauthConfig.AuthCodeURL(randomState)
	http.Redirect(w, r, url, http.StatusTemporaryRedirect)
}

func handleInstagramLogin(w http.ResponseWriter, r *http.Request) {
	url := instagramOauthConfig.AuthCodeURL(randomState)
	http.Redirect(w, r, url, http.StatusTemporaryRedirect)
}

func handleYoutubeCallback(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("state") != randomState {
		http.Error(w, "Invalid state parameter", http.StatusBadRequest)
		return
	}

	token, err := youtubeOauthConfig.Exchange(context.Background(), r.URL.Query().Get("code"))
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

func handleInstagramCallback(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("state") != randomState {
		http.Error(w, "Invalid state parameter", http.StatusBadRequest)
		return
	}

	token, err := instagramOauthConfig.Exchange(context.Background(), r.URL.Query().Get("code"))
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

func showUploadPage(w http.ResponseWriter, r *http.Request) {
	templates.ExecuteTemplate(w, "upload.html", nil)
}

func handleUpload(w http.ResponseWriter, r *http.Request) {
	result := &UploadResult{}

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

	// Handle YouTube upload if selected
	for _, platform := range platforms {
		switch platform {
		case "youtube":
			err := uploadToYoutube(file, header, r.FormValue("youtubeTitle"),
				r.FormValue("youtubeDescription"), mainCaption, result)
			if err != nil {
				result.YouTube.Success = false
				result.YouTube.Error = err.Error()
			}
		case "instagram":
			err := uploadToInstagram(file, header, r.FormValue("instagramCaption"),
				mainCaption, result)
			if err != nil {
				result.Instagram.Success = false
				result.Instagram.Error = err.Error()
			}
		}
	}

	templates.ExecuteTemplate(w, "result.html", result)
}

func uploadToYoutube(file multipart.File, header *multipart.FileHeader,
	title, description, mainCaption string, result *UploadResult) error {

	tokenFile, err := os.ReadFile("youtube_token.json")
	if err != nil {
		return fmt.Errorf("user not authenticated with YouTube")
	}

	var token oauth2.Token
	err = json.Unmarshal(tokenFile, &token)
	if err != nil {
		return fmt.Errorf("failed to parse YouTube authentication token")
	}

	client := youtubeOauthConfig.Client(context.Background(), &token)
	service, err := youtube.NewService(context.Background(), option.WithHTTPClient(client))
	if err != nil {
		return fmt.Errorf("failed to create YouTube service")
	}

	// Use main caption if no specific description provided
	if description == "" {
		description = mainCaption
	}

	upload := &youtube.Video{
		Snippet: &youtube.VideoSnippet{
			Title:       title,
			Description: description,
			CategoryId:  "22", // People & Blogs category
		},
		Status: &youtube.VideoStatus{PrivacyStatus: "private"},
	}

	call := service.Videos.Insert([]string{"snippet", "status"}, upload)
	response, err := call.Media(file).Do()
	if err != nil {
		return fmt.Errorf("failed to upload to YouTube: %v", err)
	}

	result.YouTube.Success = true
	result.YouTube.VideoID = response.Id
	return nil
}

func uploadToInstagram(file multipart.File, header *multipart.FileHeader,
	caption, mainCaption string, result *UploadResult) error {

	// Read Instagram token
	tokenFile, err := os.ReadFile("instagram_token.json")
	if err != nil {
		return fmt.Errorf("user not authenticated with Instagram")
	}

	var token oauth2.Token
	err = json.Unmarshal(tokenFile, &token)
	if err != nil {
		return fmt.Errorf("failed to parse Instagram authentication token")
	}

	// Use main caption if no specific caption provided
	if caption == "" {
		caption = mainCaption
	}

	// Create a temporary file to store the video
	tempFile, err := os.CreateTemp("", "instagram_upload_*.mp4")
	if err != nil {
		return fmt.Errorf("failed to create temporary file: %v", err)
	}
	defer os.Remove(tempFile.Name())
	defer tempFile.Close()

	// Copy the uploaded file to the temporary file
	_, err = io.Copy(tempFile, file)
	if err != nil {
		return fmt.Errorf("failed to copy upload to temporary file: %v", err)
	}

	// Step 1: Create container for the media
	containerURL := "https://graph.instagram.com/v12.0/me/media"
	containerData := map[string]string{
		"media_type": "REELS",
		"video_url":  tempFile.Name(),
		"caption":    caption,
	}

	containerJSON, _ := json.Marshal(containerData)
	req, err := http.NewRequest("POST", containerURL, bytes.NewBuffer(containerJSON))
	if err != nil {
		return fmt.Errorf("failed to create container request: %v", err)
	}

	req.Header.Set("Authorization", "Bearer "+token.AccessToken)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to create media container: %v", err)
	}
	defer resp.Body.Close()

	var containerResponse InstagramMediaResponse
	if err := json.NewDecoder(resp.Body).Decode(&containerResponse); err != nil {
		return fmt.Errorf("failed to decode container response: %v", err)
	}

	// Step 2: Poll for status until media is ready
	mediaID := containerResponse.ID
	statusURL := fmt.Sprintf("https://graph.instagram.com/v12.0/%s", mediaID)

	for i := 0; i < 30; i++ { // Poll for up to 5 minutes
		time.Sleep(10 * time.Second)

		req, _ = http.NewRequest("GET", statusURL, nil)
		req.Header.Set("Authorization", "Bearer "+token.AccessToken)

		resp, err = client.Do(req)
		if err != nil {
			continue
		}

		var statusResponse InstagramMediaResponse
		if err := json.NewDecoder(resp.Body).Decode(&statusResponse); err != nil {
			resp.Body.Close()
			continue
		}
		resp.Body.Close()

		if statusResponse.Status == "FINISHED" {
			result.Instagram.Success = true
			result.Instagram.ReelID = mediaID
			return nil
		}
	}

	return fmt.Errorf("timeout waiting for Instagram upload to complete")
}
