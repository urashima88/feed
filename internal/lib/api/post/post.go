package post

import (
	"feed/internal/lib/api/image"
)

type Post struct {
	ID        string
	Text      string
	IsDraft   bool
	CreatedAt string
}

type UserPost struct {
	Post
	Score     int
	UpdatedAt string
}

type CreatedPostResponse struct {
	ID        string                `json:"id"`
	Text      string                `json:"text"`
	Score     int                   `json:"score"`
	IsDraft   bool                  `json:"is_draft"`
	CreatedAt string                `json:"created_at"`
	UpdatedAt string                `json:"updated_at"`
	Images    []image.ImageWithTags `json:"images"`
	UserVote  *int                  `json:"user_vote,omitempty"`
}

type PostResponse struct {
	ID        string        `json:"id"`
	Text      string        `json:"text"`
	Score     int           `json:"score"`
	IsDraft   bool          `json:"is_draft"`
	CreatedAt string        `json:"created_at"`
	UpdatedAt string        `json:"updated_at"`
	Images    []image.Image `json:"images"`
	UserVote  *int          `json:"user_vote,omitempty"`
}
