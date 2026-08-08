ALTER TABLE public.pursuit_portfolio_allocations
    ADD COLUMN coordination_plan_id uuid,
    ADD COLUMN coordination_plan_revision bigint NOT NULL DEFAULT 0,
    ADD COLUMN coordination_plan_digest character(64) NOT NULL DEFAULT '',
    ADD COLUMN coordination_plan_node_id character varying(160) NOT NULL DEFAULT '';

ALTER TABLE public.pursuit_portfolio_allocations
    ADD CONSTRAINT chk_pursuit_portfolio_coordination_plan_binding_shape CHECK (
        (
            coordination_plan_id IS NULL AND
            coordination_plan_revision = 0 AND
            coordination_plan_digest = '' AND
            coordination_plan_node_id = ''
        ) OR (
            coordination_plan_id IS NOT NULL AND
            coordination_plan_revision > 0 AND
            coordination_plan_digest ~ '^[0-9a-f]{64}$' AND
            length(btrim(coordination_plan_node_id)) > 0
        )
    ),
    ADD CONSTRAINT fk_pursuit_portfolio_coordination_plan_revision
    FOREIGN KEY (owner_identity, coordination_plan_id, coordination_plan_revision, coordination_plan_digest)
    REFERENCES public.plan_graph_revisions (owner_identity, plan_id, revision, digest)
    ON UPDATE RESTRICT
    ON DELETE RESTRICT;

CREATE INDEX idx_pursuit_portfolio_allocations_owner_coordination_plan
    ON public.pursuit_portfolio_allocations (
        owner_identity,
        coordination_plan_id,
        coordination_plan_revision
    )
    WHERE coordination_plan_id IS NOT NULL;

CREATE OR REPLACE FUNCTION public.hai_validate_pursuit_portfolio_coordination_plan_insert()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    IF NEW.coordination_plan_id IS NOT NULL AND NOT EXISTS (
        SELECT 1
        FROM public.plan_graph_revisions
        WHERE owner_identity = NEW.owner_identity
          AND plan_id = NEW.coordination_plan_id
          AND revision = NEW.coordination_plan_revision
          AND digest = NEW.coordination_plan_digest
          AND status = 'accepted'
    ) THEN
        RAISE EXCEPTION 'pursuit portfolio coordination plan must reference an exact accepted revision';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER pursuit_portfolio_coordination_plan_validate_insert
BEFORE INSERT ON public.pursuit_portfolio_allocations
FOR EACH ROW EXECUTE FUNCTION public.hai_validate_pursuit_portfolio_coordination_plan_insert();

COMMENT ON COLUMN public.pursuit_portfolio_allocations.coordination_plan_id IS
    'Optional exact accepted advisory plan provenance. This binding grants no execution authority.';
