DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM public.workflow_items
        WHERE coordination_draft_plan_id IS NOT NULL
    ) THEN
        RAISE EXCEPTION 'refusing to remove non-empty workflow coordination draft bindings';
    END IF;
END;
$$;

DROP INDEX IF EXISTS public.idx_workflow_items_owner_coordination_draft;
DROP TRIGGER IF EXISTS trg_workflow_coordination_draft_binding ON public.workflow_items;
DROP FUNCTION IF EXISTS public.hai_validate_workflow_coordination_draft_binding();

ALTER TABLE public.workflow_items
    DROP CONSTRAINT IF EXISTS fk_workflow_coordination_draft_revision,
    DROP CONSTRAINT IF EXISTS chk_workflow_coordination_draft_binding_shape,
    DROP COLUMN IF EXISTS coordination_draft_node_id,
    DROP COLUMN IF EXISTS coordination_draft_digest,
    DROP COLUMN IF EXISTS coordination_draft_revision,
    DROP COLUMN IF EXISTS coordination_draft_plan_id;
