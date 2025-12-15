package posts_create

import (
	app_config "feed/internal/config/app-config"
	"feed/internal/lib/api/image"
	"feed/internal/lib/api/post"
	"feed/internal/lib/api/response"
	"feed/internal/lib/api/tag"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/render"
	"github.com/google/uuid"
)

type Request struct {
	ImageIDs []string            `json:"image_ids"`
	Text     string              `json:"text"`
	Tags     map[string][]string `json:"tags"`
	IsDraft  bool                `json:"is_draft"`
}

type Response struct {
	response.Response
	Post post.CreatedPostResponse `json:"post"`
}

type PostDBCreator interface {
	CreatePost(profileID, text string, isDraft bool, images []image.ImageWithTags) (*post.UserPost, error)
}

type ImageService interface {
	GetImages(imageIDs []string) ([]image.ImageResponse, error)
}

type TagService interface {
	CreateTags(tags []string) ([]tag.Tag, error)
}

type UUIDService interface {
	DeduplicateIDs(ids []string) []string
}

const (
	errZeroImages     = "at least one image is required"
	errTooManyImages  = "too many images were added"
	errTextIsTooLong  = "text is too long"
	errInvalidImageID = "invalid image id"
)

// @Summary Create a new post
// @Description Creates a new post with images and tags. Tags are validated and created in tag service. Images must exist in image service.
// @Tags Posts
// @Accept json
// @Produce json
// @Param X-Profile-ID header string true "Profile ID of the post creator" format(uuid)
// @Param request body Request true "Post creation data"
// @Success 201 {object} Response "Post created successfully"
// @Failure 400 {object} response.Response "Bad request - invalid input parameters or validation failed"
// @Failure 401 {object} response.Response "Unauthorized - missing or invalid authentication"
// @Failure 500 {object} response.Response "Internal server error - database or service failure"
// @Security BearerAuth
// @Router /posts [post]
func New(log *slog.Logger, postDBCreator PostDBCreator, imageService ImageService, tagService TagService, uuidService UUIDService, postMeta *app_config.PostMeta) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		const op = "handlers.posts.create.New"

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

		var req Request
		if err := render.DecodeJSON(r.Body, &req); err != nil {
			log.Error("failed to decode request body", slog.String("error", err.Error()))
			render.Status(r, http.StatusBadRequest)
			render.JSON(w, r, response.Error("invalid request format"))
			return
		}

		imageIDs, errMsg, err := validateRequest(req, uuidService, postMeta)
		if err != nil {
			log.Error("request validation failed", slog.String("error", err.Error()))
			render.Status(r, http.StatusBadRequest)
			render.JSON(w, r, response.Error(fmt.Sprintf("request validation failed: %s", errMsg)))
			return
		}

		allTags := make([]string, 0)
		tagsByImageID := make(map[string][]string)

		for _, imageID := range imageIDs {
			if tags, exists := req.Tags[imageID]; exists {
				if len(tags) > postMeta.MaxTagsPerImage {
					log.Warn("too many tags for image, truncating...",
						slog.String("image_id", imageID),
						slog.Int("requested", len(tags)),
						slog.Int("max", postMeta.MaxTagsPerImage))
					tags = tags[:postMeta.MaxTagsPerImage]
				}

				filteredTags := make([]string, 0, len(tags))
				uniqueTags := make(map[string]bool)
				for _, tag := range tags {
					if tag != "" {
						if uniqueTags[tag] {
							log.Warn("duplicated tag was found, skipping...", slog.String("tag", tag))
							continue
						}
						if len(tag) <= postMeta.MaxTagLength {
							filteredTags = append(filteredTags, tag)
							uniqueTags[tag] = true
						} else {
							log.Warn("tag is too long, skipping...",
								slog.String("tag", tag),
								slog.Int("length", len(tag)),
								slog.Int("max", postMeta.MaxTagLength))
						}
					} else {
						log.Warn("empty tag was found, skipping...")
					}
				}
				tagsByImageID[imageID] = filteredTags
				allTags = append(allTags, filteredTags...)
			}
		}

		var createdTags []tag.Tag
		if tagService != nil && len(allTags) > 0 {
			tags, err := tagService.CreateTags(allTags)
			if err != nil {
				if tags != nil && len(tags) > 0 {
					createdTags = tags
					log.Warn("some tags were created in tag service",
						slog.String("error", err.Error()),
						slog.Int("requested", len(allTags)),
						slog.Int("validated", len(createdTags)))
				} else {
					log.Warn("failed to create tags in tag service",
						slog.String("error", err.Error()))
					createdTags = []tag.Tag{}
				}
			} else {
				createdTags = tags
				log.Info("tags created in tag service",
					slog.Int("requested", len(allTags)),
					slog.Int("validated", len(createdTags)))
			}
		}

		tagsByName := make(map[string]tag.Tag)
		for _, t := range createdTags {
			tagsByName[t.Name] = t
		}

		images := make([]image.ImageWithTags, 0, len(imageIDs))
		for _, imageID := range imageIDs {
			img := image.ImageWithTags{
				ImageID: imageID,
				Tags:    []tag.Tag{},
			}

			if imageTags, exists := tagsByImageID[imageID]; exists {
				for _, tagName := range imageTags {
					if t, found := tagsByName[tagName]; found {
						img.Tags = append(img.Tags, t)
					}
				}
			}
			images = append(images, img)
		}

		createdPost, err := postDBCreator.CreatePost(profileID, req.Text, req.IsDraft, images)
		if err != nil {
			log.Error("failed to create post", slog.String("error", err.Error()))
			render.Status(r, http.StatusInternalServerError)
			render.JSON(w, r, response.Error("failed to create post"))
			return
		}

		postResponse := post.CreatedPostResponse{
			ID:        createdPost.ID,
			ProfileID: createdPost.ProfileID,
			Text:      createdPost.Text,
			Score:     createdPost.Score,
			IsDraft:   createdPost.IsDraft,
			CreatedAt: createdPost.CreatedAt,
			UpdatedAt: createdPost.UpdatedAt,
			Images:    images,
		}

		log.Info("post created successfully",
			slog.String("post_id", createdPost.ID),
			slog.Int("image_count", len(images)),
			slog.Int("tag_count", len(allTags)),
			slog.Bool("is_draft", createdPost.IsDraft))

		render.Status(r, http.StatusCreated)
		render.JSON(w, r, Response{
			Response: response.OK(),
			Post:     postResponse,
		})
	}
}

func validateRequest(req Request, uuidService UUIDService, postMeta *app_config.PostMeta) ([]string, string, error) {
	const op = "handlers.posts.create.validateRequest"

	if len(req.ImageIDs) == 0 {
		return nil,
			errZeroImages,
			fmt.Errorf("%s: at least one image is required", op)
	}

	req.ImageIDs = uuidService.DeduplicateIDs(req.ImageIDs)

	if len(req.ImageIDs) > postMeta.MaxNumberImages {
		return nil,
			fmt.Sprintf("%s (maximum %d images allowed)", errTooManyImages, postMeta.MaxNumberImages),
			fmt.Errorf("%s: %s (maximum %d images allowed)", op, errTooManyImages, postMeta.MaxNumberImages)
	}

	if len(req.Text) > postMeta.MaxTextLength {
		return nil,
			fmt.Sprintf("%s (maximum %d characters)", errTextIsTooLong, postMeta.MaxTextLength),
			fmt.Errorf("%s: %s (maximum %d characters)", op, errTextIsTooLong, postMeta.MaxTextLength)
	}

	var validImageIDs []string
	for _, imageID := range req.ImageIDs {
		if _, err := uuid.Parse(imageID); err != nil {
			return nil,
				fmt.Sprintf("%s, image_id: %s", errInvalidImageID, imageID),
				fmt.Errorf("%s: %s, image_id: %s", op, errInvalidImageID, imageID)
		}
		validImageIDs = append(validImageIDs, imageID)
	}

	return validImageIDs, "", nil
}
