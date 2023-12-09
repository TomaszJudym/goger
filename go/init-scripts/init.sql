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
    id SERIAL PRIMARY KEY,
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
    reviews_rating INTEGER NOT NULL,
    CONSTRAINT unique_id UNIQUE (id)
);

CREATE INDEX IF NOT EXISTS idx_slug ON games(slug);
CREATE INDEX IF NOT EXISTS idx_price_final ON games(price_final);

-- Remember when last run was executed
CREATE TABLE IF NOT EXISTS last_run (
    -- just for usage with ON CONFLICT to overwite it.
    -- table should have only single record to track last run time
    onerow_id BOOL PRIMARY KEY DEFAULT true, 
    ts TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP(3) NOT NULL,
    CONSTRAINT onerow_uni CHECK (onerow_id)
);
