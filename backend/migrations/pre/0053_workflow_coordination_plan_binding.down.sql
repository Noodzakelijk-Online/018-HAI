DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM public.workflow_items
        WHERE coordination_plan_id IS NOT NULL
    ) THEN
        RAISE EXCEPTION 'refusing to remove non-empty workflow coordination plan bindings';
    END IF;
END;
$$;

DROP INDEX IF EXISTS public.idx_workflow_items_owner_coordination_plan;

ALTER TABLE public.workflow_items
    DROP CONSTRAINT IF EXISTS fk_workflow_coordination_plan_revision,
    DROP CONSTRAINT IF EXISTS chk_workflow_coordination_plan_binding_shape,
    DROP COLUMN IF EXISTS coordination_plan_node_id,
    DROP COLUMN IF EXISTS coordination_plan_digest,
    DROP COLUMN IF EXISTS coordination_plan_revision,
    DROP COLUMN IF EXISTS coordination_plan_id;

ALTER TABLE public.plan_graph_revisions
    DROP CONSTRAINT IF EXISTS uq_plan_graph_owner_plan_revision_digest;
