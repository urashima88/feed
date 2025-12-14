package posts_delete

import (
	"feed/internal/lib/api/response"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/render"
	"github.com/google/uuid"
)

type Response struct {
	response.Response
	Message string `json:"message"`
}

type PostDBDeleter interface {
	DeletePost(postID, profileID string) error
}

func New(log *slog.Logger, postDBDeleter PostDBDeleter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		const op = "handlers.posts.delete.New"

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
			log.Error("invalid post id format", slog.String("error", err.Error()))
			render.Status(r, http.StatusBadRequest)
			render.JSON(w, r, response.Error("invalid post id format"))
			return
		}

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

		err := postDBDeleter.DeletePost(postID, profileID)
		if err != nil {
			log.Error("failed to delete post", slog.String("post_id", postID))
			if err.Error() == "post not found or not owned by user" {
				render.Status(r, http.StatusNotFound)
				render.JSON(w, r, response.Error("post not found or this user doesn't have permission"))
				return
			}

			render.Status(r, http.StatusInternalServerError)
			render.JSON(w, r, response.Error("failed to delete post"))
			return
		}

		log.Info("post deleted successfully", slog.String("post_id", postID))

		render.Status(r, http.StatusOK)
		render.JSON(w, r, Response{
			Response: response.OK(),
			Message:  "post deleted successfully",
		})
	}
}
