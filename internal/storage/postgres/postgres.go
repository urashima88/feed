package postgres

import (
	"database/sql"
	app_config "feed/internal/config/app-config"
	"feed/internal/lib/api/image"
	"feed/internal/lib/api/post"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	_ "github.com/lib/pq"
)

type Storage struct {
	db *sql.DB
}

func New(cfg *app_config.Config) (*Storage, error) {
	const op = "storage.postgres.New"

	connStr := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		cfg.Db.Host,
		cfg.Db.Port,
		cfg.Db.User,
		cfg.Db.Password,
		cfg.Db.Name,
		cfg.SSLMode)

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	return &Storage{db: db}, nil
}

func (s *Storage) CreatePost(profileID, text string, isDraft bool, images []image.ImageWithTags) (*post.UserPost, error) {
	const op = "storage.postgres.CreatePost"

	tx, err := s.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("%s: failed to begin transaction: %w", op, err)
	}
	defer tx.Rollback()

	postQuery := `
		INSERT INTO posts (profile_id, text, is_draft)
		VALUES ($1, $2, $3)
		RETURNING id, profile_id, text, score, is_draft, created_at, updated_at
	`

	var userPost post.UserPost
	err = tx.QueryRow(postQuery, profileID, text, isDraft).Scan(
		&userPost.ID,
		&userPost.ProfileID,
		&userPost.Text,
		&userPost.Score,
		&userPost.IsDraft,
		&userPost.CreatedAt,
		&userPost.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("%s: failed to create post: %w", op, err)
	}

	for _, img := range images {
		imageQuery := `
			INSERT INTO images (post_id, image_id)
			VALUES ($1, $2)
			RETURNING id
		`

		var imageUUID string
		err := tx.QueryRow(imageQuery, userPost.ID, img.ImageID).Scan(&imageUUID)
		if err != nil {
			return nil, fmt.Errorf("%s: failed to insert image record for image_id %s: %w", op, img.ImageID, err)
		}

		if len(img.Tags) > 0 {
			for _, t := range img.Tags {
				if _, err := uuid.Parse(t.ID); err != nil {
					slog.Warn("invalid tag id, skipping...",
						slog.String("op", op),
						slog.String("tag_id", t.ID),
						slog.String("post_id", userPost.ID),
						slog.String("image_id", img.ImageID))
					continue
				}

				tagQuery := `
					INSERT INTO tags (post_image_id, tag_id)
					VALUES ($1, $2)
					ON CONFLICT (post_image_id, tag_id) DO NOTHING
				`

				_, err := tx.Exec(tagQuery, imageUUID, t.ID)
				if err != nil {
					slog.Warn("failed to insert tag",
						slog.String("op", op),
						slog.String("error", err.Error()),
						slog.String("post_id", userPost.ID),
						slog.String("image_id", img.ImageID),
						slog.String("tag_id", t.ID))
				}
			}
		}
	}

	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("%s: failed to commit transaction: %w", op, err)
	}

	return &userPost, nil
}

func (s *Storage) GetUserPublicPosts(profileID string, cursorTime time.Time, limit int) ([]post.UserPost, error) {
	const op = "storage.postgres.GetUserPublicPosts"

	query := `
		SELECT id, profile_id, text, score, is_draft, created_at, updated_at
		FROM posts
		WHERE profile_id = $1
		AND created_at < $2
		AND is_draft = false
		ORDER BY created_at DESC
		LIMIT $3
	`

	rows, err := s.db.Query(query, profileID, cursorTime, limit)
	if err != nil {
		return nil, fmt.Errorf("%s: failed to select user public posts: %w", op, err)
	}
	defer rows.Close()

	var posts []post.UserPost
	for rows.Next() {
		var post post.UserPost
		err := rows.Scan(
			&post.ID,
			&post.ProfileID,
			&post.Text,
			&post.Score,
			&post.IsDraft,
			&post.CreatedAt,
			&post.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("%s: error scan post: %w", op, err)
		}
		posts = append(posts, post)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("%s: error iterating rows: %w", op, err)
	}

	return posts, nil
}

func (s *Storage) GetUserPosts(profileID string, cursorTime time.Time, limit int) ([]post.UserPost, error) {
	const op = "storage.postgres.GetUserPosts"

	query := `
		SELECT id, profile_id, text, score, is_draft, created_at, updated_at
		FROM posts
		WHERE profile_id = $1
		AND created_at < $2
		ORDER BY created_at DESC
		LIMIT $3
	`

	rows, err := s.db.Query(query, profileID, cursorTime, limit)
	if err != nil {
		return nil, fmt.Errorf("%s: failed to select user posts: %w", op, err)
	}
	defer rows.Close()

	var posts []post.UserPost
	for rows.Next() {
		var post post.UserPost
		err := rows.Scan(
			&post.ID,
			&post.ProfileID,
			&post.Text,
			&post.Score,
			&post.IsDraft,
			&post.CreatedAt,
			&post.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("%s: error scan post: %w", op, err)
		}
		posts = append(posts, post)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("%s: error iterating rows: %w", op, err)
	}

	return posts, nil
}

func (s *Storage) GetPostsImages(postIDs []string) ([]string, []string, map[string]map[string]string, map[string][]image.Image, error) {
	const op = "storage.postgres.GetPostsImages"

	if len(postIDs) == 0 {
		return []string{}, []string{}, make(map[string]map[string]string), make(map[string][]image.Image), nil
	}

	query := `
		SELECT id, post_id, image_id, score
		FROM images
		WHERE post_id = ANY($1)
		ORDER BY created_at ASC
	`
	rows, err := s.db.Query(query, pq.Array(postIDs))
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("%s: failed to select posts images: %w", op, err)
	}
	defer rows.Close()

	postImageIDs := []string{}
	allImageIDs := []string{}
	postsPostImageIDsMap := make(map[string]map[string]string)
	for _, postID := range postIDs {
		postsPostImageIDsMap[postID] = make(map[string]string)
	}
	postsImagesMap := make(map[string][]image.Image)
	for rows.Next() {
		var id string
		var postID string
		var img image.Image

		err := rows.Scan(&id, &postID, &img.ImageID, &img.Score)
		if err != nil {
			slog.Error("failed to scan image row",
				slog.String("op", op),
				slog.String("error", err.Error()))
			continue
		}

		postImageIDs = append(postImageIDs, id)
		allImageIDs = append(allImageIDs, img.ImageID)
		postsPostImageIDsMap[postID][img.ImageID] = id
		postsImagesMap[postID] = append(postsImagesMap[postID], img)
	}

	if err = rows.Err(); err != nil {
		return nil, nil, nil, nil, fmt.Errorf("%s: error iterating rows: %w", op, err)
	}

	return postImageIDs, allImageIDs, postsPostImageIDsMap, postsImagesMap, nil
}

func (s *Storage) GetImagesTags(postImageIDs []string) (map[string][]string, error) {
	const op = "storage.postgres.GetImagesTags"

	if len(postImageIDs) == 0 {
		return make(map[string][]string), nil
	}

	query := `
		SELECT i.id, t.tag_id
		FROM images i
		JOIN tags t ON i.id = t.post_image_id
		WHERE i.id = ANY($1)
		ORDER BY i.id, t.tag_id
	`

	rows, err := s.db.Query(query, pq.Array(postImageIDs))
	if err != nil {
		return nil, fmt.Errorf("%s: failed to select images tags: %w", op, err)
	}
	defer rows.Close()

	res := make(map[string][]string)
	for rows.Next() {
		var id, tagID string

		err := rows.Scan(&id, &tagID)
		if err != nil {
			slog.Error("failed to scan tag row",
				slog.String("op", op),
				slog.String("error", err.Error()))
			continue
		}

		res[id] = append(res[id], tagID)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("%s: error iterating rows: %w", op, err)
	}

	return res, nil
}

func (s *Storage) GetPostsVotes(postIDs []string, viewerProfileID string) (map[string]int, error) {
	const op = "storage.postgres.GetPostsVotes"

	if len(postIDs) == 0 {
		return make(map[string]int), nil
	}

	query := `
		SELECT post_id, value
		FROM post_votes
		WHERE post_id = ANY($1) AND profile_id = $2
	`

	rows, err := s.db.Query(query, pq.Array(postIDs), viewerProfileID)
	if err != nil {
		return nil, fmt.Errorf("%s: failed to select post votes: %w", op, err)
	}
	defer rows.Close()

	res := make(map[string]int)
	for rows.Next() {
		var postID string
		var value int
		if err := rows.Scan(&postID, &value); err != nil {
			slog.Error("failed to scan post vote",
				slog.String("op", op),
				slog.String("error", err.Error()))
			continue
		}
		res[postID] = value
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("%s: error iterating rows: %w", op, err)
	}

	return res, nil
}

func (s *Storage) GetImagesVotes(postImageIDs []string, viewerProfileID string) (map[string]int, error) {
	const op = "storage.postgres.GetImagesVotes"

	if len(postImageIDs) == 0 {
		return make(map[string]int), nil
	}

	query := `
		SELECT iv.post_image_id, COALESCE(iv.value, 0) as value
		FROM images i
		LEFT JOIN image_votes iv ON i.id = iv.post_image_id
		WHERE iv.post_image_id = ANY($2) AND iv.profile_id = $1
	`

	rows, err := s.db.Query(query, viewerProfileID, pq.Array(postImageIDs))
	if err != nil {
		return nil, fmt.Errorf("%s: failed to select image votes: %w", op, err)
	}
	defer rows.Close()

	res := make(map[string]int)
	for rows.Next() {
		var id string
		var value int
		if err := rows.Scan(&id, &value); err != nil {
			slog.Error("failed to scan image vote",
				slog.String("op", op),
				slog.String("error", err.Error()))
			continue
		}
		res[id] = value
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("%s: error iterating rows: %w", op, err)
	}

	return res, nil
}

func (s *Storage) GetPostInfo(postID string) (exists bool, isDraft bool, score int, err error) {
	const op = "storage.postgres.GetPostInfo"

	query := `
		SELECT is_draft, score
		FROM posts 
		WHERE id = $1
	`

	var draft sql.NullBool
	var scr sql.NullInt64

	err = s.db.QueryRow(query, postID).Scan(&draft, &scr)
	if err != nil {
		if err == sql.ErrNoRows {
			return false, false, 0, nil
		}
		return false, false, 0, fmt.Errorf("%s: failed to get post info: %w", op, err)
	}

	exists = true
	isDraft = draft.Valid && draft.Bool
	score = int(scr.Int64)

	return exists, isDraft, score, nil
}

func (s *Storage) VotePost(postID, profileID string, value int) (int, error) {
	const op = "storage.postgres.VotePost"

	if value < -1 || value > 1 {
		return 0, fmt.Errorf("%s: invalid vote value: %d", op, value)
	}

	tx, err := s.db.Begin()
	if err != nil {
		return 0, fmt.Errorf("%s: failed to begin transaction: %w", op, err)
	}
	defer tx.Rollback()

	var isDraft bool
	checkQuery := `
		SELECT is_draft
		FROM posts 
		WHERE id = $1
		FOR UPDATE
	`

	err = tx.QueryRow(checkQuery, postID).Scan(&isDraft)
	if err != nil {
		if err == sql.ErrNoRows {
			return 0, fmt.Errorf("%s: post not found: %s", op, postID)
		}
		return 0, fmt.Errorf("%s: failed to check post: %w", op, err)
	}

	if isDraft {
		return 0, fmt.Errorf("%s: cannot vote on draft post", op)
	}

	if value == 0 {
		deleteQuery := `DELETE FROM post_votes WHERE post_id = $1 AND profile_id = $2`

		res, err := tx.Exec(deleteQuery, postID, profileID)
		if err != nil && err != sql.ErrNoRows {
			return 0, fmt.Errorf("%s: failed to delete vote: %w", op, err)
		}

		rowsAffected, _ := res.RowsAffected()
		if rowsAffected > 0 {
			slog.Debug("vote removed",
				slog.String("op", op),
				slog.String("post_id", postID))
		}
	} else {
		upsertQuery := `
			INSERT INTO post_votes (post_id, profile_id, value)
			VALUES ($1, $2, $3)
			ON CONFLICT (post_id, profile_id)
			DO UPDATE SET value = EXCLUDED.value, updated_at = NOW()
			RETURNING (xmax = 0) AS inserted
		`

		var inserted bool
		err = tx.QueryRow(upsertQuery, postID, profileID, value).Scan(&inserted)
		if err != nil {
			return 0, fmt.Errorf("%s: failed to vote post: %w", op, err)
		}
	}

	var newScore int
	updateQuery := `
		UPDATE posts 
		SET score = COALESCE((
			SELECT SUM(value)
			FROM post_votes 
			WHERE post_id = $1
		), 0),
		updated_at = NOW()
		WHERE id = $1
		RETURNING score
	`

	err = tx.QueryRow(updateQuery, postID).Scan(&newScore)
	if err != nil {
		return 0, fmt.Errorf("%s: failed to update post score: %w", op, err)
	}

	if err = tx.Commit(); err != nil {
		return 0, fmt.Errorf("%s: failed to commit transaction: %w", op, err)
	}

	return newScore, nil
}

func (s *Storage) GetImageInfo(postID, imageID string) (exists bool, score int, err error) {
	const op = "storage.postgres.GetImageInfo"

	query := `
		SELECT score
		FROM images 
		WHERE post_id = $1 AND image_id = $2 
	`

	var scr sql.NullInt64

	err = s.db.QueryRow(query, postID, imageID).Scan(&scr)
	if err != nil {
		if err == sql.ErrNoRows {
			return false, 0, nil
		}
		return false, 0, fmt.Errorf("%s: failed to get image info: %w", op, err)
	}

	exists = true
	score = int(scr.Int64)

	return exists, score, nil
}

func (s *Storage) VoteImage(postID, imageID, profileID string, value int) (int, error) {
	const op = "storage.postgres.VoteImage"

	if value < -1 || value > 1 {
		return 0, fmt.Errorf("%s: invalid vote value: %d", op, value)
	}

	tx, err := s.db.Begin()
	if err != nil {
		return 0, fmt.Errorf("%s: failed to begin transaction: %w", op, err)
	}
	defer tx.Rollback()

	var postImageID string
	var isPostDraft bool

	findQuery := `
		SELECT 
			i.id,
			p.is_draft
		FROM images i
		JOIN posts p ON i.post_id = p.id
		WHERE i.post_id = $1 AND i.image_id = $2
		FOR UPDATE OF i, p
	`

	err = tx.QueryRow(findQuery, postID, imageID).Scan(&postImageID, &isPostDraft)
	if err != nil {
		if err == sql.ErrNoRows {
			return 0, fmt.Errorf("%s: image not found: %s", op, imageID)
		}
		return 0, fmt.Errorf("%s: failed to find image: %w", op, err)
	}

	if isPostDraft {
		return 0, fmt.Errorf("cannot vote on image in draft post")
	}

	if value == 0 {
		deleteQuery := `DELETE FROM image_votes WHERE post_image_id = $1 AND profile_id = $2`
		res, err := tx.Exec(deleteQuery, postImageID, profileID)
		if err != nil && err != sql.ErrNoRows {
			return 0, fmt.Errorf("%s: failed to delete image vote: %w", op, err)
		}

		rowsAffected, _ := res.RowsAffected()
		if rowsAffected > 0 {
			slog.Debug("image vote removed",
				slog.String("op", op),
				slog.String("image_id", imageID),
				slog.String("post_image_id", postImageID))
		}
	} else {
		upsertQuery := `
			INSERT INTO image_votes (post_image_id, profile_id, value)
			VALUES ($1, $2, $3)
			ON CONFLICT (post_image_id, profile_id) 
			DO UPDATE SET value = EXCLUDED.value, updated_at = NOW()
			RETURNING (xmax = 0) as inserted
		`

		var inserted bool
		err = tx.QueryRow(upsertQuery, postImageID, profileID, value).Scan(&inserted)
		if err != nil {
			return 0, fmt.Errorf("%s: failed to vote image: %w", op, err)
		}
	}

	var newScore int
	updateQuery := `
		UPDATE images 
		SET score = COALESCE((
			SELECT SUM(value)
			FROM image_votes 
			WHERE post_image_id = $1
		), 0)
		WHERE id = $1
		RETURNING score
	`

	err = tx.QueryRow(updateQuery, postImageID).Scan(&newScore)
	if err != nil {
		return 0, fmt.Errorf("%s: failed to update image score: %w", op, err)
	}

	if err = tx.Commit(); err != nil {
		return 0, fmt.Errorf("%s: failed to commit transaction: %w", op, err)
	}

	return newScore, nil
}

func (s *Storage) GetFeedPosts(cursorTime time.Time, limit int) ([]post.UserPost, error) {
	const op = "storage.postgres.GetFeedPosts"

	query := `
		SELECT id, profile_id, text, score, is_draft, created_at, updated_at
		FROM posts
		WHERE is_draft = false
		AND created_at < $1
		ORDER BY created_at DESC
		LIMIT $2
	`

	rows, err := s.db.Query(query, cursorTime, limit)
	if err != nil {
		return nil, fmt.Errorf("%s: failed to select feed posts: %w", op, err)
	}
	defer rows.Close()

	var posts []post.UserPost
	for rows.Next() {
		var post post.UserPost
		err := rows.Scan(
			&post.ID,
			&post.ProfileID,
			&post.Text,
			&post.Score,
			&post.IsDraft,
			&post.CreatedAt,
			&post.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("%s: error scan post: %w", op, err)
		}
		posts = append(posts, post)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("%s: error iterating rows: %w", op, err)
	}

	return posts, nil
}

func (s *Storage) GetFeedPostsByScore(cursorScore int, cursorTime time.Time, limit int) ([]post.UserPost, error) {
	const op = "storage.postgres.GetFeedPostsByScore"

	query := `
		SELECT id, profile_id, text, score, is_draft, created_at, updated_at
		FROM posts
		WHERE is_draft = false
		AND (score < $1 OR (score = $1 AND created_at < $2))
		ORDER BY score DESC, created_at DESC
		LIMIT $3
	`

	rows, err := s.db.Query(query, cursorScore, cursorTime, limit)
	if err != nil {
		return nil, fmt.Errorf("%s: failed to select feed posts by score: %w", op, err)
	}
	defer rows.Close()

	var posts []post.UserPost
	for rows.Next() {
		var post post.UserPost
		err := rows.Scan(
			&post.ID,
			&post.ProfileID,
			&post.Text,
			&post.Score,
			&post.IsDraft,
			&post.CreatedAt,
			&post.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("%s: error scan post: %w", op, err)
		}
		posts = append(posts, post)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("%s: error iterating rows: %w", op, err)
	}

	return posts, nil
}

func (s *Storage) SearchPostsByTagsRelevance(tagIDs []string, cursorRelevance int, cursorTime time.Time, limit int) ([]post.UserPostWithRelevance, error) {
	const op = "storage.postgres.SearchPostsByTagsRelevance"

	query := `
		WITH post_tags AS (
			SELECT 
				p.id,
				p.profile_id,
				p.text,
				p.score,
				p.is_draft,
				p.created_at,
				p.updated_at,
				COUNT(DISTINCT t.tag_id) as relevance
			FROM posts p
			JOIN images i ON p.id = i.post_id
			JOIN tags t ON i.id = t.post_image_id
			WHERE p.is_draft = false
				AND t.tag_id = ANY($1)
			GROUP BY p.id
			HAVING COUNT(DISTINCT t.tag_id) > 0
		)
		SELECT id, profile_id, text, score, is_draft, created_at, updated_at, relevance
		FROM post_tags
		WHERE relevance <= $2 AND created_at < $3
		ORDER BY relevance DESC, created_at DESC
		LIMIT $4
	`

	rows, err := s.db.Query(query, pq.Array(tagIDs), cursorRelevance, cursorTime, limit)
	if err != nil {
		return nil, fmt.Errorf("%s: failed to search posts by tags with relevance: %w", op, err)
	}
	defer rows.Close()

	var posts []post.UserPostWithRelevance
	for rows.Next() {
		var post post.UserPostWithRelevance
		err := rows.Scan(
			&post.ID,
			&post.ProfileID,
			&post.Text,
			&post.Score,
			&post.IsDraft,
			&post.CreatedAt,
			&post.UpdatedAt,
			&post.Relevance,
		)
		if err != nil {
			return nil, fmt.Errorf("%s: error scan post: %w", op, err)
		}
		posts = append(posts, post)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("%s: error iterating rows: %w", op, err)
	}

	return posts, nil
}

func (s *Storage) SearchImagesByTagsRelevance(tagIDs []string, cursorRelevance int, cursorTime time.Time, limit int) ([]image.ImageWithRelevance, error) {
	const op = "storage.postgres.SearchImagesByTagsRelevance"

	query := `
        WITH image_tags AS (
            SELECT 
                i.id,
                i.image_id,
                i.score,
                i.created_at,
                p.profile_id,
				i.post_id,
                COUNT(DISTINCT t.tag_id) as relevance
            FROM images i
            JOIN posts p ON i.post_id = p.id
            JOIN tags t ON i.id = t.post_image_id
            WHERE p.is_draft = false
                AND t.tag_id = ANY($1)
            GROUP BY i.id, p.profile_id
            HAVING COUNT(DISTINCT t.tag_id) > 0
        )
        SELECT id, image_id, score, created_at, profile_id, post_id, relevance
        FROM image_tags
        WHERE relevance <= $2 AND created_at < $3
        ORDER BY relevance DESC, created_at DESC
        LIMIT $4
    `

	rows, err := s.db.Query(query, pq.Array(tagIDs), cursorRelevance, cursorTime, limit)
	if err != nil {
		return nil, fmt.Errorf("%s: failed to search images by tags with relevance: %w", op, err)
	}
	defer rows.Close()

	var images []image.ImageWithRelevance
	for rows.Next() {
		var img image.ImageWithRelevance
		err := rows.Scan(
			&img.ID,
			&img.ImageID,
			&img.Score,
			&img.CreatedAt,
			&img.ProfileID,
			&img.PostID,
			&img.Relevance,
		)
		if err != nil {
			return nil, fmt.Errorf("%s: error scan image: %w", op, err)
		}
		images = append(images, img)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("%s: error iterating rows: %w", op, err)
	}

	return images, nil
}

func (s *Storage) DeletePost(postID, profileID string) error {
	const op = "storage.postgres.DeletePost"

	var exists bool
	checkQuery := `
        SELECT EXISTS(
            SELECT 1 FROM posts 
            WHERE id = $1 AND profile_id = $2
        )
    `

	err := s.db.QueryRow(checkQuery, postID, profileID).Scan(&exists)
	if err != nil {
		return fmt.Errorf("%s: failed to check post ownership: %w", op, err)
	}

	if !exists {
		slog.Warn("post not found or not owned by user", slog.String("op", op))
		return fmt.Errorf("post not found or not owned by user")
	}

	deleteQuery := `
        DELETE FROM posts 
        WHERE id = $1 AND profile_id = $2
    `

	res, err := s.db.Exec(deleteQuery, postID, profileID)
	if err != nil {
		return fmt.Errorf("%s: failed to delete post: %w", op, err)
	}

	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("%s: failed to get rows affected: %w", op, err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("%s: post not found or not owned by user", op)
	}

	return nil
}

func (s *Storage) GetPostByID(postID string) (*post.UserPost, error) {
	const op = "storage.postgres.GetPostByID"

	query := `
        SELECT id, profile_id, text, score, is_draft, created_at, updated_at
        FROM posts
        WHERE id = $1
    `

	var userPost post.UserPost
	err := s.db.QueryRow(query, postID).Scan(
		&userPost.ID,
		&userPost.ProfileID,
		&userPost.Text,
		&userPost.Score,
		&userPost.IsDraft,
		&userPost.CreatedAt,
		&userPost.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("%s: failed to get post by id: %w", op, err)
	}

	return &userPost, nil
}

func (s *Storage) GetPostImages(postID string) ([]string, []string, map[string]string, []image.Image, error) {
	const op = "storage.postgres.GetPostImages"

	query := `
        SELECT id, post_id, image_id, score
        FROM images
        WHERE post_id = $1
        ORDER BY created_at ASC
    `

	rows, err := s.db.Query(query, postID)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("%s: failed to select post images: %w", op, err)
	}
	defer rows.Close()

	postImageIDs := []string{}
	allImageIDs := []string{}
	imageIDsPostImageIDsMap := make(map[string]string)
	postImages := []image.Image{}

	for rows.Next() {
		var id string
		var postID string
		var img image.Image

		err := rows.Scan(&id, &postID, &img.ImageID, &img.Score)
		if err != nil {
			slog.Error("failed to scan image row",
				slog.String("op", op),
				slog.String("error", err.Error()))
			continue
		}

		postImageIDs = append(postImageIDs, id)
		allImageIDs = append(allImageIDs, img.ImageID)
		imageIDsPostImageIDsMap[img.ImageID] = id
		postImages = append(postImages, img)
	}

	if err = rows.Err(); err != nil {
		return nil, nil, nil, nil, fmt.Errorf("%s: error iterating rows: %w", op, err)
	}

	return postImageIDs, allImageIDs, imageIDsPostImageIDsMap, postImages, nil
}

func (s *Storage) GetPostVote(postID string, viewerProfileID string) (int, error) {
	const op = "storage.postgres.GetPostsVotes"

	query := `
		SELECT post_id, value
		FROM post_votes
		WHERE post_id = $1 AND profile_id = $2
	`

	var value int
	err := s.db.QueryRow(query, postID, viewerProfileID).Scan(&value)
	if err != nil {
		if err == sql.ErrNoRows {
			return 0, nil
		}
		return 0, fmt.Errorf("%s: failed to get post vote: %w", op, err)
	}

	return value, nil
}

func (s *Storage) PublishPost(postID, profileID string) error {
	const op = "storage.postgres.PublishPost"

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("%s: failed to begin transaction: %w", op, err)
	}
	defer tx.Rollback()

	var isDraft bool
	var ownerID string
	checkQuery := `
        SELECT profile_id, is_draft
        FROM posts 
        WHERE id = $1
        FOR UPDATE
    `

	err = tx.QueryRow(checkQuery, postID).Scan(&ownerID, &isDraft)
	if err != nil {
		if err == sql.ErrNoRows {
			slog.Warn("post not found",
				slog.String("op", op),
				slog.String("error", err.Error()))
			return fmt.Errorf("post not found")
		}
		return fmt.Errorf("%s: failed to check post: %w", op, err)
	}

	if ownerID != profileID {
		slog.Warn("user is not the owner of the post", slog.String("op", op))
		return fmt.Errorf("user is not the owner of the post")
	}

	if !isDraft {
		slog.Warn("post is already published", slog.String("op", op))
		return fmt.Errorf("post is already published")
	}

	updateQuery := `
        UPDATE posts 
        SET is_draft = false, updated_at = NOW()
        WHERE id = $1 AND profile_id = $2
        RETURNING id
    `

	var updatedID string
	err = tx.QueryRow(updateQuery, postID, profileID).Scan(&updatedID)
	if err != nil {
		return fmt.Errorf("%s: failed to publish post: %w", op, err)
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("%s: failed to commit transaction: %w", op, err)
	}

	return nil
}
