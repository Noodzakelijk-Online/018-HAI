-- Conversation identities were originally global. Keep legacy ownerless imports
-- readable, but make new records unique per authenticated owner so two local HAI
-- users cannot overwrite each other's imported history.
DROP INDEX IF EXISTS idx_ai_conversation_identity;
CREATE UNIQUE INDEX IF NOT EXISTS idx_ai_conversation_owner_identity
  ON ai_conversation_archives (owner_identity, platform, external_id);
