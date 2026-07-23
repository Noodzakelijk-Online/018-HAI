-- The extension is shared infrastructure; dropping it could break other objects
-- that depend on uuid_generate_v4(), so the rollback is intentionally a no-op.
-- (Kept as an explicit file so every up has a matching, reviewed down.)
SELECT 1;
