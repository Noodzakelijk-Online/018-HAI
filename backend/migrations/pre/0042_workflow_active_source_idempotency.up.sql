DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM public.workflow_items
        WHERE archived = false
          AND btrim(owner_identity) <> ''
          AND btrim(source_type) <> ''
          AND btrim(source_id) <> ''
        GROUP BY owner_identity, source_type, source_id
        HAVING count(*) > 1
    ) THEN
        RAISE EXCEPTION 'cannot enforce workflow source idempotency while duplicate active source identities exist';
    END IF;
END
$$;

CREATE UNIQUE INDEX idx_workflow_items_active_owner_source_identity
    ON public.workflow_items (owner_identity, source_type, source_id)
    WHERE archived = false
      AND btrim(owner_identity) <> ''
      AND btrim(source_type) <> ''
      AND btrim(source_id) <> '';
