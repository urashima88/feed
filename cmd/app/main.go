// @title Feed Microservice API
// @version 1.0
// @description Microservice for managing posts, post images, voting, feed, and search functionality
//
// @host localhost:8095
// @BasePath /api/v1
//
// @tag.name Posts
// @tag.description "Post operations: create post, get single post, vote post/image, delete, etc"
//
// @tag.name Feed
// @tag.description "Feed operations: get content for the user"
//
// @tag.name Search
// @tag.description "Search operations: tag search for posts/images"
package main

import (
	"context"
	feed_posts "feed/internal/app/handlers/feed/posts"
	posts_create "feed/internal/app/handlers/posts/create"
	posts_delete "feed/internal/app/handlers/posts/delete"
	posts_get "feed/internal/app/handlers/posts/get"
	images_vote "feed/internal/app/handlers/posts/images/vote"
	posts_publish "feed/internal/app/handlers/posts/publish"
	posts_user_all "feed/internal/app/handlers/posts/user/all"
	posts_vote "feed/internal/app/handlers/posts/vote"
	search_images "feed/internal/app/handlers/search/images"
	search_posts "feed/internal/app/handlers/search/posts"
	"feed/internal/app/middleware/logger"
	app_config "feed/internal/config/app-config"
	"feed/internal/lib/logger/sl"
	image_service "feed/internal/services/image"
	tag_service "feed/internal/services/tag"
	uuid_service "feed/internal/services/uuid-service"
	"feed/internal/storage/postgres"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "feed/docs"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	httpSwagger "github.com/swaggo/http-swagger"
)

const (
	envLocal = "local"
	envDev   = "dev"
	envProd  = "prod"
)

func main() {
	cfg := app_config.MustLoad()
	log := setupLogger(cfg.Env)
	log = log.With(slog.String("env", cfg.Env))

	log.Info("starting feed-app")
	log.Debug("logger debug mode enabled")

	storage, err := postgres.New(cfg)
	if err != nil {
		log.Error("failed to initialize storage", sl.Err(err))
		os.Exit(1)
	}

	imageService := image_service.New(
		log,
		fmt.Sprintf("http://%s:%s", cfg.ImageService.Host, cfg.ImageService.Port),
		cfg.ImageService.GetImagesURL,
		cfg.ImageService.Timeout,
	)

	tagService := tag_service.New(
		log,
		fmt.Sprintf("http://%s:%s", cfg.TagService.Host, cfg.TagService.Port),
		cfg.TagService.CreateTagsURL,
		cfg.GetTagsURL,
		cfg.TagService.Timeout,
	)

	uuidService := uuid_service.New()

	router := chi.NewRouter()

	router.Use(middleware.RequestID)
	router.Use(middleware.Logger)
	router.Use(logger.New(log))
	router.Use(middleware.Recoverer)
	router.Use(middleware.URLFormat)

	router.Route("/api", func(r chi.Router) {
		r.Route("/v1", func(r chi.Router) {
			r.Post("/posts", posts_create.New(log, storage, imageService, tagService, uuidService, &cfg.PostMeta))
			r.Get("/posts/user/all", posts_user_all.New(log, storage, imageService, tagService, uuidService))
			r.Get("/posts/{post_id}", posts_get.New(log, storage, imageService, tagService, uuidService))
			r.Put("/posts/{post_id}/publish", posts_publish.New(log, storage))
			r.Put("/posts/{post_id}/vote", posts_vote.New(log, storage))
			r.Put("/posts/{post_id}/images/{image_id}/vote", images_vote.New(log, storage))
			r.Delete("/posts/{post_id}", posts_delete.New(log, storage))

			r.Get("/feed/posts", feed_posts.New(log, storage, imageService, tagService, uuidService))

			r.Get("/search/posts", search_posts.New(log, storage, imageService, tagService, uuidService))
			r.Get("/search/images", search_images.New(log, storage, imageService, tagService, uuidService))
		})
	})

	router.Get("/swagger/*", httpSwagger.Handler(
		httpSwagger.URL("/swagger/doc.json"),
	))

	log.Info("starting server", slog.String("address", cfg.HTTPServer.Host+":"+cfg.HTTPServer.Port))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	srv := &http.Server{
		Addr:         cfg.HTTPServer.Host + ":" + cfg.HTTPServer.Port,
		Handler:      router,
		ReadTimeout:  cfg.HTTPServer.Timeout,
		WriteTimeout: cfg.HTTPServer.Timeout,
		IdleTimeout:  cfg.HTTPServer.IdleTimeout,
	}

	serverErrors := make(chan error, 1)

	go func() {
		log.Info("server started")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverErrors <- err
		}
	}()

	select {
	case <-ctx.Done():
		log.Info("shutdown signal received")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)

		defer cancel()

		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Error("failed to shutdown server gracefully", sl.Err(err))
			if err := srv.Close(); err != nil {
				log.Error("failed to close server", sl.Err(err))
			}
		}
		log.Info("server stopped gracefully")
	case err := <-serverErrors:
		log.Error("server failed to start", sl.Err(err))
		os.Exit(1)
	}
}

func setupLogger(env string) *slog.Logger {
	var log *slog.Logger

	switch env {
	case envLocal:
		log = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	case envDev:
		log = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	case envProd:
		log = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	}

	return log
}
