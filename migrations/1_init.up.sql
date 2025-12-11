CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TABLE IF NOT EXISTS posts (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    profile_id UUID NOT NULL,
    text TEXT NOT NULL,
    score INTEGER NOT NULL DEFAULT 0,
    is_draft BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS posts_profile_id_idx ON posts (profile_id);
CREATE INDEX IF NOT EXISTS posts_score_idx ON posts (score);
CREATE INDEX IF NOT EXISTS posts_created_at_idx ON posts (created_at);


CREATE TABLE IF NOT EXISTS images (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    post_id UUID NOT NULL,
    image_id UUID NOT NULL,
    score INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT fk_images_posts FOREIGN KEY (post_id) REFERENCES posts(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS images_post_id_idx ON images (post_id);
CREATE INDEX IF NOT EXISTS images_image_id_idx ON images (image_id);
CREATE INDEX IF NOT EXISTS images_created_at_idx ON images (created_at);

CREATE TABLE IF NOT EXISTS tags (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    post_image_id UUID NOT NULL,
    tag_id UUID NOT NULL,

    CONSTRAINT fk_tags_post_images FOREIGN KEY (post_image_id) REFERENCES images(id) ON DELETE CASCADE,
    CONSTRAINT unique_tag_per_image UNIQUE (post_image_id, tag_id)
);

CREATE INDEX IF NOT EXISTS tags_post_image_id_idx ON tags (post_image_id);
CREATE INDEX IF NOT EXISTS tags_tag_id_idx ON tags (tag_id);

CREATE TABLE IF NOT EXISTS post_votes (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    post_id UUID NOT NULL,
    profile_id UUID NOT NULL,
    value INTEGER NOT NULL CHECK (value IN (-1, 0, 1)),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT fk_post_votes_posts FOREIGN KEY (post_id) REFERENCES posts(id) ON DELETE CASCADE,
    CONSTRAINT unique_post_vote UNIQUE(post_id, profile_id)
);

CREATE INDEX IF NOT EXISTS post_votes_post_id_idx ON post_votes (post_id);
CREATE INDEX IF NOT EXISTS post_votes_profile_id_idx ON post_votes (profile_id);
CREATE INDEX IF NOT EXISTS post_votes_value_idx ON post_votes (value);

CREATE TABLE IF NOT EXISTS image_votes (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    post_image_id UUID NOT NULL,
    profile_id UUID NOT NULL,
    value INTEGER NOT NULL CHECK (value IN (-1, 0, 1)),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT fk_tags_post_images FOREIGN KEY (post_image_id) REFERENCES images(id) ON DELETE CASCADE,
    CONSTRAINT unique_image_vote UNIQUE(post_image_id, profile_id)
);

CREATE INDEX IF NOT EXISTS image_votes_post_image_id_idx ON image_votes (post_image_id);
CREATE INDEX IF NOT EXISTS image_votes_profile_id_idx ON image_votes (profile_id);
CREATE INDEX IF NOT EXISTS image_votes_value_idx ON image_votes (value);