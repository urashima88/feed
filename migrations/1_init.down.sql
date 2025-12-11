DROP TABLE IF EXISTS posts;
DROP TABLE IF EXISTS images;
DROP TABLE IF EXISTS tags;
DROP TABLE IF EXISTS post_votes;
DROP TABLE IF EXISTS image_votes;

DROP INDEX IF EXISTS posts_profile_id_idx;
DROP INDEX IF EXISTS posts_created_at_idx;
DROP INDEX IF EXISTS posts_score_idx

DROP INDEX IF EXISTS images_post_id_idx;
DROP INDEX IF EXISTS images_image_id_idx;
DROP INDEX IF EXISTS images_created_at_idx;

DROP INDEX IF EXISTS tags_image_post_id_idx;
DROP INDEX IF EXISTS tags_tag_id_idx;

DROP INDEX IF EXISTS post_votes_post_id_idx;
DROP INDEX IF EXISTS post_votes_profile_id_idx;
DROP INDEX IF EXISTS post_votes_value_idx;

DROP INDEX IF EXISTS image_votes_post_image_id_idx;
DROP INDEX IF EXISTS image_votes_profile_id_idx;
DROP INDEX IF EXISTS image_votes_value_idx;