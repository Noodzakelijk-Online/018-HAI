-- Compatibility migration 0024 reconciles installations that applied an
-- earlier 0014 with the schema currently owned by 0014. Removing these columns
-- would make the current repository unusable and would also damage fresh
-- installations where they predate 0024, so rollback is intentionally a no-op.
DO $$
BEGIN
    RAISE NOTICE '0024 compatibility schema remains in place on rollback';
END;
$$;
