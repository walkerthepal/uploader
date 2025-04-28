package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"time"

	"uploader/internal/models"
)

// UploadToTikTok uploads a video to TikTok using the v2 API Direct Post method
func UploadToTikTok(ctx context.Context, file multipart.File, header *multipart.FileHeader,
	caption, mainCaption string, result *models.UploadResult) error {

	// If no specific caption is provided, use the main caption
	if caption == "" {
		caption = mainCaption
	}

	// --- 1. Read authentication token ---
	tokenFile, err := os.ReadFile("tiktok_token.json")
	if err != nil {
		result.TikTok.Success = false
		result.TikTok.Error = "User not authenticated with TikTok"
		return fmt.Errorf("user not authenticated with TikTok: %v", err)
	}

	var tokenResponse models.TikTokTokenResponse
	err = json.Unmarshal(tokenFile, &tokenResponse)
	if err != nil {
		result.TikTok.Success = false
		result.TikTok.Error = "Failed to parse TikTok authentication token"
		return fmt.Errorf("failed to parse TikTok authentication token: %v", err)
	}

	// --- 2. Calculate file size and chunk information ---
	fileSize := header.Size
	if fileSize == 0 {
		result.TikTok.Success = false
		result.TikTok.Error = "Cannot upload empty file"
		return fmt.Errorf("cannot upload empty file")
	}

	// Try using integer division only with no remainder handling
	// In TikTok's example: 50,000,123 ÷ 10,000,000 = 5.0000123, but they use 5 chunks
	const exactChunkSize = 10 * 1000 * 1000 // exactly 10,000,000 bytes as in their docs

	// Calculate total chunks using ONLY integer division (truncate decimal part)
	totalChunks := int(fileSize / exactChunkSize)

	// Ensure we have at least 1 chunk
	if totalChunks < 1 {
		totalChunks = 1
	}

	log.Printf("TikTok upload details: File size = %d bytes, Chunk size = %d bytes, Total chunks = %d",
		fileSize, exactChunkSize, totalChunks)

	// --- 3. Initialize upload (get upload URL) ---
	client := &http.Client{Timeout: 60 * time.Second}
	initEndpoint := "https://open.tiktokapis.com/v2/post/publish/video/init/"

	initRequest := map[string]interface{}{
		"post_info": map[string]interface{}{
			"title":           caption,
			"privacy_level":   "SELF_ONLY", // Start with private visibility
			"disable_duet":    false,
			"disable_comment": false,
			"disable_stitch":  false,
		},
		"source_info": map[string]interface{}{
			"source":            "FILE_UPLOAD",
			"video_size":        fileSize,
			"chunk_size":        exactChunkSize,
			"total_chunk_count": totalChunks,
		},
	}

	initJSON, err := json.Marshal(initRequest)
	if err != nil {
		result.TikTok.Success = false
		result.TikTok.Error = "Failed to create initialization request"
		return fmt.Errorf("failed to create initialization request: %v", err)
	}

	log.Printf("Sending TikTok init request: %s", string(initJSON))

	req, err := http.NewRequestWithContext(ctx, "POST", initEndpoint, bytes.NewBuffer(initJSON))
	if err != nil {
		result.TikTok.Success = false
		result.TikTok.Error = "Failed to create initialization request"
		return fmt.Errorf("failed to create initialization request: %v", err)
	}

	req.Header.Set("Authorization", "Bearer "+tokenResponse.AccessToken)
	req.Header.Set("Content-Type", "application/json; charset=UTF-8")

	resp, err := client.Do(req)
	if err != nil {
		result.TikTok.Success = false
		result.TikTok.Error = "Failed to connect to TikTok API"
		return fmt.Errorf("failed to connect to TikTok API: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		result.TikTok.Success = false
		result.TikTok.Error = "Failed to read API response"
		return fmt.Errorf("failed to read API response: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		result.TikTok.Success = false
		result.TikTok.Error = fmt.Sprintf("Failed to initialize upload (HTTP %d)", resp.StatusCode)
		log.Printf("TikTok init failed with status %d: %s", resp.StatusCode, string(body))
		return fmt.Errorf("failed to initialize upload: %s", string(body))
	}

	// Parse the initialization response
	var initResponse struct {
		Data struct {
			PublishID string `json:"publish_id"`
			UploadURL string `json:"upload_url"`
		} `json:"data"`
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
			LogID   string `json:"log_id"`
		} `json:"error"`
	}

	err = json.Unmarshal(body, &initResponse)
	if err != nil {
		result.TikTok.Success = false
		result.TikTok.Error = "Failed to parse initialization response"
		return fmt.Errorf("failed to parse initialization response: %v", err)
	}

	// Check for API errors
	if initResponse.Error.Code != "ok" && initResponse.Error.Code != "" {
		result.TikTok.Success = false
		result.TikTok.Error = fmt.Sprintf("TikTok API error: %s", initResponse.Error.Message)
		return fmt.Errorf("TikTok API error: %s (code: %s, log: %s)",
			initResponse.Error.Message, initResponse.Error.Code, initResponse.Error.LogID)
	}

	// Validate response data
	if initResponse.Data.UploadURL == "" {
		result.TikTok.Success = false
		result.TikTok.Error = "No upload URL received from TikTok API"
		return fmt.Errorf("no upload URL received from TikTok API")
	}

	if initResponse.Data.PublishID == "" {
		result.TikTok.Success = false
		result.TikTok.Error = "No publish ID received from TikTok API"
		return fmt.Errorf("no publish ID received from TikTok API")
	}

	// --- 4. Upload the video file in chunks ---
	uploadURL := initResponse.Data.UploadURL
	publishID := initResponse.Data.PublishID

	log.Printf("TikTok initialization successful. PublishID: %s", publishID)

	// Reset file position to the beginning
	if _, err := file.Seek(0, 0); err != nil {
		result.TikTok.Success = false
		result.TikTok.Error = "Failed to prepare file for upload"
		return fmt.Errorf("failed to prepare file for upload: %v", err)
	}

	// Prepare for chunked upload
	uploadClient := &http.Client{Timeout: 15 * time.Minute}

	// Upload each chunk
	for i := 0; i < totalChunks; i++ {
		// Check if request is cancelled
		if ctx.Err() != nil {
			result.TikTok.Success = false
			result.TikTok.Error = "Upload cancelled"
			return fmt.Errorf("upload cancelled: %v", ctx.Err())
		}

		// Calculate chunk bounds
		startByte := int64(i) * exactChunkSize
		endByte := startByte + exactChunkSize - 1

		// Adjust the last chunk size if needed
		if endByte >= fileSize {
			endByte = fileSize - 1
		}
		currentChunkSize := endByte - startByte + 1

		// For the last chunk, make sure we read the exact remaining bytes
		var bytesToRead int64
		if i == totalChunks-1 {
			bytesToRead = fileSize - startByte
		} else {
			bytesToRead = currentChunkSize
		}

		// Read the chunk data
		chunkData := make([]byte, bytesToRead)
		n, err := io.ReadFull(file, chunkData)
		if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
			result.TikTok.Success = false
			result.TikTok.Error = fmt.Sprintf("Failed to read chunk %d", i+1)
			return fmt.Errorf("failed to read chunk %d: %v", i+1, err)
		}

		// Prepare the chunk data

		// Create the upload request for this chunk
		uploadReq, err := http.NewRequestWithContext(ctx, "PUT", uploadURL, bytes.NewReader(chunkData[:n]))
		if err != nil {
			result.TikTok.Success = false
			result.TikTok.Error = fmt.Sprintf("Failed to create upload request for chunk %d", i+1)
			return fmt.Errorf("failed to create upload request for chunk %d: %v", i+1, err)
		}

		// Set headers for chunked upload
		uploadReq.Header.Set("Content-Type", "video/mp4")
		uploadReq.Header.Set("Content-Length", fmt.Sprintf("%d", n))
		uploadReq.Header.Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", startByte, startByte+int64(n)-1, fileSize))

		log.Printf("Uploading chunk %d/%d: bytes %d-%d/%d", i+1, totalChunks,
			startByte, startByte+int64(n)-1, fileSize)

		// Send the chunk
		uploadResp, err := uploadClient.Do(uploadReq)
		if err != nil {
			result.TikTok.Success = false
			result.TikTok.Error = fmt.Sprintf("Failed to upload chunk %d", i+1)
			return fmt.Errorf("failed to upload chunk %d: %v", i+1, err)
		}

		// Read response body
		uploadRespBody, _ := io.ReadAll(uploadResp.Body)
		uploadResp.Body.Close()

		// Check response status
		isLastChunk := (i == totalChunks-1)
		if isLastChunk {
			// Final chunk should return 200 OK or 201 Created
			if uploadResp.StatusCode != http.StatusOK && uploadResp.StatusCode != http.StatusCreated {
				result.TikTok.Success = false
				result.TikTok.Error = fmt.Sprintf("Failed to upload final chunk (HTTP %d)", uploadResp.StatusCode)
				return fmt.Errorf("failed to upload final chunk: status %d, response: %s",
					uploadResp.StatusCode, string(uploadRespBody))
			}
		} else {
			// Intermediate chunks should return 206 Partial Content
			if uploadResp.StatusCode != http.StatusPartialContent {
				result.TikTok.Success = false
				result.TikTok.Error = fmt.Sprintf("Failed to upload chunk %d (HTTP %d)", i+1, uploadResp.StatusCode)
				return fmt.Errorf("failed to upload chunk %d: expected status 206, got %d, response: %s",
					i+1, uploadResp.StatusCode, string(uploadRespBody))
			}
		}

		log.Printf("Successfully uploaded chunk %d/%d", i+1, totalChunks)
	}

	// --- 5. Success! ---
	log.Printf("TikTok upload completed successfully. PublishID: %s", publishID)
	result.TikTok.Success = true
	result.TikTok.PostID = publishID
	return nil
}
