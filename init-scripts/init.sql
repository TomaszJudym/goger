-- init.sql

-- Create a user
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

CREATE TABLE IF NOT EXISTS language_codes_to_names (
    code VARCHAR(10) PRIMARY KEY,
    name VARCHAR(255) NOT NULL
);

INSERT INTO language_codes_to_names (code, name) VALUES
('en-US', 'English (United States)'),
('ru-RU', 'Russian'),
('de-DE', 'German'),
('pl-PL', 'Polish'),
('fr-FR', 'French'),
('zh-Hans', 'Chinese (Simplified)'),
('es', 'Spanish'),
('pt-BR', 'Portuguese (Brazil)'),
('it', 'Italian'),
('uk', 'Ukrainian'),
('cs', 'Czech'),
('ko', 'Korean'),
('tr', 'Turkish'),
('nl', 'Dutch'),
('zh-Hant', 'Chinese (Traditional)'),
('ja', 'Japanese'),
('hu', 'Hungarian'),
('es-ES', 'Spanish (Spain)'),
('sr', 'Serbian'),
('da', 'Danish'),
('sv', 'Swedish'),
('cy', 'Welsh'),
('sk', 'Slovak'),
('es-MX', 'Spanish (Mexico)'),
('eu', 'Basque'),
('no', 'Norwegian'),
('sco', 'Scots'),
('so', 'Somali'),
('et', 'Estonian'),
('ca', 'Catalan'),
('id', 'Indonesian'),
('fi', 'Finnish'),
('wo', 'Wolof'),
('af', 'Afrikaans'),
('lb', 'Luxembourgish'),
('lt', 'Lithuanian'),
('ro', 'Romanian'),
('la', 'Latin'),
('uk-UA', 'Ukrainian (Ukraine)'),
('vi', 'Vietnamese'),
('fy', 'Frisian'),
('sl', 'Slovenian'),
('th', 'Thai'),
('ar', 'Arabic'),
('fo', 'Faroese'),
('ga', 'Irish'),
('it-IT', 'Italian (Italy)'),
('mt', 'Maltese'),
('br', 'Breton'),
('nv', 'Navajo'),
('ie', 'Interlingue'),
('eo', 'Esperanto'),
('dk', 'Danish'),
('vo', 'Volapük'),
('bg', 'Bulgarian'),
('mk', 'Macedonian'),
('wa', 'Walloon'),
('war', 'Waray'),
('nn', 'Norwegian Nynorsk'),
('rw', 'Kinyarwanda'),
('iw', 'Hebrew'),
('bi', 'Bislama'),
('el', 'Greek'),
('ab', 'Abkhazian'),
('sq', 'Albanian'),
('pt-PT', 'Portuguese (Portugal)'),
('is', 'Icelandic'),
('gd', 'Scottish Gaelic'),
('ko-KR', 'Korean (South Korea)'),
('kr', 'Korowai'),
('ug-Latn', 'Uighur (Latin)'),
('lv', 'Latvian'),
('mh', 'Marshallese'),
('ng', 'Ndonga'),
('be', 'Belarusian'),
('fj', 'Fijian'),
('gn', 'Guarani'),
('io', 'Ido'),
('co', 'Corsican'),
('ja-JP', 'Japanese (Japan)'),
('jv', 'Javanese'),
('ch', 'Chamorro'),
('mg', 'Malagasy'),
('qu', 'Quechua'),
('bs', 'Bosnian'),
('tt', 'Tatar'),
('uz', 'Uzbek'),
('xh', 'Xhosa'),
('om', 'Oromo'),
('nr', 'North Ndebele'),
('ceb', 'Cebuano'),
('sr-Latn', 'Serbian (Latin)'),
('az-Latn', 'Azerbaijani (Latin)'),
('mn-Cyrl', 'Mongolian (Cyrillic)'),
('tlh', 'Klingon'),
('az', 'Azerbaijani'),
('za', 'Zhuang'),
('ln', 'Lingala'),
('ay', 'Aymara'),
('lg', 'Ganda'),
('ha', 'Hausa'),
('ve', 'Venda'),
('kl', 'Greenlandic'),
('ka', 'Georgian'),
('ia', 'Interlingua'),
('hr', 'Croatian'),
('hmn', 'Hmong'),
('gl', 'Galician'),
('bs-Cyrl', 'Bosnian (Cyrillic)'),
('bs-Latn', 'Bosnian (Latin)'),
('sm', 'Samoan');


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

-- Create a materialized view for better performance with periodic updates
CREATE MATERIALIZED VIEW game_statistics AS
WITH review_stats AS (
    SELECT
        product_id,
        COUNT(*) AS total_reviews,
        AVG(rating_value) AS avg_rating,
        COUNT(CASE WHEN rating_value >= 4 THEN 1 END) AS positive_reviews,
        COUNT(CASE WHEN rating_value <= 2 THEN 1 END) AS negative_reviews,
        MAX(review_date) AS latest_review_date,
        SUM(upvotes) AS total_upvotes,
        SUM(downvotes) AS total_downvotes,
        COUNT(DISTINCT reviewer_id) AS unique_reviewers,
        COUNT(CASE WHEN array_length(labels, 1) > 0 THEN 1 END) AS labeled_reviews
    FROM reviews
    GROUP BY product_id
),
price_stats AS (
    SELECT
        price_currency,
        AVG(price_final) AS avg_price,
        MIN(price_final) AS min_price,
        MAX(price_final) AS max_price,
        AVG(price_discount) AS avg_discount,
        COUNT(CASE WHEN price_discount > 0 THEN 1 END) AS games_on_sale,
        COUNT(*) AS total_games_in_currency
    FROM games
    WHERE price_currency IS NOT NULL AND price_currency != '' -- Add this WHERE clause
    GROUP BY price_currency
),
genre_stats AS (
    SELECT
        unnest(genres) AS genre,
        COUNT(*) AS game_count,
        AVG(reviews_rating) AS avg_genre_rating,
        AVG(price_final) AS avg_genre_price
    FROM games
    GROUP BY genre
),
os_stats AS (
    SELECT
        unnest(operating_systems) AS os,
        COUNT(*) AS game_count
    FROM games
    GROUP BY os
),
language_stats AS (
    SELECT
        user_preferred_language_code AS language_code,
        lc.name AS language_name,
        COUNT(*) AS game_count
    FROM games g
    JOIN language_codes_to_names lc ON g.user_preferred_language_code = lc.code
    GROUP BY user_preferred_language_code, lc.name
),
publisher_stats AS (
    SELECT
        unnest(publishers) AS publisher,
        COUNT(*) AS published_games,
        AVG(reviews_rating) AS avg_publisher_rating
    FROM games
    GROUP BY publisher
),
developer_stats AS (
    SELECT
        unnest(developers) AS developer,
        COUNT(*) AS developed_games,
        AVG(reviews_rating) AS avg_developer_rating
    FROM games
    GROUP BY developer
),
time_stats AS (
    SELECT
        date_trunc('month', release_date) AS release_month,
        COUNT(*) AS games_released,
        AVG(reviews_rating) AS avg_monthly_rating
    FROM games
    GROUP BY release_month
    ORDER BY release_month
)
SELECT
    -- General statistics
    (SELECT COUNT(*) FROM games) AS total_games,
    (SELECT COUNT(*) FROM reviews) AS total_reviews,
    (SELECT AVG(reviews_rating) FROM games) AS avg_game_rating,
    (SELECT COUNT(*) FROM games WHERE reviews_count > 0) AS games_with_reviews,
    
    -- Price statistics by currency
    (SELECT json_agg(jsonb_build_object(
        'currency', p.price_currency,
        'avg_price', p.avg_price,
        'min_price', p.min_price,
        'max_price', p.max_price,
        'avg_discount', p.avg_discount,
        'games_on_sale', p.games_on_sale,
        'total_games', p.total_games_in_currency
    )) FROM price_stats p) AS price_statistics,
    
    -- Top genres
    (SELECT json_agg(jsonb_build_object(
        'genre', genre,
        'game_count', game_count,
        'avg_rating', avg_genre_rating,
        'avg_price', avg_genre_price
    ))
    FROM (SELECT * FROM genre_stats ORDER BY game_count DESC LIMIT 10) AS top_genres) AS top_genres,
    
    -- Operating system distribution
    (SELECT json_agg(jsonb_build_object(
        'os', o.os,
        'game_count', o.game_count
    )) FROM os_stats o) AS os_distribution,
    
    -- Top publishers
    (SELECT json_agg(jsonb_build_object(
        'publisher', publisher,
        'published_games', published_games,
        'avg_rating', avg_publisher_rating
    ))
    FROM (SELECT * FROM publisher_stats ORDER BY published_games DESC LIMIT 10) AS top_pubs) AS top_publishers,
    
    -- Top developers
    (SELECT json_agg(jsonb_build_object(
        'developer', developer,
        'developed_games', developed_games,
        'avg_rating', avg_developer_rating
    ))
    FROM (SELECT * FROM developer_stats ORDER BY developed_games DESC LIMIT 10) AS top_devs) AS top_developers,
    
    -- Release trends
    (SELECT json_agg(jsonb_build_object(
        'month', release_month,
        'games_released', games_released,
        'avg_rating', avg_monthly_rating
    ))
    FROM (SELECT * FROM time_stats ORDER BY release_month DESC LIMIT 24) AS recent_months) AS release_trends,
    
    -- Games with most reviews
    (SELECT json_agg(jsonb_build_object(
        'id', id,
        'title', title,
        'reviews_count', reviews_count,
        'rating', reviews_rating,
        'release_date', release_date
    ))
    FROM (SELECT id, title, reviews_count, reviews_rating, release_date 
          FROM games 
          ORDER BY reviews_count DESC LIMIT 10) AS most_reviewed) AS most_reviewed_games,
    
    -- Highest rated games
    (SELECT json_agg(jsonb_build_object(
        'id', id,
        'title', title,
        'reviews_count', reviews_count,
        'rating', reviews_rating,
        'release_date', release_date
    ))
    FROM (SELECT id, title, reviews_count, reviews_rating, release_date 
          FROM games 
          WHERE reviews_count > 10 -- Minimum threshold to avoid games with few reviews
          ORDER BY reviews_rating DESC LIMIT 10) AS top_rated) AS highest_rated_games,
    
    -- Latest updated
    (SELECT json_agg(jsonb_build_object(
        'id', id,
        'title', title,
        'updated_at', updated_at
    ))
    FROM (SELECT id, title, updated_at 
          FROM games 
          ORDER BY updated_at DESC NULLS LAST LIMIT 10) AS latest) AS recently_updated_games,
    
    now() AS view_refresh_time
;

-- Create an index on the materialized view for faster queries
CREATE UNIQUE INDEX ON game_statistics (view_refresh_time);

-- Create a trigger function to refresh the view when relevant tables change
CREATE OR REPLACE FUNCTION trigger_refresh_game_statistics()
RETURNS TRIGGER AS $$
BEGIN
    -- Use pg_notify to signal that statistics should be refreshed
    -- This avoids refreshing immediately after every change
    PERFORM pg_notify('refresh_statistics', 'true');
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

-- Create triggers on both tables
CREATE or replace TRIGGER refresh_stats_games
AFTER INSERT OR UPDATE OR DELETE ON games
FOR EACH STATEMENT EXECUTE FUNCTION trigger_refresh_game_statistics();

create or replace  TRIGGER refresh_stats_reviews
AFTER INSERT OR UPDATE OR DELETE ON reviews
FOR EACH STATEMENT EXECUTE FUNCTION trigger_refresh_game_statistics();
