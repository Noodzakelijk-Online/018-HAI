DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM public.pursuit_portfolio_allocations
        WHERE coordination_plan_id IS NOT NULL
    ) THEN
        RAISE EXCEPTION 'refusing to remove non-empty pursuit portfolio coordination plan bindings';
    END IF;
END;
$$;

DROP INDEX IF EXISTS public.idx_pursuit_portfolio_allocations_owner_coordination_plan;

DROP TRIGGER IF EXISTS pursuit_portfolio_coordination_plan_validate_insert
    ON public.pursuit_portfolio_allocations;
DROP FUNCTION IF EXISTS public.hai_validate_pursuit_portfolio_coordination_plan_insert();

ALTER TABLE public.pursuit_portfolio_allocations
    DROP CONSTRAINT IF EXISTS fk_pursuit_portfolio_coordination_plan_revision,
    DROP CONSTRAINT IF EXISTS chk_pursuit_portfolio_coordination_plan_binding_shape,
    DROP COLUMN IF EXISTS coordination_plan_node_id,
    DROP COLUMN IF EXISTS coordination_plan_digest,
    DROP COLUMN IF EXISTS coordination_plan_revision,
    DROP COLUMN IF EXISTS coordination_plan_id;
