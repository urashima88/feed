package image

import (
	"feed/internal/lib/api/tag"
	"time"
)

type ImageResponse struct {
	ImageID   string    `json:"image_id"`
	Width     int       `json:"width"`
	Height    int       `json:"height"`
	Extension string    `json:"extension"`
	CreatedAt time.Time `json:"created_at"`
	FileURL   string    `json:"file_url"`
}

type ImageWithTags struct {
	ImageID string    `json:"image_id"`
	Tags    []tag.Tag `json:"tags,omitempty"`
}

type Image struct {
	ImageID   string    `json:"image_id"`
	ProfileID string    `json:"profile_id,omitempty"`
	Score     int       `json:"score"`
	Width     int       `json:"width"`
	Height    int       `json:"height"`
	Extension string    `json:"extension"`
	CreatedAt time.Time `json:"created_at"`
	FileURL   string    `json:"file_url"`
	Tags      []tag.Tag `json:"tags,omitempty"`
	UserVote  *int      `json:"user_vote,omitempty"`
}

type ImageWithRelevance struct {
	ID        string
	ImageID   string
	Score     int
	CreatedAt time.Time
	ProfileID string
	PostID    string
	Relevance int
}

type ImageResponseWithRelevance struct {
	Image
	PostID    string `json:"post_id"`
	Relevance int    `json:"relevance"`
}
