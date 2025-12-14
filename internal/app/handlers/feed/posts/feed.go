package feed_posts

import (
	"feed/internal/lib/api/image"
	"feed/internal/lib/api/post"
	"feed/internal/lib/api/response"
	"feed/internal/lib/api/tag"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/render"
	"github.com/google/uuid"
)

type Response struct {
	response.Response
	Posts      []post.FeedPostResponse `json:"posts"`
	NextCursor string                  `json:"next_cursor,omitempty"`
	NextScore  int                     `json:"next_score,omitempty"`
	HasNext    bool                    `json:"has_next"`
	Limit      int                     `json:"limit"`
}

type FeedDBGetter interface {
	GetFeedPosts(cursorTime time.Time, limit int) ([]post.UserPost, error)
	GetFeedPostsByScore(cursorScore int, cursorTime time.Time, limit int) ([]post.UserPost, error)
	GetPostsVotes(postIDs []string, viewerProfileID string) (map[string]int, error)
	GetPostsImages(postIDs []string) ([]string, []string, map[string]map[string]string, map[string][]image.Image, error)
	GetImagesVotes(postImageIDs []string, viewerProfileID string) (map[string]int, error)
	GetImagesTags(postImageIDs []string) (map[string][]string, error)
}

type ImageService interface {
	GetImages(imageIDs []string) ([]image.ImageResponse, error)
}

type TagService interface {
	GetTags(tagIDs []string) ([]tag.Tag, error)
}

type UUIDService interface {
	DeduplicateIDs(ids []string) []string
}

const (
	defaultLimit = 20
	maxLimit     = 100

	errInvalidCursorFormat      = "cursor must be valid RFC3339 timestamp"
	errInvalidLimitFormat       = "limit must be positive integer"
	errInvalidScoreCursorFormat = "score_cursor must be positive integer"
)

func New(log *slog.Logger, feedDBGetter FeedDBGetter, imageService ImageService, tagService TagService, uuidService UUIDService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		const op = "handlers.feed.posts.New"

		log = log.With(
			slog.String("op", op),
			slog.String("request_id", middleware.GetReqID(r.Context())),
		)

		viewerProfileID := r.Header.Get("X-Viewer-Profile-ID")
		if viewerProfileID == "" {
			log.Error("X-Viewer-Profile-ID header is required")
			render.Status(r, http.StatusUnauthorized)
			render.JSON(w, r, response.Error("X-Viewer-Profile-ID header is required"))
			return
		}

		if _, err := uuid.Parse(viewerProfileID); err != nil {
			log.Error("invalid viewer profile id format", slog.String("error", err.Error()))
			render.Status(r, http.StatusBadRequest)
			render.JSON(w, r, response.Error("invalid viewer profile id format"))
			return
		}

		sortType := r.URL.Query().Get("sort")

		cursorTime, cursorScore, limit, errMsg, err := parseQueryParams(r, sortType)

		if err != nil {
			log.Error("failed to parse query params", slog.String("error", err.Error()))
			render.Status(r, http.StatusBadRequest)
			render.JSON(w, r, response.Error(fmt.Sprintf("invalid query params: %s", errMsg)))
			return
		}

		var userPosts []post.UserPost
		switch {
		case sortType == "score" || sortType == "popular":
			userPosts, err = feedDBGetter.GetFeedPostsByScore(cursorScore, cursorTime, limit)
		default:
			userPosts, err = feedDBGetter.GetFeedPosts(cursorTime, limit)
		}

		if err != nil {
			log.Error("failed to get feed posts", slog.String("error", err.Error()))
			render.Status(r, http.StatusInternalServerError)
			render.JSON(w, r, response.Error("failed to get feed posts"))
			return
		}

		if len(userPosts) == 0 {
			log.Info("no posts found in feed")
			render.Status(r, http.StatusOK)
			render.JSON(w, r, Response{
				Response: response.OK(),
				Posts:    []post.FeedPostResponse{},
				HasNext:  false,
				Limit:    limit,
			})
			return
		}

		var nextCursor string
		var nextScore int
		hasNext := false

		if len(userPosts) >= limit {
			lastPost := userPosts[len(userPosts)-1]
			nextCursor = lastPost.CreatedAt.UTC().Format(time.RFC3339)
			if sortType == "score" || sortType == "popular" {
				nextScore = lastPost.Score
			}
			hasNext = true
		}

		postIDs := make([]string, 0, len(userPosts))
		for _, post := range userPosts {
			postIDs = append(postIDs, post.ID)
		}

		postVotesMap := make(map[string]int)
		if viewerProfileID != "" {
			votes, err := feedDBGetter.GetPostsVotes(postIDs, viewerProfileID)
			if err != nil {
				log.Error("failed to get posts votes", slog.String("error", err.Error()))
			} else {
				postVotesMap = votes
			}
		}

		postImageIDs, allImageIDs, postsPostImageIDsMap, postsImagesMap, err := feedDBGetter.GetPostsImages(postIDs)
		if err != nil {
			log.Error("failed to get posts images", slog.String("error", err.Error()))
			render.Status(r, http.StatusInternalServerError)
			render.JSON(w, r, response.Error("failed to get posts images"))
			return
		}

		uniqueImageIDs := uuidService.DeduplicateIDs(allImageIDs)

		postImageVotesMap := make(map[string]int)
		if viewerProfileID != "" && len(postImageIDs) > 0 {
			votes, err := feedDBGetter.GetImagesVotes(postImageIDs, viewerProfileID)
			if err != nil {
				log.Error("failed to get images votes", slog.String("error", err.Error()))
			} else {
				postImageVotesMap = votes
			}
		}

		postImageTagsMap := make(map[string][]string)
		if len(allImageIDs) > 0 {
			postImageTagsMap, err = feedDBGetter.GetImagesTags(postImageIDs)
			if err != nil {
				log.Error("failed to get images tags", slog.String("error", err.Error()))
			}
		}

		allTagIDs := make([]string, 0)
		for _, tagIDs := range postImageTagsMap {
			allTagIDs = append(allTagIDs, tagIDs...)
		}

		uniqueTagIDs := uuidService.DeduplicateIDs(allTagIDs)

		tagsMap := make(map[string]tag.Tag)
		if len(uniqueTagIDs) > 0 && tagService != nil {
			tags, err := tagService.GetTags(uniqueTagIDs)
			if err != nil {
				log.Error("failed to get tags info", slog.String("error", err.Error()))
			} else {
				for _, t := range tags {
					tagsMap[t.ID] = t
				}
			}
		}

		imagesInfoMap := make(map[string]image.ImageResponse)
		if len(uniqueImageIDs) > 0 && imageService != nil {
			imagesInfo, err := imageService.GetImages(uniqueImageIDs)
			if err != nil {
				log.Error("failed to get images info", slog.String("error", err.Error()))
			} else {
				for _, imageInfo := range imagesInfo {
					imagesInfoMap[imageInfo.ImageID] = imageInfo
				}
			}
		}

		postsResponse := make([]post.FeedPostResponse, 0, len(userPosts))

		for _, userPost := range userPosts {
			postImages := postsImagesMap[userPost.ID]
			imageIDsPostImageIDsMap := postsPostImageIDsMap[userPost.ID]
			images := make([]image.Image, 0, len(postImages))

			for _, img := range postImages {
				img.ProfileID = userPost.ProfileID
				if imageInfo, ok := imagesInfoMap[img.ImageID]; ok {
					img.Width = imageInfo.Width
					img.Height = imageInfo.Height
					img.Extension = imageInfo.Extension
					img.CreatedAt = imageInfo.CreatedAt
					img.FileURL = imageInfo.FileURL
				}

				if tagIDs, ok := postImageTagsMap[imageIDsPostImageIDsMap[img.ImageID]]; ok {
					tags := make([]tag.Tag, 0, len(tagIDs))
					for _, tagID := range tagIDs {
						if t, found := tagsMap[tagID]; found {
							tags = append(tags, t)
						}
					}
					img.Tags = tags
				}

				if voteValue, ok := postImageVotesMap[imageIDsPostImageIDsMap[img.ImageID]]; ok && voteValue != 0 {
					img.UserVote = &voteValue
				}

				images = append(images, img)
			}

			postResponse := post.FeedPostResponse{
				ID:        userPost.ID,
				ProfileID: userPost.ProfileID,
				Text:      userPost.Text,
				Score:     userPost.Score,
				IsDraft:   userPost.IsDraft,
				CreatedAt: userPost.CreatedAt,
				UpdatedAt: userPost.UpdatedAt,
				Images:    images,
			}

			if voteValue, ok := postVotesMap[userPost.ID]; ok && voteValue != 0 {
				postResponse.UserVote = &voteValue
			}

			postsResponse = append(postsResponse, postResponse)
		}

		log.Info("feed posts retrieved successfully",
			slog.String("sort", sortType),
			slog.String("cursor", cursorTime.UTC().Format(time.RFC3339)),
			slog.Int("cursor_score", cursorScore),
			slog.Int("limit", limit),
			slog.Int("posts_count", len(postsResponse)),
			slog.Int("total_images", len(allImageIDs)),
			slog.Int("total_tags", len(allTagIDs)),
			slog.Bool("has_next", hasNext))

		responseData := Response{
			Response:   response.OK(),
			Posts:      postsResponse,
			NextCursor: nextCursor,
			HasNext:    hasNext,
			Limit:      limit,
		}

		if sortType == "score" || sortType == "popular" {
			responseData.NextScore = nextScore
		}

		render.Status(r, http.StatusOK)
		render.JSON(w, r, responseData)
	}
}

func parseQueryParams(r *http.Request, sortType string) (time.Time, int, int, string, error) {
	const op = "handlers.feed.parseQueryParams"

	cursorTime := time.Now().UTC()
	cursorScore := math.MaxInt32
	limit := defaultLimit

	query := r.URL.Query()

	if cursorStr := query.Get("cursor"); cursorStr != "" {
		t, err := time.Parse(time.RFC3339, cursorStr)
		if err != nil {
			return time.Time{}, 0, 0, errInvalidCursorFormat, fmt.Errorf("%s: cursor must be valid RFC3339 timestamp: %w", op, err)
		}
		cursorTime = t.UTC()
	}

	if scoreStr := query.Get("score_cursor"); scoreStr != "" && (sortType == "score" || sortType == "popular") {
		score, err := strconv.Atoi(scoreStr)
		if err != nil || score < 0 {
			return time.Time{}, 0, 0, errInvalidScoreCursorFormat, fmt.Errorf("%s: score_cursor must be positive integer: %w", op, err)
		}
		cursorScore = score
	}

	if limitStr := query.Get("limit"); limitStr != "" {
		l, err := strconv.Atoi(limitStr)
		if err != nil || l < 1 {
			return time.Time{}, 0, 0, errInvalidLimitFormat, fmt.Errorf("%s: limit must be positive integer: %w", op, err)
		}
		if l > maxLimit {
			l = maxLimit
		}
		limit = l
	}

	return cursorTime, cursorScore, limit, "", nil
}
