package search_images

import (
	"feed/internal/lib/api/image"
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
	Images        []image.ImageResponseWithRelevance `json:"images"`
	NextCursor    string                             `json:"next_cursor,omitempty"`
	NextRelevance int                                `json:"next_relevance,omitempty"`
	HasNext       bool                               `json:"has_next"`
	Limit         int                                `json:"limit"`
}

type SearchDBGetter interface {
	SearchImagesByTagsRelevance(tagIDs []string, cursorRelevance int, cursorTime time.Time, limit int) ([]image.ImageWithRelevance, error)
	GetImagesTags(postImageIDs []string) (map[string][]string, error)
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

func New(log *slog.Logger, searchDBGetter SearchDBGetter, imageService ImageService, tagService TagService, uuidService UUIDService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		const op = "handlers.search.images.New"

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
				Images:   []image.ImageResponseWithRelevance{},
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

		images, err := searchDBGetter.SearchImagesByTagsRelevance(tagIDs, cursorRelevance, cursorTime, limit)
		if err != nil {
			log.Error("failed to search images by tags with relevance", slog.String("error", err.Error()))
			render.Status(r, http.StatusInternalServerError)
			render.JSON(w, r, response.Error("failed to search images by tags"))
			return
		}

		if len(images) == 0 {
			log.Info("no images found for the given tags")
			render.Status(r, http.StatusOK)
			render.JSON(w, r, Response{
				Response: response.OK(),
				Images:   []image.ImageResponseWithRelevance{},
				HasNext:  false,
				Limit:    limit,
			})
			return
		}

		var nextCursor string
		var nextRelevance int
		hasNext := false
		if len(images) >= limit {
			lastImage := images[len(images)-1]
			nextRelevance = lastImage.Relevance
			nextCursor = fmt.Sprintf("%d_%s", lastImage.Relevance, lastImage.CreatedAt.UTC().Format(time.RFC3339))
			hasNext = true
		}

		imageIDs := make([]string, 0, len(images))
		postImageIDs := make([]string, 0, len(images))
		for _, img := range images {
			imageIDs = append(imageIDs, img.ImageID)
			postImageIDs = append(postImageIDs, img.ID)
		}

		uniqueImageIDs := uuidService.DeduplicateIDs(imageIDs)

		imagesVotesMap := make(map[string]int)
		if viewerProfileID != "" && len(postImageIDs) > 0 {
			votes, err := searchDBGetter.GetImagesVotes(postImageIDs, viewerProfileID)
			if err != nil {
				log.Error("failed to get images votes", slog.String("error", err.Error()))
			} else {
				imagesVotesMap = votes
			}
		}

		imagesTagsMap := make(map[string][]string)
		if len(postImageIDs) > 0 {
			tags, err := searchDBGetter.GetImagesTags(postImageIDs)
			if err != nil {
				log.Error("failed to get images tags", slog.String("error", err.Error()))
			} else {
				imagesTagsMap = tags
			}
		}

		allTagIDsFromImages := make([]string, 0)
		for _, tagIDs := range imagesTagsMap {
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

		imagesResponse := make([]image.ImageResponseWithRelevance, 0, len(images))

		for _, img := range images {
			var imageResp image.ImageResponseWithRelevance

			if imageInfo, ok := imagesInfoMap[img.ImageID]; ok {
				imageResp = image.ImageResponseWithRelevance{
					Image: image.Image{
						ImageID:   imageInfo.ImageID,
						ProfileID: img.ProfileID,
						Width:     imageInfo.Width,
						Height:    imageInfo.Height,
						Extension: imageInfo.Extension,
						CreatedAt: imageInfo.CreatedAt,
						FileURL:   imageInfo.FileURL,
						Score:     img.Score},
					PostID:    img.PostID,
					Relevance: img.Relevance,
				}
			} else {
				imageResp = image.ImageResponseWithRelevance{
					Image: image.Image{
						ImageID:   img.ImageID,
						ProfileID: img.ProfileID,
						Score:     img.Score,
						CreatedAt: img.CreatedAt},
					PostID:    img.PostID,
					Relevance: img.Relevance,
				}
			}

			if tagIDs, ok := imagesTagsMap[img.ID]; ok {
				tags := make([]tag.Tag, 0, len(tagIDs))
				for _, tagID := range tagIDs {
					if t, found := imageTagsMap[tagID]; found {
						tags = append(tags, t)
					}
				}
				imageResp.Tags = tags
			}

			if voteValue, ok := imagesVotesMap[img.ID]; ok && voteValue != 0 {
				imageResp.UserVote = &voteValue
			}

			imagesResponse = append(imagesResponse, imageResp)
		}

		log.Info("images search completed",
			slog.Int("requested_tags", len(tagNames)),
			slog.Int("found_tags", len(tags)),
			slog.Int("cursor_relevance", cursorRelevance),
			slog.String("cursor_time", cursorTime.UTC().Format(time.RFC3339)),
			slog.Int("limit", limit),
			slog.Int("images_count", len(imagesResponse)),
			slog.Bool("has_next", hasNext))

		render.Status(r, http.StatusOK)
		render.JSON(w, r, Response{
			Response:      response.OK(),
			Images:        imagesResponse,
			NextCursor:    nextCursor,
			NextRelevance: nextRelevance,
			HasNext:       hasNext,
			Limit:         limit,
		})
	}
}

func parseQueryParams(r *http.Request, maxRelevance int) (int, time.Time, int, string, error) {
	const op = "handlers.search.images.parseQueryParams"

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
