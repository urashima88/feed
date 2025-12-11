package image

import "feed/internal/lib/api/tag"

type ImageResponse struct {
	ImageID   string `json:"image_id"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
	Extension string `json:"extension"`
	CreatedAt string `json:"created_at"`
	FileURL   string `json:"file_url"`
}

type ImageWithTags struct {
	ImageID string    `json:"image_id"`
	Tags    []tag.Tag `json:"tags,omitempty"`
}

type Image struct {
	ImageID   string    `json:"image_id"`
	Score     int       `json:"score"`
	Width     int       `json:"width"`
	Height    int       `json:"height"`
	Extension string    `json:"extension"`
	CreatedAt string    `json:"created_at"`
	FileURL   string    `json:"file_url"`
	Tags      []tag.Tag `json:"tags,omitempty"`
	UserVote  *int      `json:"user_vote,omitempty"`
}
