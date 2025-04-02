package models

// UploadResult represents the result of uploading a video to various platforms
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

// InstagramTokenResponse represents the OAuth token response from Instagram
type InstagramTokenResponse struct {
	AccessToken string `json:"access_token"`
	UserID      string `json:"user_id"`
}

// InstagramMediaResponse represents the response from Instagram media endpoints
type InstagramMediaResponse struct {
	ID      string `json:"id"`
	Status  string `json:"status"`
	MediaID string `json:"media_id"`
}
