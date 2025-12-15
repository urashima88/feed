package search_posts

import (
	"feed/internal/lib/api/image"
	"feed/internal/lib/api/post"
	"feed/internal/lib/api/response"
	"feed/internal/lib/api/tag"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/render"
	"github.com/google/uuid"
)

type Response struct {
	response.Response
	Posts         []post.PostResponseWithRelevance `json:"posts"`
	NextCursor    string                           `json:"next_cursor,omitempty"`
	NextRelevance int                              `json:"next_relevance,omitempty"`
	HasNext       bool                             `json:"has_next"`
	Limit         int                              `json:"limit"`
}

type SearchDBGetter interface {
	SearchPostsByTagsRelevance(tagIDs []string, cursorRelevance int, cursorTime time.Time, limit int) ([]post.UserPostWithRelevance, error)
	GetPostsImages(postIDs []string) ([]string, []string, map[string]map[string]string, map[string][]image.Image, error)
	GetImagesTags(postImageIDs []string) (map[string][]string, error)
	GetPostsVotes(postIDs []string, viewerProfileID string) (map[string]int, error)
	GetImagesVotes(postImageIDs []string, viewerProfileID string) (map[string]int, error)
}

type ImageService interface {
	GetImages(imageIDs []string) ([]image.ImageResponse, error)
}

type TagService interface {
	CreateTags(tags []string) ([]tag.Tag, error)
	GetTags(tagIDs []string) ([]tag.Tag, error)
}

type UUIDService interface {
	DeduplicateIDs(ids []string) []string
}

const (
	defaultLimit         = 20
	maxLimit             = 100
	startCursorRelevance = 200

	errInvalidCursorFormat = "cursor must be in format 'relevance_timestamp' or 'timestamp'"
	errInvalidLimitFormat  = "limit must be positive integer"
)

// @Summary Search posts by tags
// @Description Searches for posts based on tag relevance. Posts are ranked by how many of the requested tags they contain. Supports pagination using a combined cursor of relevance score and timestamp. Tags are validated and created if they don't exist.
// @Tags Search
// @Accept json
// @Produce json
// @Param X-Viewer-Profile-ID header string true "Profile ID of the viewer (for vote information)" format(uuid)
// @Param tags query string true "Comma-separated list of tags to search for" example:"nature,mountains,sunset"
// @Param cursor query string false "Pagination cursor in format 'relevance_timestamp' or just 'timestamp'" example:"5_2024-01-15T10:30:00Z"
// @Param limit query integer false "Number of posts to return (1-100, default: 20)" minimum(1) maximum(100) default(20)
// @Success 200 {object} Response "Posts retrieved successfully with relevance scoring and pagination"
// @Failure 400 {object} response.Response "Bad request - missing tags parameter or invalid cursor format"
// @Failure 401 {object} response.Response "Unauthorized - missing or invalid authentication headers"
// @Failure 500 {object} response.Response "Internal server error - database or service failure"
// @Security BearerAuth
// @Router /search/posts [get]
func New(log *slog.Logger, searchDBGetter SearchDBGetter, imageService ImageService, tagService TagService, uuidService UUIDService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		const op = "handlers.search.posts.New"

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

		tagsParam := r.URL.Query().Get("tags")
		if tagsParam == "" {
			log.Error("tags parameter is required")
			render.Status(r, http.StatusBadRequest)
			render.JSON(w, r, response.Error("tags parameter is required"))
			return
		}

		tagNames := strings.Split(tagsParam, ",")
		for i, tagName := range tagNames {
			tagNames[i] = strings.TrimSpace(tagName)
		}

		tags, err := tagService.CreateTags(tagNames)
		if err != nil {
			log.Error("failed to validate tags", slog.String("error", err.Error()))
			render.Status(r, http.StatusInternalServerError)
			render.JSON(w, r, response.Error("failed to validate tags"))
			return
		}

		if len(tags) == 0 {
			log.Info("no valid tags found for search")
			render.Status(r, http.StatusOK)
			render.JSON(w, r, Response{
				Response: response.OK(),
				Posts:    []post.PostResponseWithRelevance{},
				HasNext:  false,
				Limit:    defaultLimit,
			})
			return
		}

		tagIDs := make([]string, 0, len(tags))
		tagsMap := make(map[string]tag.Tag)
		for _, t := range tags {
			tagIDs = append(tagIDs, t.ID)
			tagsMap[t.ID] = t
		}

		cursorRelevance, cursorTime, limit, errMsg, err := parseQueryParams(r, len(tagIDs))
		if err != nil {
			log.Error("failed to parse query params", slog.String("error", err.Error()))
			render.Status(r, http.StatusBadRequest)
			render.JSON(w, r, response.Error(fmt.Sprintf("invalid query params: %s", errMsg)))
			return
		}

		userPosts, err := searchDBGetter.SearchPostsByTagsRelevance(tagIDs, cursorRelevance, cursorTime, limit)
		if err != nil {
			log.Error("failed to search posts by tags with relevance", slog.String("error", err.Error()))
			render.Status(r, http.StatusInternalServerError)
			render.JSON(w, r, response.Error("failed to search posts by tags"))
			return
		}

		if len(userPosts) == 0 {
			log.Info("no posts found for the given tags")
			render.Status(r, http.StatusOK)
			render.JSON(w, r, Response{
				Response: response.OK(),
				Posts:    []post.PostResponseWithRelevance{},
				HasNext:  false,
				Limit:    limit,
			})
			return
		}

		var nextCursor string
		var nextRelevance int
		hasNext := false
		if len(userPosts) >= limit {
			lastPost := userPosts[len(userPosts)-1]
			nextRelevance = lastPost.Relevance
			nextCursor = fmt.Sprintf("%d_%s", lastPost.Relevance, lastPost.CreatedAt.UTC().Format(time.RFC3339))
			hasNext = true
		}

		postIDs := make([]string, 0, len(userPosts))
		for _, post := range userPosts {
			postIDs = append(postIDs, post.ID)
		}

		postVotesMap := make(map[string]int)
		if viewerProfileID != "" {
			votes, err := searchDBGetter.GetPostsVotes(postIDs, viewerProfileID)
			if err != nil {
				log.Error("failed to get posts votes", slog.String("error", err.Error()))
			} else {
				postVotesMap = votes
			}
		}

		postImageIDs, allImageIDs, postsPostImageIDsMap, postsImagesMap, err := searchDBGetter.GetPostsImages(postIDs)
		if err != nil {
			log.Error("failed to get posts images", slog.String("error", err.Error()))
			render.Status(r, http.StatusInternalServerError)
			render.JSON(w, r, response.Error("failed to get posts images"))
			return
		}

		uniqueImageIDs := uuidService.DeduplicateIDs(allImageIDs)

		postImageVotesMap := make(map[string]int)
		if viewerProfileID != "" && len(postImageIDs) > 0 {
			votes, err := searchDBGetter.GetImagesVotes(postImageIDs, viewerProfileID)
			if err != nil {
				log.Error("failed to get images votes", slog.String("error", err.Error()))
			} else {
				postImageVotesMap = votes
			}
		}

		postImageTagsMap := make(map[string][]string)
		if len(allImageIDs) > 0 {
			postImageTagsMap, err = searchDBGetter.GetImagesTags(postImageIDs)
			if err != nil {
				log.Error("failed to get images tags", slog.String("error", err.Error()))
			}
		}

		allTagIDsFromImages := make([]string, 0)
		for _, tagIDs := range postImageTagsMap {
			allTagIDsFromImages = append(allTagIDsFromImages, tagIDs...)
		}

		uniqueTagIDsFromImages := uuidService.DeduplicateIDs(allTagIDsFromImages)

		imageTagsMap := make(map[string]tag.Tag)
		if len(uniqueTagIDsFromImages) > 0 && tagService != nil {
			tagsFromImages, err := tagService.GetTags(uniqueTagIDsFromImages)
			if err != nil {
				log.Error("failed to get tags info for images", slog.String("error", err.Error()))
			} else {
				for _, t := range tagsFromImages {
					imageTagsMap[t.ID] = t
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

		postsResponse := make([]post.PostResponseWithRelevance, 0, len(userPosts))

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
						if t, found := imageTagsMap[tagID]; found {
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

			postResponse := post.PostResponseWithRelevance{
				PostResponse: post.PostResponse{
					ID:        userPost.ID,
					ProfileID: userPost.ProfileID,
					Text:      userPost.Text,
					Score:     userPost.Score,
					IsDraft:   userPost.IsDraft,
					CreatedAt: userPost.CreatedAt,
					UpdatedAt: userPost.UpdatedAt,
					Images:    images,
				},
				Relevance: userPost.Relevance,
			}

			if voteValue, ok := postVotesMap[userPost.ID]; ok && voteValue != 0 {
				postResponse.UserVote = &voteValue
			}

			postsResponse = append(postsResponse, postResponse)
		}

		log.Info("posts search with relevance completed",
			slog.Int("requested_tags", len(tagNames)),
			slog.Int("found_tags", len(tags)),
			slog.Int("cursor_relevance", cursorRelevance),
			slog.String("cursor_time", cursorTime.UTC().Format(time.RFC3339)),
			slog.Int("limit", limit),
			slog.Int("posts_count", len(postsResponse)),
			slog.Int("total_images", len(allImageIDs)),
			slog.Bool("has_next", hasNext))

		render.Status(r, http.StatusOK)
		render.JSON(w, r, Response{
			Response:      response.OK(),
			Posts:         postsResponse,
			NextCursor:    nextCursor,
			NextRelevance: nextRelevance,
			HasNext:       hasNext,
			Limit:         limit,
		})
	}
}

func parseQueryParams(r *http.Request, maxRelevance int) (int, time.Time, int, string, error) {
	const op = "handlers.search.posts.parseQueryParams"

	cursorRelevance := maxRelevance
	if cursorRelevance == 0 {
		cursorRelevance = startCursorRelevance
	}

	cursorTime := time.Now().UTC()
	limit := defaultLimit

	query := r.URL.Query()

	if cursorStr := query.Get("cursor"); cursorStr != "" {
		parts := strings.Split(cursorStr, "_")
		if len(parts) == 2 {
			relevance, err := strconv.Atoi(parts[0])
			if err != nil || relevance < 0 {
				return 0, time.Time{}, 0, errInvalidCursorFormat, fmt.Errorf("%s: invalid relevance in cursor: %w", op, err)
			}
			cursorRelevance = relevance

			t, err := time.Parse(time.RFC3339, parts[1])
			if err != nil {
				return 0, time.Time{}, 0, errInvalidCursorFormat, fmt.Errorf("%s: invalid time in cursor: %w", op, err)
			}
			cursorTime = t.UTC()
		} else if len(parts) == 1 {
			t, err := time.Parse(time.RFC3339, cursorStr)
			if err != nil {
				return 0, time.Time{}, 0, errInvalidCursorFormat, fmt.Errorf("%s: cursor must be valid RFC3339 timestamp: %w", op, err)
			}
			cursorTime = t.UTC()
		} else {
			return 0, time.Time{}, 0, errInvalidCursorFormat, fmt.Errorf("%s: invalid cursor format", op)
		}

		if limitStr := query.Get("limit"); limitStr != "" {
			l, err := strconv.Atoi(limitStr)
			if err != nil || l < 1 {
				return 0, time.Time{}, 0, errInvalidLimitFormat, fmt.Errorf("%s: limit must be positive integer: %w", op, err)
			}
			if l > maxLimit {
				l = maxLimit
			}
			limit = l
		}
	}

	return cursorRelevance, cursorTime, limit, "", nil
}
