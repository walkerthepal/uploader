package services

import (
	"bytes"
	"context" // Import context package
	"encoding/json"
	"fmt"
	"io"
	"log" // Added for potential debug logging

	// "math" // No longer needed as we use integer division
	"mime/multipart"
	"net/http"
	"os"
	"time"

	"uploader/internal/models"
)

// Define chunk size (e.g., 32MB). TikTok recommends >= 5MB. Max 64MB is common.
const defaultTikTokChunkSize = 32 * 1024 * 1024 // 32 MB

// UploadToTikTok uploads a video to TikTok using the v2 API Direct Post method with chunking.
func UploadToTikTok(ctx context.Context, file multipart.File, header *multipart.FileHeader,
	caption, mainCaption string, result *models.UploadResult) error {

	// --- 1. Authentication and Setup ---
	tokenFile, err := os.ReadFile("tiktok_token.json")
	if err != nil {
		result.TikTok.Success = false
		result.TikTok.Error = fmt.Sprintf("user not authenticated with TikTok: %v", err)
		return fmt.Errorf("user not authenticated with TikTok: %v", err)
	}
	var tokenResponse models.TikTokTokenResponse
	err = json.Unmarshal(tokenFile, &tokenResponse)
	if err != nil {
		result.TikTok.Success = false
		result.TikTok.Error = fmt.Sprintf("failed to parse TikTok authentication token: %v", err)
		return fmt.Errorf("failed to parse TikTok authentication token: %v", err)
	}
	if caption == "" {
		caption = mainCaption
	}
	fileSize := header.Size
	if fileSize == 0 {
		result.TikTok.Success = false
		result.TikTok.Error = "cannot upload empty file"
		return fmt.Errorf("cannot upload empty file")
	}

	// --- 2. Chunking Calculation (Using 32MB Chunk Size) ---
	actualChunkSize := int64(defaultTikTokChunkSize)
	// Calculate total chunks using integer division (rounds down)
	totalChunks := int(fileSize / actualChunkSize)
	// If there's a remainder, we need one more chunk
	if fileSize%actualChunkSize != 0 {
		totalChunks++
	}
	// Ensure totalChunks is at least 1
	if totalChunks == 0 {
		totalChunks = 1
	}

	// --- 3. Initialization Request (Get Upload URL) ---
	initClient := &http.Client{Timeout: 60 * time.Second}
	createUploadURLEndpoint := "https://open.tiktokapis.com/v2/post/publish/video/init/"
	requestData := map[string]interface{}{
		"source_info": map[string]interface{}{
			"source":            "FILE_UPLOAD",
			"video_size":        fileSize,
			"chunk_size":        actualChunkSize,    // Now 32MB
			"total_chunk_count": int64(totalChunks), // Now calculated based on 32MB chunks (should be 2)
		},
		"post_info": map[string]interface{}{
			"title":           caption,
			"privacy_level":   "SELF_ONLY",
			"disable_duet":    false,
			"disable_comment": false,
			"disable_stitch":  false,
		},
	}

	// Log the request data being sent for debugging
	jsonDataLog, _ := json.MarshalIndent(requestData, "", "  ")
	log.Printf("Sending TikTok init request data:\n%s", string(jsonDataLog))

	jsonData, err := json.Marshal(requestData)
	if err != nil {
		result.TikTok.Success = false
		result.TikTok.Error = fmt.Sprintf("failed to marshal init request data: %v", err)
		return fmt.Errorf("failed to marshal init request data: %v", err)
	}

	initReq, err := http.NewRequestWithContext(ctx, "POST", createUploadURLEndpoint, bytes.NewBuffer(jsonData))
	if err != nil {
		result.TikTok.Success = false
		result.TikTok.Error = fmt.Sprintf("failed to create init request: %v", err)
		return fmt.Errorf("failed to create init request: %v", err)
	}
	initReq.Header.Set("Authorization", "Bearer "+tokenResponse.AccessToken)
	initReq.Header.Set("Content-Type", "application/json; charset=UTF-8")

	initResp, err := initClient.Do(initReq)
	if err != nil {
		if ctx.Err() != nil {
			result.TikTok.Success = false
			result.TikTok.Error = fmt.Sprintf("init request cancelled: %v", ctx.Err())
			return fmt.Errorf("init request cancelled: %v", ctx.Err())
		}
		result.TikTok.Success = false
		result.TikTok.Error = fmt.Sprintf("failed to send init request: %v", err)
		return fmt.Errorf("failed to send init request: %v", err)
	}
	defer initResp.Body.Close()

	if initResp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(initResp.Body)
		result.TikTok.Success = false
		result.TikTok.Error = fmt.Sprintf("failed to get upload URL, status: %d, response: %s", initResp.StatusCode, string(bodyBytes))
		log.Printf("TikTok init request failed. Status: %d, Response: %s", initResp.StatusCode, string(bodyBytes))
		return fmt.Errorf("failed to get upload URL, status: %d, response: %s", initResp.StatusCode, string(bodyBytes))
	}

	var uploadURLResponse struct {
		Data struct {
			UploadURL string `json:"upload_url"`
			PublishID string `json:"publish_id"`
		} `json:"data"`
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
			LogID   string `json:"log_id"`
		} `json:"error"`
	}

	bodyBytes, readErr := io.ReadAll(initResp.Body)
	if readErr != nil {
		result.TikTok.Success = false
		result.TikTok.Error = fmt.Sprintf("failed to read init response body: %v", readErr)
		return fmt.Errorf("failed to read init response body: %v", readErr)
	}
	if err := json.Unmarshal(bodyBytes, &uploadURLResponse); err != nil {
		result.TikTok.Success = false
		result.TikTok.Error = fmt.Sprintf("failed to decode init response: %v. Body: %s", err, string(bodyBytes))
		return fmt.Errorf("failed to decode init response: %v", err)
	}
	if uploadURLResponse.Error.Code != "ok" && uploadURLResponse.Error.Code != "" {
		errorMsg := fmt.Sprintf("API error during init: %s (%s, log: %s)", uploadURLResponse.Error.Message, uploadURLResponse.Error.Code, uploadURLResponse.Error.LogID)
		result.TikTok.Success = false
		result.TikTok.Error = errorMsg
		return fmt.Errorf(errorMsg)
	}
	if uploadURLResponse.Data.UploadURL == "" {
		errorMsg := "API did not return an upload URL despite OK status"
		result.TikTok.Success = false
		result.TikTok.Error = errorMsg
		return fmt.Errorf(errorMsg)
	}

	// --- 4. Chunked Upload ---
	if _, err := file.Seek(0, 0); err != nil {
		result.TikTok.Success = false
		result.TikTok.Error = fmt.Sprintf("failed to reset file position for chunking: %v", err)
		return fmt.Errorf("failed to reset file position for chunking: %v", err)
	}

	uploadClient := &http.Client{Timeout: 30 * time.Minute}
	var bytesUploaded int64 = 0
	chunkBuffer := make([]byte, actualChunkSize)

	// Use the same 'totalChunks' calculation for the loop count
	log.Printf("Starting TikTok chunked upload. Total size: %d, Chunk size: %d, Total chunks: %d", fileSize, actualChunkSize, totalChunks)

	for i := 0; i < totalChunks; i++ { // Loop 'totalChunks' times
		if ctx.Err() != nil {
			result.TikTok.Success = false
			result.TikTok.Error = fmt.Sprintf("upload cancelled before chunk %d: %v", i+1, ctx.Err())
			return fmt.Errorf("upload cancelled before chunk %d: %v", i+1, ctx.Err())
		}

		startByte := int64(i) * actualChunkSize
		bytesToRead := actualChunkSize
		if startByte+actualChunkSize > fileSize {
			bytesToRead = fileSize - startByte
		}

		n, readErr := io.ReadFull(file, chunkBuffer[:bytesToRead])
		if readErr != nil && readErr != io.EOF && readErr != io.ErrUnexpectedEOF {
			errorMsg := fmt.Sprintf("failed to read chunk %d data: %v", i+1, readErr)
			result.TikTok.Success = false
			result.TikTok.Error = errorMsg
			return fmt.Errorf(errorMsg)
		}
		currentChunkBytes := chunkBuffer[:n]
		currentChunkSize := int64(n)
		endByte := startByte + currentChunkSize - 1

		uploadReq, err := http.NewRequestWithContext(ctx, "PUT", uploadURLResponse.Data.UploadURL, bytes.NewReader(currentChunkBytes))
		if err != nil {
			errorMsg := fmt.Sprintf("failed to create upload request for chunk %d: %v", i+1, err)
			result.TikTok.Success = false
			result.TikTok.Error = errorMsg
			return fmt.Errorf(errorMsg)
		}

		uploadReq.Header.Set("Content-Type", "video/mp4")
		uploadReq.Header.Set("Content-Length", fmt.Sprintf("%d", currentChunkSize))
		contentRange := fmt.Sprintf("bytes %d-%d/%d", startByte, endByte, fileSize)
		uploadReq.Header.Set("Content-Range", contentRange)

		// log.Printf("Uploading chunk %d/%d: Range %s, Size %d", i+1, totalChunks, contentRange, currentChunkSize) // Debug log
		uploadResp, err := uploadClient.Do(uploadReq)
		if err != nil {
			if ctx.Err() != nil {
				result.TikTok.Success = false
				result.TikTok.Error = fmt.Sprintf("upload of chunk %d cancelled: %v", i+1, ctx.Err())
				return fmt.Errorf("upload of chunk %d cancelled: %v", i+1, ctx.Err())
			}
			errorMsg := fmt.Sprintf("failed to upload chunk %d: %v", i+1, err)
			result.TikTok.Success = false
			result.TikTok.Error = errorMsg
			return fmt.Errorf(errorMsg)
		}

		respBodyBytes, _ := io.ReadAll(uploadResp.Body)
		uploadResp.Body.Close()
		statusCode := uploadResp.StatusCode

		isLastChunk := (i == totalChunks-1) // Check against totalChunks

		if isLastChunk {
			if statusCode != http.StatusOK && statusCode != http.StatusCreated {
				errorMsg := fmt.Sprintf("failed to upload final chunk %d, status: %d, response: %s", i+1, statusCode, string(respBodyBytes))
				result.TikTok.Success = false
				result.TikTok.Error = errorMsg
				return fmt.Errorf(errorMsg)
			}
			bytesUploaded = fileSize
			log.Printf("Successfully uploaded final chunk %d. Total size: %d", i+1, bytesUploaded)
		} else {
			if statusCode != http.StatusPartialContent {
				errorMsg := fmt.Sprintf("failed to upload intermediate chunk %d, expected status 206, got %d, response: %s", i+1, statusCode, string(respBodyBytes))
				result.TikTok.Success = false
				result.TikTok.Error = errorMsg
				return fmt.Errorf(errorMsg)
			}
			bytesUploaded = endByte + 1
			// log.Printf("Successfully uploaded intermediate chunk %d. Bytes uploaded: %d", i+1, bytesUploaded) // Debug log
		}
	}

	// --- 5. Finalize ---
	log.Printf("TikTok chunked upload completed successfully. Publish ID: %s", uploadURLResponse.Data.PublishID)
	result.TikTok.Success = true
	result.TikTok.PostID = uploadURLResponse.Data.PublishID
	return nil // Success!
}
