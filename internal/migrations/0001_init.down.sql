-- Rollback for 0001_init.
DROP TRIGGER IF EXISTS trg_conversations_touch ON conversations;
DROP FUNCTION IF EXISTS touch_updated_at();
DROP TABLE IF EXISTS usage_events;
DROP TABLE IF EXISTS messages;
DROP TABLE IF EXISTS conversations;
DROP TABLE IF EXISTS application_settings;
