-- Snake.io PostgreSQL Schema Bootstrap

CREATE TABLE IF NOT EXISTS players (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    username    VARCHAR(64) NOT NULL UNIQUE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS player_stats (
    player_id       UUID PRIMARY KEY REFERENCES players(id) ON DELETE CASCADE,
    games_played    BIGINT  NOT NULL DEFAULT 0,
    total_kills     BIGINT  NOT NULL DEFAULT 0,
    total_score     BIGINT  NOT NULL DEFAULT 0,
    highest_score   BIGINT  NOT NULL DEFAULT 0,
    total_playtime  INTERVAL NOT NULL DEFAULT '0'
);

CREATE TABLE IF NOT EXISTS leaderboard (
    id          BIGSERIAL PRIMARY KEY,
    player_id   UUID NOT NULL REFERENCES players(id) ON DELETE CASCADE,
    score       BIGINT NOT NULL,
    kills       INT NOT NULL DEFAULT 0,
    duration    INTERVAL NOT NULL DEFAULT '0',
    played_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_leaderboard_score ON leaderboard(score DESC);
CREATE INDEX idx_leaderboard_played_at ON leaderboard(played_at DESC);
