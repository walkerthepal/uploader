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
)

// UploadToTikTok uploads a video to TikTok using the v2 API
func UploadToTikTok(file multipart.File, header *multipart.FileHeader,
	caption, mainCaption string, result *models.UploadResult) error {

	// Read TikTok token
	tokenFile, err := os.ReadFile("tiktok_token.json")
	if err != nil {
		return fmt.Errorf("user not authenticated with TikTok")
	}

	var tokenResponse models.TikTokTokenResponse
	err = json.Unmarshal(tokenFile, &tokenResponse)
	if err != nil {
		return fmt.Errorf("failed to parse TikTok authentication token: %v", err)
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

	// Read entire file into memory
	fileBytes, err := io.ReadAll(file)
	if err != nil {
		return fmt.Errorf("failed to read file: %v", err)
	}

	// Step 1: Get an upload URL using the new API
	client := &http.Client{Timeout: 30 * time.Second}

	createUploadURLEndpoint := "https://open.tiktokapis.com/v2/post/publish/creator_upload/init/"

	requestData := map[string]interface{}{
		"source_info": map[string]string{
			"source": "PULL_FROM_URL",
		},
	}

	jsonData, err := json.Marshal(requestData)
	if err != nil {
		return fmt.Errorf("failed to create request data: %v", err)
	}

	req, err := http.NewRequest("POST", createUploadURLEndpoint, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %v", err)
	}

	req.Header.Set("Authorization", "Bearer "+tokenResponse.AccessToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to get upload URL: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to get upload URL, status: %d, response: %s", resp.StatusCode, string(bodyBytes))
	}

	var uploadURLResponse struct {
		Data struct {
			UploadURL string `json:"upload_url"`
			PublishID string `json:"publish_id"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&uploadURLResponse); err != nil {
		return fmt.Errorf("failed to decode upload URL response: %v", err)
	}

	// Step 2: Upload the video to the provided URL
	uploadReq, err := http.NewRequest("PUT", uploadURLResponse.Data.UploadURL, bytes.NewReader(fileBytes))
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
	createPostURL := "https://open.tiktokapis.com/v2/post/publish/creator_upload/publish/"

	postData := map[string]interface{}{
		"publish_id": uploadURLResponse.Data.PublishID,
		"post_info": map[string]interface{}{
			"title":           caption,
			"privacy_level":   "SELF_ONLY", // Start with private visibility
			"disable_duet":    false,
			"disable_comment": false,
			"disable_stitch":  false,
		},
	}

	postJSON, _ := json.Marshal(postData)
	postReq, err := http.NewRequest("POST", createPostURL, bytes.NewBuffer(postJSON))
	if err != nil {
		return fmt.Errorf("failed to create post request: %v", err)
	}

	postReq.Header.Set("Authorization", "Bearer "+tokenResponse.AccessToken)
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
