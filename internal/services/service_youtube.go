package services

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"os"
	"strings"
	"time"

	"uploader/internal/config"
	"uploader/internal/models"

	"golang.org/x/oauth2"
	"google.golang.org/api/option"
	"google.golang.org/api/youtube/v3"
)

// UploadToYoutube uploads a video to YouTube
func UploadToYoutube(file multipart.File, header *multipart.FileHeader,
	title, description, mainCaption string, result *models.UploadResult) error {

	log.Printf("Starting YouTube upload process for file: %s, size: %d bytes", header.Filename, header.Size)

	// Check if file is provided
	if file == nil || header == nil {
		log.Printf("YouTube upload failed: no file provided")
		return fmt.Errorf("no video file provided")
	}

	// Check file size (YouTube has a limit of 256GB)
	if header.Size > 256*1024*1024*1024 {
		log.Printf("YouTube upload failed: file size %d bytes exceeds YouTube's maximum limit of 256GB", header.Size)
		return fmt.Errorf("file size exceeds YouTube's maximum limit of 256GB")
	}

	// Check file type
	if !strings.HasSuffix(strings.ToLower(header.Filename), ".mp4") {
		log.Printf("YouTube upload failed: file type %s is not supported", header.Filename)
		return fmt.Errorf("only MP4 files are supported")
	}

	log.Printf("Reading YouTube authentication token")
	tokenFile, err := os.ReadFile("youtube_token.json")
	if err != nil {
		log.Printf("YouTube upload failed: authentication token file not found: %v", err)
		return fmt.Errorf("YouTube authentication required: %v", err)
	}

	var token oauth2.Token
	err = json.Unmarshal(tokenFile, &token)
	if err != nil {
		log.Printf("YouTube upload failed: invalid authentication token format: %v", err)
		return fmt.Errorf("invalid YouTube authentication token: %v", err)
	}

	// Check if token is expired
	if time.Now().After(token.Expiry) {
		log.Printf("YouTube upload failed: authentication token expired at %v", token.Expiry)
		return fmt.Errorf("YouTube authentication token has expired, please login again")
	}

	log.Printf("Initializing YouTube service with OAuth client")
	cfg := config.Get()
	client := cfg.YouTubeOAuthConfig.Client(context.Background(), &token)
	service, err := youtube.NewService(context.Background(), option.WithHTTPClient(client))
	if err != nil {
		log.Printf("YouTube upload failed: service initialization error: %v", err)
		return fmt.Errorf("failed to initialize YouTube service: %v", err)
	}

	// Validate title
	if title == "" {
		log.Printf("YouTube upload failed: title is required")
		return fmt.Errorf("video title is required")
	}

	// Use main caption if no specific description provided
	if description == "" {
		description = mainCaption
		log.Printf("Using main caption as description")
	}

	log.Printf("Preparing video metadata: title='%s', description length=%d", title, len(description))
	upload := &youtube.Video{
		Snippet: &youtube.VideoSnippet{
			Title:       title,
			Description: description,
			CategoryId:  "22", // People & Blogs category
		},
		Status: &youtube.VideoStatus{PrivacyStatus: "private"},
	}

	log.Printf("Starting YouTube API upload call")
	call := service.Videos.Insert([]string{"snippet", "status"}, upload)

	// Create a progress reader to track upload progress
	progressReader := &ProgressReader{
		Reader: file,
		Total:  header.Size,
		OnProgress: func(current, total int64) {
			percent := float64(current) / float64(total) * 100
			log.Printf("YouTube upload progress: %.2f%% (%d/%d bytes)", percent, current, total)
		},
	}

	response, err := call.Media(progressReader).Do()
	if err != nil {
		// Check for specific YouTube API errors
		if strings.Contains(err.Error(), "quotaExceeded") {
			log.Printf("YouTube upload failed: API quota exceeded")
			return fmt.Errorf("YouTube API quota exceeded, please try again later")
		} else if strings.Contains(err.Error(), "invalidCredentials") {
			log.Printf("YouTube upload failed: invalid credentials")
			return fmt.Errorf("YouTube authentication failed, please login again")
		} else if strings.Contains(err.Error(), "invalidContent") {
			log.Printf("YouTube upload failed: invalid content: %v", err)
			return fmt.Errorf("invalid video content: %v", err)
		}
		log.Printf("YouTube upload failed with error: %v", err)
		return fmt.Errorf("failed to upload to YouTube: %v", err)
	}

	log.Printf("YouTube upload completed successfully. Video ID: %s", response.Id)
	result.YouTube.Success = true
	result.YouTube.VideoID = response.Id
	return nil
}

// ProgressReader is a wrapper around io.Reader that tracks progress
type ProgressReader struct {
	Reader     io.Reader
	Total      int64
	Current    int64
	OnProgress func(current, total int64)
}

func (pr *ProgressReader) Read(p []byte) (int, error) {
	n, err := pr.Reader.Read(p)
	pr.Current += int64(n)
	if pr.OnProgress != nil {
		pr.OnProgress(pr.Current, pr.Total)
	}
	return n, err
}
