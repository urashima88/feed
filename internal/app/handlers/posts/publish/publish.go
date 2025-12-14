package posts_publish

import (
	"feed/internal/lib/api/response"
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
	Message string `json:"message"`
}

type PostDBPublisher interface {
	PublishPost(postID, profileID string) error
}

func New(log *slog.Logger, postDBPublisher PostDBPublisher) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		const op = "handlers.posts.publish.New"

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

		profileID := r.Header.Get("X-Profile-ID")
		if profileID == "" {
			log.Error("X-Profile-ID header is required")
			render.Status(r, http.StatusBadRequest)
			render.JSON(w, r, response.Error("X-Profile-ID header is required"))
			return
		}

		if _, err := uuid.Parse(profileID); err != nil {
			log.Error("invalid profile id format", sl.Err(err))
			render.Status(r, http.StatusBadRequest)
			render.JSON(w, r, response.Error("invalid profile id format"))
			return
		}

		err := postDBPublisher.PublishPost(postID, profileID)
		if err != nil {
			log.Error("failed to publish post",
				slog.String("post_id", postID),
				slog.String("error", err.Error()))

			switch {
			case err.Error() == "post not found":
				render.Status(r, http.StatusNotFound)
				render.JSON(w, r, response.Error("post not found"))
			case err.Error() == "user is not the owner of the post":
				render.Status(r, http.StatusForbidden)
				render.JSON(w, r, response.Error("this user doesn't have permission to publish this post"))
			case err.Error() == "post is already published":
				render.Status(r, http.StatusBadRequest)
				render.JSON(w, r, response.Error("post is already published"))
			default:
				render.Status(r, http.StatusInternalServerError)
				render.JSON(w, r, response.Error("failed to publish post"))
			}
			return
		}

		log.Info("post published successfully", slog.String("post_id", postID))

		render.Status(r, http.StatusOK)
		render.JSON(w, r, Response{
			Response: response.OK(),
			Message:  "post published successfully",
		})
	}
}
