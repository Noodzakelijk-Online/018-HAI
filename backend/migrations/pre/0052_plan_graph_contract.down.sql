DO $$
BEGIN
    IF to_regclass('public.plan_graph_revisions') IS NOT NULL
       AND EXISTS (SELECT 1 FROM public.plan_graph_revisions LIMIT 1) THEN
        RAISE EXCEPTION 'refusing to roll back non-empty immutable plan graph revision history';
    END IF;
END;
$$;

DROP TRIGGER IF EXISTS trg_plan_graph_revision_no_truncate ON public.plan_graph_revisions;
DROP TRIGGER IF EXISTS trg_plan_graph_revision_immutable ON public.plan_graph_revisions;
DROP TRIGGER IF EXISTS trg_plan_graph_revision_insert ON public.plan_graph_revisions;
DROP FUNCTION IF EXISTS public.hai_reject_plan_graph_truncate();
DROP FUNCTION IF EXISTS public.hai_reject_plan_graph_mutation();
DROP FUNCTION IF EXISTS public.hai_validate_plan_graph_revision_insert();
DROP TABLE IF EXISTS public.plan_graph_revisions;
