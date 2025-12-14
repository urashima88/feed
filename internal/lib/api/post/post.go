package post

import (
	"feed/internal/lib/api/image"
	"time"
)

type Post struct {
	ID        string
	ProfileID string
	Text      string
	IsDraft   bool
	CreatedAt time.Time
}

type UserPost struct {
	Post
	Score     int
	UpdatedAt time.Time
}

type CreatedPostResponse struct {
	ID        string                `json:"id"`
	ProfileID string                `json:"profile_id"`
	Text      string                `json:"text"`
	Score     int                   `json:"score"`
	IsDraft   bool                  `json:"is_draft"`
	CreatedAt time.Time             `json:"created_at"`
	UpdatedAt time.Time             `json:"updated_at"`
	Images    []image.ImageWithTags `json:"images"`
	UserVote  *int                  `json:"user_vote,omitempty"`
}

type PostResponse struct {
	ID        string        `json:"id"`
	ProfileID string        `json:"profile_id"`
	Text      string        `json:"text"`
	Score     int           `json:"score"`
	IsDraft   bool          `json:"is_draft"`
	CreatedAt time.Time     `json:"created_at"`
	UpdatedAt time.Time     `json:"updated_at"`
	Images    []image.Image `json:"images"`
	UserVote  *int          `json:"user_vote,omitempty"`
}

type FeedPostResponse struct {
	ID        string        `json:"id"`
	ProfileID string        `json:"profile_id"`
	Text      string        `json:"text"`
	Score     int           `json:"score"`
	IsDraft   bool          `json:"is_draft"`
	CreatedAt time.Time     `json:"created_at"`
	UpdatedAt time.Time     `json:"updated_at"`
	Images    []image.Image `json:"images"`
	UserVote  *int          `json:"user_vote,omitempty"`
}

type UserPostWithRelevance struct {
	UserPost
	Relevance int
}

type PostResponseWithRelevance struct {
	PostResponse
	Relevance int `json:"relevance"`
}
