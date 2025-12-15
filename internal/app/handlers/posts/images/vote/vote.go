package images_vote

import (
	"feed/internal/lib/api/response"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/render"
	"github.com/google/uuid"
)

type Request struct {
	Value int `json:"value"`
}

type Response struct {
	response.Response
	ImageID  string `json:"image_id"`
	NewScore int    `json:"new_score"`
	UserVote int    `json:"user_vote"`
}

type ImageDBVoter interface {
	VoteImage(postID, imageID, profileID string, value int) (newScore int, err error)
	GetImageInfo(postID, imageID string) (exists bool, score int, err error)
}

// @Summary Vote on an image within a post
// @Description Allows users to upvote (1), downvote (-1), or remove vote (0) on a specific image within a published post. Images in draft posts cannot be voted on.
// @Tags Posts
// @Accept json
// @Produce json
// @Param X-Profile-ID header string true "Profile ID of the voter" format(uuid)
// @Param post_id path string true "Post ID containing the image" format(uuid)
// @Param image_id path string true "Image ID to vote on" format(uuid)
// @Param request body Request true "Vote value"
// @Success 200 {object} Response "Vote recorded successfully with updated image scores"
// @Failure 400 {object} response.Response "Bad request - invalid vote value, invalid parameters, or missing headers"
// @Failure 403 {object} response.Response "Forbidden - attempting to vote on image in draft post"
// @Failure 404 {object} response.Response "Image not found"
// @Failure 500 {object} response.Response "Internal server error - database failure"
// @Security BearerAuth
// @Router /posts/{post_id}/images/{image_id}/vote [put]
func New(log *slog.Logger, imageDBVoter ImageDBVoter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		const op = "handlers.images.vote.New"

		log = log.With(
			slog.String("op", op),
			slog.String("request_id", middleware.GetReqID(r.Context())),
		)

		postID := chi.URLParam(r, "post_id")
		if postID == "" {
			log.Error("post id is required in URL")
			render.Status(r, http.StatusBadRequest)
			render.JSON(w, r, response.Error("post id is required in URL"))
			return
		}

		if _, err := uuid.Parse(postID); err != nil {
			log.Error("invalid post id format",
				slog.String("post_id", postID),
				slog.String("error", err.Error()))
			render.Status(r, http.StatusBadRequest)
			render.JSON(w, r, response.Error("invalid post id"))
			return
		}

		imageID := chi.URLParam(r, "image_id")
		if imageID == "" {
			log.Error("image id is required in URL")
			render.Status(r, http.StatusBadRequest)
			render.JSON(w, r, response.Error("image id is required in URL"))
			return
		}

		if _, err := uuid.Parse(imageID); err != nil {
			log.Error("invalid image id format",
				slog.String("image_id", imageID),
				slog.String("error", err.Error()))
			render.Status(r, http.StatusBadRequest)
			render.JSON(w, r, response.Error("invalid image id"))
			return
		}

		profileID := r.Header.Get("X-Profile-ID")
		if profileID == "" {
			log.Error("X-Profile-ID header is required")
			render.Status(r, http.StatusUnauthorized)
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

		if req.Value < -1 || req.Value > 1 {
			log.Error("invalid vote value",
				slog.Int("value", req.Value),
				slog.String("image_id", imageID))
			render.Status(r, http.StatusBadRequest)
			render.JSON(w, r, response.Error("invalid vote value"))
			return
		}

		exists, currentScore, err := imageDBVoter.GetImageInfo(postID, imageID)
		if err != nil {
			log.Error("failed to get image info",
				slog.String("image_id", imageID),
				slog.String("error", err.Error()))
			render.Status(r, http.StatusInternalServerError)
			render.JSON(w, r, response.Error("failed to check image"))
			return
		}

		if !exists {
			log.Error("image not found", slog.String("image_id", imageID))
			render.Status(r, http.StatusNotFound)
			render.JSON(w, r, response.Error("image not found"))
			return
		}

		newScore, err := imageDBVoter.VoteImage(postID, imageID, profileID, req.Value)
		if err != nil {
			if err.Error() == "cannot vote on image in draft post" {
				log.Error("cannot vote on image in draft post", slog.String("image_id", imageID))
				render.Status(r, http.StatusForbidden)
				render.JSON(w, r, response.Error("cannot vote on image in draft post"))
				return
			}

			log.Error("failed to vote image",
				slog.String("image_id", imageID),
				slog.Int("value", req.Value),
				slog.String("error", err.Error()))
			render.Status(r, http.StatusInternalServerError)
			render.JSON(w, r, response.Error("failed to vote image"))
			return
		}

		log.Info("image voted successfully",
			slog.String("image_id", imageID),
			slog.Int("value", req.Value),
			slog.Int("old_score", currentScore),
			slog.Int("new_score", newScore))

		render.Status(r, http.StatusOK)
		render.JSON(w, r, Response{
			Response: response.OK(),
			ImageID:  imageID,
			NewScore: newScore,
			UserVote: req.Value,
		})
	}
}
