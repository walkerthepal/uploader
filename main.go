package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/option"
	"google.golang.org/api/youtube/v3"
)

var (
	googleOauthConfig *oauth2.Config
	randomState       = "random"
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
	router := gin.Default()

	router.GET("/login", handleGoogleLogin)
	router.GET("/callback", handleGoogleCallback)
	router.POST("/upload", uploadVideo)

	router.Run(":8080")
}

func handleGoogleLogin(c *gin.Context) {
	url := googleOauthConfig.AuthCodeURL(randomState)
	c.Redirect(http.StatusTemporaryRedirect, url)
}

func handleGoogleCallback(c *gin.Context) {
	if c.Query("state") != randomState {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid state parameter"})
		return
	}

	token, err := googleOauthConfig.Exchange(context.Background(), c.Query("code"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Code exchange failed"})
		return
	}

	// Store the token securely (in a real application, you'd use a database)
	tokenFile, err := json.Marshal(token)
	if err != nil {
		log.Printf("Unable to marshal token: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to process authentication token"})
		return
	}

	err = os.WriteFile("token.json", tokenFile, 0600)
	if err != nil {
		log.Printf("Unable to write token to file: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save authentication token"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Successfully authenticated with Google"})
}

func uploadVideo(c *gin.Context) {
	// Read the stored token
	tokenFile, err := os.ReadFile("token.json")
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	var token oauth2.Token
	err = json.Unmarshal(tokenFile, &token)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to parse authentication token"})
		return
	}

	client := googleOauthConfig.Client(context.Background(), &token)

	service, err := youtube.NewService(context.Background(), option.WithHTTPClient(client))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create YouTube service"})
		return
	}

	file, err := c.FormFile("video")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to get video file from form"})
		return
	}

	fileContent, err := file.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to open video file"})
		return
	}
	defer fileContent.Close()

	upload := &youtube.Video{
		Snippet: &youtube.VideoSnippet{
			Title:       c.PostForm("title"),
			Description: c.PostForm("description"),
			CategoryId:  "22", // People & Blogs category
		},
		Status: &youtube.VideoStatus{PrivacyStatus: "private"},
	}

	call := service.Videos.Insert([]string{"snippet", "status"}, upload)
	response, err := call.Media(fileContent).Do()

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Error making YouTube API call: %v", err)})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Video uploaded successfully", "video_id": response.Id})
}
