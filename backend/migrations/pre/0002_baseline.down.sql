-- Reversing the baseline would drop every application table and all data.
-- That is never something a migration tool should do automatically, so the
-- rollback is deliberately a no-op. To reset a development database, drop and
-- recreate the schema manually, then re-run `migrate up`.
SELECT 1;
