-- Reverse the owner-scoped uniqueness, restoring the original global index.
DROP INDEX IF EXISTS idx_ai_conversation_owner_identity;
CREATE UNIQUE INDEX IF NOT EXISTS idx_ai_conversation_identity
  ON ai_conversation_archives (platform, external_id);
