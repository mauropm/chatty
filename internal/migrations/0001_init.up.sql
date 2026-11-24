-- ============================================================================
-- Chatty — initial schema (0001)
-- All timestamps are stored in UTC (TIMESTAMPTZ). UUIDs use gen_random_uuid().
-- ============================================================================

CREATE TABLE conversations (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    title       TEXT   NOT NULL DEFAULT 'New conversation',
    archived    BOOLEAN NOT NULL DEFAULT FALSE,
    metadata    JSONB  NOT NULL DEFAULT '{}'::jsonb,
    search      TSVECTOR GENERATED ALWAYS AS (to_tsvector('simple', title)) STORED,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE messages (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    conversation_id   UUID NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    role              TEXT NOT NULL CHECK (role IN ('user', 'assistant', 'system')),
    content           TEXT NOT NULL DEFAULT '',
    status            TEXT NOT NULL DEFAULT 'complete'
                        CHECK (status IN ('pending','streaming','complete','failed','stopped')),
    model             TEXT,
    prompt_tokens     BIGINT NOT NULL DEFAULT 0,
    completion_tokens BIGINT NOT NULL DEFAULT 0,
    latency_ms        BIGINT NOT NULL DEFAULT 0,
    error             TEXT,
    metadata          JSONB NOT NULL DEFAULT '{}'::jsonb,
    search            TSVECTOR GENERATED ALWAYS AS (to_tsvector('simple', content)) STORED,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at      TIMESTAMPTZ
);

CREATE TABLE usage_events (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    conversation_id   UUID REFERENCES conversations(id) ON DELETE CASCADE,
    message_id        UUID REFERENCES messages(id) ON DELETE CASCADE,
    event_type        TEXT NOT NULL DEFAULT 'chat_completion',
    model             TEXT,
    prompt_tokens     BIGINT NOT NULL DEFAULT 0,
    completion_tokens BIGINT NOT NULL DEFAULT 0,
    latency_ms        BIGINT NOT NULL DEFAULT 0,
    success           BOOLEAN NOT NULL DEFAULT TRUE,
    metadata          JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE application_settings (
    key        TEXT PRIMARY KEY,
    value      JSONB NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ---- indexes --------------------------------------------------------------

-- Conversation listing / ordering (active first, most recent update on top).
CREATE INDEX idx_conversations_active_updated ON conversations (archived, updated_at DESC);
CREATE INDEX idx_conversations_created ON conversations (created_at DESC);
CREATE INDEX idx_conversations_search ON conversations USING GIN (search);

-- Message retrieval per conversation in insertion order; global time ordering.
CREATE INDEX idx_messages_conversation_created ON messages (conversation_id, created_at, id);
CREATE INDEX idx_messages_created ON messages (created_at DESC);
CREATE INDEX idx_messages_search ON messages USING GIN (search);
CREATE INDEX idx_messages_conversation_status ON messages (conversation_id, status);

-- Usage reporting.
CREATE INDEX idx_usage_created ON usage_events (created_at DESC);
CREATE INDEX idx_usage_conversation ON usage_events (conversation_id);
CREATE INDEX idx_usage_model ON usage_events (model);

-- ---- trigger: keep conversations.updated_at fresh -------------------------

CREATE FUNCTION touch_updated_at() RETURNS trigger AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_conversations_touch
    BEFORE UPDATE ON conversations
    FOR EACH ROW EXECUTE FUNCTION touch_updated_at();
