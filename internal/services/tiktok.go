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

// UploadToTikTok uploads a video to TikTok
func UploadToTikTok(file multipart.File, header *multipart.FileHeader,
	caption, mainCaption string, result *models.UploadResult) error {

	// Read TikTok token
	tokenFile, err := os.ReadFile("tiktok_token.json")
	if err != nil {
		return fmt.Errorf("user not authenticated with TikTok")
	}

	var token oauth2.Token
	err = json.Unmarshal(tokenFile, &token)
	if err != nil {
		return fmt.Errorf("failed to parse TikTok authentication token")
	}

	// Use main caption if no specific caption provided
	if caption == "" {
		caption = mainCaption
	}

	// Create a temporary file to store the video
	tempFile, err := os.CreateTemp("", "tiktok_upload_*.mp4")
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

	// Seek back to beginning of file for reading
	if _, err := file.Seek(0, 0); err != nil {
		return fmt.Errorf("failed to reset file position: %v", err)
	}

	// Step 1: Initiate upload
	initiateURL := "https://open.tiktokapis.com/v2/video/upload/"

	// Create request for file upload initiation
	req, err := http.NewRequest("POST", initiateURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create initiate request: %v", err)
	}

	req.Header.Set("Authorization", "Bearer "+token.AccessToken)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to initiate upload: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to initiate upload, status: %d, response: %s", resp.StatusCode, string(bodyBytes))
	}

	var initiateResponse struct {
		Data struct {
			UploadURL string `json:"upload_url"`
			VideoID   string `json:"video_id"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&initiateResponse); err != nil {
		return fmt.Errorf("failed to decode initiate response: %v", err)
	}

	// Step 2: Upload video to the provided URL
	fileBytes, err := io.ReadAll(file)
	if err != nil {
		return fmt.Errorf("failed to read file: %v", err)
	}

	uploadReq, err := http.NewRequest("PUT", initiateResponse.Data.UploadURL, bytes.NewReader(fileBytes))
	if err != nil {
		return fmt.Errorf("failed to create upload request: %v", err)
	}

	uploadReq.Header.Set("Content-Type", "application/octet-stream")

	uploadResp, err := client.Do(uploadReq)
	if err != nil {
		return fmt.Errorf("failed to upload video: %v", err)
	}
	defer uploadResp.Body.Close()

	if uploadResp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(uploadResp.Body)
		return fmt.Errorf("failed to upload video, status: %d, response: %s", uploadResp.StatusCode, string(bodyBytes))
	}

	// Step 3: Create post with the uploaded video
	createPostURL := "https://open.tiktokapis.com/v2/video/publish/"

	postData := map[string]interface{}{
		"video_id":      initiateResponse.Data.VideoID,
		"text":          caption,
		"privacy_level": "self_only", // Start with private visibility
	}

	postJSON, _ := json.Marshal(postData)
	postReq, err := http.NewRequest("POST", createPostURL, bytes.NewBuffer(postJSON))
	if err != nil {
		return fmt.Errorf("failed to create post request: %v", err)
	}

	postReq.Header.Set("Authorization", "Bearer "+token.AccessToken)
	postReq.Header.Set("Content-Type", "application/json")

	postResp, err := client.Do(postReq)
	if err != nil {
		return fmt.Errorf("failed to create post: %v", err)
	}
	defer postResp.Body.Close()

	if postResp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(postResp.Body)
		return fmt.Errorf("failed to create post, status: %d, response: %s", postResp.StatusCode, string(bodyBytes))
	}

	var postResponse struct {
		Data struct {
			PostID string `json:"post_id"`
		} `json:"data"`
	}

	if err := json.NewDecoder(postResp.Body).Decode(&postResponse); err != nil {
		return fmt.Errorf("failed to decode post response: %v", err)
	}

	// Update result with success information
	result.TikTok.Success = true
	result.TikTok.PostID = postResponse.Data.PostID
	return nil
}
