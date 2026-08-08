DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM public.workflow_items
        WHERE mandate_id IS NOT NULL
        UNION ALL
        SELECT 1
        FROM public.pursuits
        WHERE mandate_id IS NOT NULL
    ) THEN
        RAISE EXCEPTION 'refusing to remove non-empty workflow standing mandate bindings';
    END IF;
END;
$$;

DROP INDEX public.idx_workflow_items_owner_mandate;

DROP INDEX public.idx_pursuits_owner_mandate;

ALTER TABLE public.pursuits
    DROP CONSTRAINT fk_pursuits_owner_mandate,
    DROP COLUMN mandate_id;

ALTER TABLE public.workflow_items
    DROP CONSTRAINT fk_workflow_items_owner_mandate,
    DROP COLUMN mandate_id;
