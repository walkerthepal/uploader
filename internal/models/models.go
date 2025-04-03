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
	TikTok struct {
		Success bool   `json:"success"`
		PostID  string `json:"postId,omitempty"`
		Error   string `json:"error,omitempty"`
	} `json:"tiktok"`
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

// TikTokTokenResponse represents the OAuth token response from TikTok
type TikTokTokenResponse struct {
	AccessToken  string `json:"access_token"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
	OpenID       string `json:"open_id"`
	Scope        string `json:"scope"`
}
