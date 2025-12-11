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
	GetUserPublicPosts(profileID, cursor string, limit int) ([]post.UserPost, error)
	GetUserPosts(profileID, cursor string, limit int) ([]post.UserPost, error)
	GetPostsImages(postIDs []string) ([]string, []string, map[string]map[string]string, map[string][]image.Image, error)
	GetImagesTags(imageIDs []string) (map[string][]string, error)
	GetPostsVotes(postIDs []string, viewerProfileID string) (map[string]int, error)
	GetImagesVotes(imageIDs []string, viewerProfileID string) (map[string]int, error)
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
		if viewerProfileID != "" {
			if _, err := uuid.Parse(viewerProfileID); err != nil {
				log.Error("invalid viewer profile id format", slog.String("error", err.Error()))
				viewerProfileID = ""
			}
		}

		isOwnerViewing := (viewerProfileID != "" && viewerProfileID == profileID)

		cursor, limit, errMsg, err := parseQueryParams(r)
		if err != nil {
			log.Error("failed to parse query params", slog.String("error", err.Error()))
			render.Status(r, http.StatusBadRequest)
			render.JSON(w, r, response.Error(fmt.Sprintf("invalid query params: %s", errMsg)))
			return
		}

		var userPosts []post.UserPost
		if isOwnerViewing {
			userPosts, err = postDBGetter.GetUserPosts(profileID, cursor, limit)
		} else {
			userPosts, err = postDBGetter.GetUserPublicPosts(profileID, cursor, limit)
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
			nextCursor = lastPost.CreatedAt
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
			slog.String("cursor", cursor),
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

func parseQueryParams(r *http.Request) (string, int, string, error) {
	const op = "handlers.posts.user.all.parseQueryParams"

	cursor := time.Now().UTC().Format(time.RFC3339)
	limit := defaultLimit

	query := r.URL.Query()

	if cursorStr := query.Get("cursor"); cursorStr != "" {
		if _, err := time.Parse(time.RFC3339, cursorStr); err != nil {
			return "", 0, errInvalidCursorFormat, fmt.Errorf("%s: cursor must be valid RFC3339 timestamp: %w", op, err)
		}
		cursor = cursorStr
	}

	if limitStr := query.Get("limit"); limitStr != "" {
		l, err := strconv.Atoi(limitStr)
		if err != nil || l < 1 {
			return "", 0, errInvalidLimitFormat, fmt.Errorf("%s: limit must be positive integer: %w", op, err)
		}
		if l > maxLimit {
			l = maxLimit
		}
		limit = l
	}
	return cursor, limit, "", nil
}
