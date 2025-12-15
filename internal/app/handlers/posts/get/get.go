package posts_get

import (
	"feed/internal/lib/api/image"
	"feed/internal/lib/api/post"
	"feed/internal/lib/api/response"
	"feed/internal/lib/api/tag"
	"feed/internal/lib/logger/sl"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/render"
	"github.com/google/uuid"
)

type Response struct {
	response.Response
	Post *post.PostResponse `json:"post"`
}

type PostDBGetter interface {
	GetPostByID(postID string) (*post.UserPost, error)
	GetPostImages(postID string) ([]string, []string, map[string]string, []image.Image, error)
	GetImagesTags(imageIDs []string) (map[string][]string, error)
	GetPostVote(postID string, viewerProfileID string) (int, error)
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

// @Summary Get a specific post by ID
// @Description Retrieves detailed information about a specific post including images, tags, votes, and metadata. Draft posts are only accessible by the post owner. Returns comprehensive post data with user-specific vote information.
// @Tags Posts
// @Accept json
// @Produce json
// @Param X-Viewer-Profile-ID header string true "Profile ID of the viewer (used for access control and vote information)" format(uuid)
// @Param post_id path string true "Post ID to retrieve" format(uuid)
// @Success 200 {object} Response "Post retrieved successfully"
// @Failure 400 {object} response.Response "Bad request - invalid post ID format"
// @Failure 401 {object} response.Response "Unauthorized - missing or invalid authentication"
// @Failure 404 {object} response.Response "Post not found - either doesn't exist or viewer doesn't have access"
// @Failure 500 {object} response.Response "Internal server error - database or service failure"
// @Security BearerAuth
// @Router /posts/{post_id} [get]
func New(log *slog.Logger, postDBGetter PostDBGetter, imageService ImageService, tagService TagService, uuidService UUIDService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		const op = "handlers.posts.get.New"

		log = log.With(
			slog.String("op", op),
			slog.String("request_id", middleware.GetReqID(r.Context())),
		)

		postID := chi.URLParam(r, "post_id")
		if postID == "" {
			log.Error("post_id is required")
			render.Status(r, http.StatusBadRequest)
			render.JSON(w, r, response.Error("post_id is required"))
			return
		}

		if _, err := uuid.Parse(postID); err != nil {
			log.Error("invalid post id format", sl.Err(err))
			render.Status(r, http.StatusBadRequest)
			render.JSON(w, r, response.Error("invalid post id format"))
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

		userPost, err := postDBGetter.GetPostByID(postID)
		if err != nil {
			log.Error("failed to get post", slog.String("error", err.Error()))
			render.Status(r, http.StatusInternalServerError)
			render.JSON(w, r, response.Error("failed to get post"))
			return
		}

		if userPost == nil {
			log.Info("post not found", slog.String("post_id", postID))
			render.Status(r, http.StatusNotFound)
			render.JSON(w, r, response.Error("post not found"))
			return
		}

		if userPost.IsDraft {
			if viewerProfileID == "" || viewerProfileID != userPost.ProfileID {
				log.Warn("attempt to access draft post by non-owner", slog.String("post_id", postID))
				render.Status(r, http.StatusNotFound)
				render.JSON(w, r, response.Error("post not found"))
				return
			}
		}

		postImageIDs, allImageIDs, imageIDsPostImageIDsMap, postImages, err := postDBGetter.GetPostImages(postID)
		if err != nil {
			log.Error("failed to get post images", slog.String("error", err.Error()))
			render.Status(r, http.StatusInternalServerError)
			render.JSON(w, r, response.Error("failed to get post images"))
			return
		}

		var postVote int
		if viewerProfileID != "" {
			vote, err := postDBGetter.GetPostVote(postID, viewerProfileID)
			if err != nil {
				log.Error("failed to get post vote", slog.String("error", err.Error()))
			} else {
				postVote = vote
			}
		}

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
		if len(postImageIDs) > 0 {
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

		uniqueImageIDs := uuidService.DeduplicateIDs(allImageIDs)

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

		images := make([]image.Image, 0)
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

		postResponse := &post.PostResponse{
			ID:        userPost.ID,
			ProfileID: userPost.ProfileID,
			Text:      userPost.Text,
			Score:     userPost.Score,
			IsDraft:   userPost.IsDraft,
			CreatedAt: userPost.CreatedAt,
			UpdatedAt: userPost.UpdatedAt,
			Images:    images,
		}

		if postVote != 0 {
			postResponse.UserVote = &postVote
		}

		render.Status(r, http.StatusOK)
		render.JSON(w, r, Response{
			Response: response.OK(),
			Post:     postResponse,
		})
	}
}
