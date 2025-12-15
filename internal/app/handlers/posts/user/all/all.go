package posts_user_all

import (
	"feed/internal/lib/api/image"
	"feed/internal/lib/api/post"
	"feed/internal/lib/api/response"
	"feed/internal/lib/api/tag"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/render"
	"github.com/google/uuid"
)

type Response struct {
	response.Response
	Posts      []post.PostResponse `json:"posts"`
	NextCursor string              `json:"next_cursor,omitempty"`
	HasNext    bool                `json:"has_next"`
	Limit      int                 `json:"limit"`
}

type PostDBGetter interface {
	GetUserPublicPosts(profileID string, cursorTime time.Time, limit int) ([]post.UserPost, error)
	GetUserPosts(profileID string, cursorTime time.Time, limit int) ([]post.UserPost, error)
	GetPostsImages(postIDs []string) ([]string, []string, map[string]map[string]string, map[string][]image.Image, error)
	GetImagesTags(postImageIDs []string) (map[string][]string, error)
	GetPostsVotes(postIDs []string, viewerProfileID string) (map[string]int, error)
	GetImagesVotes(postImageIDs []string, viewerProfileID string) (map[string]int, error)
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

	errInvalidCursorFormat = "cursor must be valid RFC3339 timestamp"
	errInvalidLimitFormat  = "limit must be positive integer"
)

// @Summary Get all posts for a user
// @Description Retrieves all posts for a specific user with cursor-based pagination. Returns public posts for any viewer, but includes drafts if the viewer is the post owner. Includes detailed post information, images, tags, and vote status.
// @Tags Posts
// @Accept json
// @Produce json
// @Param X-Profile-ID header string true "Profile ID of the user whose posts are being retrieved" format(uuid)
// @Param X-Viewer-Profile-ID header string true "Profile ID of the viewer (used for access control and vote information)" format(uuid)
// @Param cursor query string false "Pagination cursor (RFC3339 timestamp) - returns posts created before this time" format(date-time)
// @Param limit query integer false "Number of posts to return (1-100, default: 20)" minimum(1) maximum(100) default(20)
// @Success 200 {object} Response "Posts retrieved successfully with pagination metadata"
// @Failure 400 {object} response.Response "Bad request - invalid parameters or malformed request"
// @Failure 401 {object} response.Response "Unauthorized - missing or invalid authentication headers"
// @Failure 500 {object} response.Response "Internal server error - database or service failure"
// @Security BearerAuth
// @Router /posts/user/all [get]
func New(log *slog.Logger, postDBGetter PostDBGetter, imageService ImageService, tagService TagService, uuidService UUIDService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		const op = "handlers.posts.user.all.New"

		log = log.With(
			slog.String("op", op),
			slog.String("request_id", middleware.GetReqID(r.Context())),
		)

		profileID := r.Header.Get("X-Profile-ID")
		if profileID == "" {
			log.Error("X-Profile-ID header is required")
			render.Status(r, http.StatusBadRequest)
			render.JSON(w, r, response.Error("X-Profile-ID header is required"))
			return
		}

		if _, err := uuid.Parse(profileID); err != nil {
			log.Error("invalid profile id format", slog.String("error", err.Error()))
			render.Status(r, http.StatusBadRequest)
			render.JSON(w, r, response.Error("invalid profile id format"))
			return
		}

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

		isOwnerViewing := viewerProfileID == profileID

		cursorTime, limit, errMsg, err := parseQueryParams(r)
		if err != nil {
			log.Error("failed to parse query params", slog.String("error", err.Error()))
			render.Status(r, http.StatusBadRequest)
			render.JSON(w, r, response.Error(fmt.Sprintf("invalid query params: %s", errMsg)))
			return
		}

		var userPosts []post.UserPost
		if isOwnerViewing {
			userPosts, err = postDBGetter.GetUserPosts(profileID, cursorTime, limit)
		} else {
			userPosts, err = postDBGetter.GetUserPublicPosts(profileID, cursorTime, limit)
		}

		if err != nil {
			log.Error("failed to get user posts", slog.String("error", err.Error()))
			render.Status(r, http.StatusInternalServerError)
			render.JSON(w, r, response.Error("failed to get user posts"))
			return
		}

		if len(userPosts) == 0 {
			log.Info("no posts found for user")
			render.Status(r, http.StatusOK)
			render.JSON(w, r, Response{
				Response: response.OK(),
				Posts:    []post.PostResponse{},
				HasNext:  false,
				Limit:    limit,
			})
			return
		}

		var nextCursor string
		hasNext := false
		if len(userPosts) >= limit {
			lastPost := userPosts[len(userPosts)-1]
			nextCursor = lastPost.CreatedAt.UTC().Format(time.RFC3339)
			hasNext = true
		}

		postIDs := make([]string, 0, len(userPosts))
		for _, post := range userPosts {
			postIDs = append(postIDs, post.ID)
		}

		postVotesMap := make(map[string]int)
		if viewerProfileID != "" {
			votes, err := postDBGetter.GetPostsVotes(postIDs, viewerProfileID)
			if err != nil {
				log.Error("failed to get posts votes", slog.String("error", err.Error()))
			} else {
				postVotesMap = votes
			}
		}

		postImageIDs, allImageIDs, postsPostImageIDsMap, postsImagesMap, err := postDBGetter.GetPostsImages(postIDs)
		if err != nil {
			log.Error("failed to get posts images", slog.String("error", err.Error()))
			render.Status(r, http.StatusInternalServerError)
			render.JSON(w, r, response.Error("failed to get posts images"))
			return
		}

		uniqueImageIDs := uuidService.DeduplicateIDs(allImageIDs)

		postImageVotesMap := make(map[string]int)
		if viewerProfileID != "" && len(postImageIDs) > 0 {
			votes, err := postDBGetter.GetImagesVotes(postImageIDs, viewerProfileID)
			if err != nil {
				log.Error("failed to get images votes", slog.String("error", err.Error()))
			} else {
				postImageVotesMap = votes
			}
		}

		postImageTagsMap := make(map[string][]string)
		if len(allImageIDs) > 0 {
			postImageTagsMap, err = postDBGetter.GetImagesTags(postImageIDs)
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

		postsResponse := make([]post.PostResponse, 0, len(userPosts))

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

			postResponse := post.PostResponse{
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

		log.Info("user posts retrieved successfully",
			slog.String("cursor", cursorTime.UTC().Format(time.RFC3339)),
			slog.Int("limit", limit),
			slog.Int("posts_count", len(postsResponse)),
			slog.Int("total_images", len(allImageIDs)),
			slog.Int("total_tags", len(allTagIDs)),
			slog.Bool("has_next", hasNext))

		render.Status(r, http.StatusOK)
		render.JSON(w, r, Response{
			Response:   response.OK(),
			Posts:      postsResponse,
			NextCursor: nextCursor,
			HasNext:    hasNext,
			Limit:      limit,
		})
	}
}

func parseQueryParams(r *http.Request) (time.Time, int, string, error) {
	const op = "handlers.posts.user.all.parseQueryParams"

	cursorTime := time.Now().UTC()
	limit := defaultLimit

	query := r.URL.Query()

	if cursorStr := query.Get("cursor"); cursorStr != "" {
		t, err := time.Parse(time.RFC3339, cursorStr)
		if err != nil {
			return time.Time{}, 0, errInvalidCursorFormat, fmt.Errorf("%s: cursor must be valid RFC3339 timestamp: %w", op, err)
		}
		cursorTime = t.UTC()
	}

	if limitStr := query.Get("limit"); limitStr != "" {
		l, err := strconv.Atoi(limitStr)
		if err != nil || l < 1 {
			return time.Time{}, 0, errInvalidLimitFormat, fmt.Errorf("%s: limit must be positive integer: %w", op, err)
		}
		if l > maxLimit {
			l = maxLimit
		}
		limit = l
	}
	return cursorTime, limit, "", nil
}
