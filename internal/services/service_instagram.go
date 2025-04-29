package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"time"
	
	"uploader/internal/models"

	"golang.org/x/oauth2"
)

// UploadToInstagram uploads a video to Instagram as a Reel
func UploadToInstagram(file multipart.File, header *multipart.FileHeader,
	caption, mainCaption string, result *models.UploadResult) error {

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
	containerURL := "https://graph.instagram.com/v22.0/me/media"
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

	var containerResponse models.InstagramMediaResponse
	if err := json.NewDecoder(resp.Body).Decode(&containerResponse); err != nil {
		return fmt.Errorf("failed to decode container response: %v", err)
	}

	// Step 2: Poll for status until media is ready
	mediaID := containerResponse.ID
	statusURL := fmt.Sprintf("https://graph.instagram.com/v22.0/%s", mediaID)

	for i := 0; i < 30; i++ { // Poll for up to 5 minutes
		time.Sleep(10 * time.Second)

		req, _ = http.NewRequest("GET", statusURL, nil)
		req.Header.Set("Authorization", "Bearer "+token.AccessToken)

		resp, err = client.Do(req)
		if err != nil {
			continue
		}

		var statusResponse models.InstagramMediaResponse
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
