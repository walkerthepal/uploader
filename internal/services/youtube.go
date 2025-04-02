package services

import (
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"os"
	
	"uploader/internal/config"
	"uploader/internal/models"

	"golang.org/x/oauth2"
	"google.golang.org/api/option"
	"google.golang.org/api/youtube/v3"
)

// UploadToYoutube uploads a video to YouTube
func UploadToYoutube(file multipart.File, header *multipart.FileHeader,
	title, description, mainCaption string, result *models.UploadResult) error {

	tokenFile, err := os.ReadFile("youtube_token.json")
	if err != nil {
		return fmt.Errorf("user not authenticated with YouTube")
	}

	var token oauth2.Token
	err = json.Unmarshal(tokenFile, &token)
	if err != nil {
		return fmt.Errorf("failed to parse YouTube authentication token")
	}

	cfg := config.Get()
	client := cfg.YouTubeOAuthConfig.Client(context.Background(), &token)
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
