package model

type ShortenRequest struct {
	URL string `json:"url" binding:"required"`
}

type ShortenResponse struct {
	Result string `json:"result"`
}
