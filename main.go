package main

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/option"
	"google.golang.org/api/youtube/v3"
)

var (
	googleOauthConfig *oauth2.Config
	randomState       = "random"
	templates         *template.Template
)

type Credentials struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
}

func init() {
	creds, err := loadCredentials("creds.json")
	if err != nil {
		log.Fatalf("Failed to load credentials: %v", err)
	}

	googleOauthConfig = &oauth2.Config{
		RedirectURL:  "http://localhost:8080/callback",
		ClientID:     creds.ClientID,
		ClientSecret: creds.ClientSecret,
		Scopes:       []string{"https://www.googleapis.com/auth/youtube.upload"},
		Endpoint:     google.Endpoint,
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

	if creds.ClientID == "" || creds.ClientSecret == "" {
		return nil, fmt.Errorf("client ID or client secret is missing in the credentials file")
	}

	return &creds, nil
}

func main() {
	r := chi.NewRouter()
	r.Use(middleware.Logger)

	r.Get("/", showHomePage)
	r.Get("/login", handleGoogleLogin)
	r.Get("/callback", handleGoogleCallback)
	r.Get("/upload", showUploadPage)
	r.Post("/upload", uploadVideo)

	log.Println("Server is running on :8080")
	http.ListenAndServe(":8080", r)
}

func showHomePage(w http.ResponseWriter, r *http.Request) {
	templates.ExecuteTemplate(w, "index.html", nil)
}

func handleGoogleLogin(w http.ResponseWriter, r *http.Request) {
	url := googleOauthConfig.AuthCodeURL(randomState)
	http.Redirect(w, r, url, http.StatusTemporaryRedirect)
}

func handleGoogleCallback(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("state") != randomState {
		http.Error(w, "Invalid state parameter", http.StatusBadRequest)
		return
	}

	token, err := googleOauthConfig.Exchange(context.Background(), r.URL.Query().Get("code"))
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

	err = os.WriteFile("token.json", tokenFile, 0600)
	if err != nil {
		log.Printf("Unable to write token to file: %v", err)
		http.Error(w, "Failed to save authentication token", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/upload", http.StatusSeeOther)
}

func showUploadPage(w http.ResponseWriter, r *http.Request) {
	templates.ExecuteTemplate(w, "upload.html", nil)
}

func uploadVideo(w http.ResponseWriter, r *http.Request) {
	tokenFile, err := os.ReadFile("token.json")
	if err != nil {
		http.Error(w, "User not authenticated", http.StatusUnauthorized)
		return
	}

	var token oauth2.Token
	err = json.Unmarshal(tokenFile, &token)
	if err != nil {
		http.Error(w, "Failed to parse authentication token", http.StatusInternalServerError)
		return
	}

	client := googleOauthConfig.Client(context.Background(), &token)

	service, err := youtube.NewService(context.Background(), option.WithHTTPClient(client))
	if err != nil {
		http.Error(w, "Failed to create YouTube service", http.StatusInternalServerError)
		return
	}

	file, _, err := r.FormFile("video")
	if err != nil {
		http.Error(w, "Failed to get video file from form", http.StatusBadRequest)
		return
	}
	defer file.Close()

	upload := &youtube.Video{
		Snippet: &youtube.VideoSnippet{
			Title:       r.FormValue("title"),
			Description: r.FormValue("description"),
			CategoryId:  "22", // People & Blogs category
		},
		Status: &youtube.VideoStatus{PrivacyStatus: "private"},
	}

	call := service.Videos.Insert([]string{"snippet", "status"}, upload)
	response, err := call.Media(file).Do()

	if err != nil {
		templates.ExecuteTemplate(w, "result.html", map[string]string{"message": fmt.Sprintf("Error uploading video: %v", err)})
		return
	}

	templates.ExecuteTemplate(w, "result.html", map[string]string{"message": "Video uploaded successfully", "videoId": response.Id})
}
