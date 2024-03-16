-- init.sql

-- Create a user
-- CREATE USER goger WITH PASSWORD 'goger';
DO $$
BEGIN
CREATE USER goger WITH PASSWORD 'goger';
EXCEPTION WHEN duplicate_object THEN RAISE NOTICE '%, moving to next statement', SQLERRM USING ERRCODE = SQLSTATE;
END
$$;

-- Grant privileges
GRANT ALL PRIVILEGES ON DATABASE goger TO goger;

-- Keep all games from gog page
CREATE TABLE IF NOT EXISTS games (
    id INT PRIMARY KEY,
    slug VARCHAR(255) NOT NULL,
    features VARCHAR[] NOT NULL,
    screenshots VARCHAR[] NOT NULL,
    user_preferred_language_code VARCHAR,
    user_preferred_language_in_audio BOOLEAN,
    user_preferred_language_in_text BOOLEAN,
    release_date DATE NOT NULL,
    store_release_date DATE NOT NULL,
    product_type VARCHAR(50) NOT NULL,
    title VARCHAR(255) NOT NULL,
    cover_horizontal VARCHAR(255) NOT NULL,
    cover_vertical VARCHAR(255) NOT NULL,
    developers VARCHAR[] NOT NULL,
    publishers VARCHAR[] NOT NULL,
    operating_systems VARCHAR[] NOT NULL,
    price_final NUMERIC(10, 2) NOT NULL,
    price_base NUMERIC(10, 2) NOT NULL,
    price_currency VARCHAR(10) NOT NULL,
    price_discount NUMERIC(10, 2) NOT NULL,
    product_state VARCHAR(50) NOT NULL,
    genres VARCHAR[] NOT NULL,
    tags VARCHAR[] NOT NULL,
    reviews_count INTEGER DEFAULT 0,
    reviews_rating INTEGER NOT NULL,
    updated_at TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_price_final ON games(price_final);
CREATE INDEX IF NOT EXISTS idx_reviews_count ON games(reviews_count);
CLUSTER games USING idx_reviews_count;

-- Remember when last run was executed
CREATE TABLE IF NOT EXISTS last_run (
    -- just for usage with ON CONFLICT to overwite it.
    -- table should have only single record to track last run time
    onerow_id BOOL PRIMARY KEY DEFAULT true, 
    ts TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP(3) NOT NULL,
    CONSTRAINT onerow_uni CHECK (onerow_id)
);

-- Store reviews for all games
CREATE TABLE IF NOT EXISTS reviews (
    id VARCHAR(255) PRIMARY KEY,
    product_id INT,
    rating_value INT,
    title VARCHAR(255),
    description TEXT,
    language VARCHAR(255),
    reviewer_id VARCHAR(255),
    reviewer_username VARCHAR(255),
    avatar_gog_image_id VARCHAR(255),
    avatar_large VARCHAR(255),
    avatar_sdk_img_184 VARCHAR(255),
    avatar_menu_big VARCHAR(255),
    counters_games INT,
    counters_reviews INT,
    labels VARCHAR(255)[] DEFAULT '{}',
    downvotes INT,
    upvotes INT,
    review_date TIMESTAMPTZ,
    creation_date TIMESTAMPTZ,
    internal_update_date TIMESTAMPTZ,
    updated_at TIMESTAMP
);

CREATE INDEX idx_review_date ON reviews (review_date);
CREATE INDEX idx_downvotes ON reviews (downvotes);
CREATE INDEX idx_upvotes ON reviews (upvotes);

-- Remember execution time of each run with count of
-- fetched games and pages
CREATE TABLE IF NOT EXISTS run (
    id SERIAL PRIMARY KEY,
    games INT NOT NULL,
    pages INT NOT NULL,
    start_ts TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    end_ts   TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
);

-- Create a trigger function to maintain the fixed size of the run table.
-- Keep only 100 most recent runs
CREATE OR REPLACE FUNCTION maintain_queue_size()
RETURNS TRIGGER AS $$
BEGIN
    -- Remove the oldest elements if the queue size exceeds 100
    IF (SELECT COUNT(*) FROM run) > 100 THEN
        DELETE FROM run
        WHERE id IN (SELECT id FROM run ORDER BY start_ts LIMIT (SELECT COUNT(*) - 100 FROM run));
    END IF;

    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

-- Create an AFTER INSERT trigger to invoke the maintain_queue_size function
CREATE TRIGGER maintain_queue_size_trigger
AFTER INSERT ON run
FOR EACH STATEMENT EXECUTE FUNCTION maintain_queue_size();

CREATE OR REPLACE FUNCTION notify_games_change() RETURNS TRIGGER AS $$
BEGIN
  PERFORM pg_notify('games_changes', row_to_json(NEW)::text);
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER games_after_change
AFTER INSERT OR UPDATE OR DELETE ON games
FOR EACH ROW EXECUTE FUNCTION notify_games_change();