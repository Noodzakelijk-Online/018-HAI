ALTER TABLE public.workflow_items
    ADD COLUMN coordination_draft_plan_id uuid,
    ADD COLUMN coordination_draft_revision bigint NOT NULL DEFAULT 0,
    ADD COLUMN coordination_draft_digest character(64) NOT NULL DEFAULT '',
    ADD COLUMN coordination_draft_node_id character varying(160) NOT NULL DEFAULT '';

ALTER TABLE public.workflow_items
    ADD CONSTRAINT chk_workflow_coordination_draft_binding_shape CHECK (
        (
            coordination_draft_plan_id IS NULL AND
            coordination_draft_revision = 0 AND
            coordination_draft_digest = '' AND
            coordination_draft_node_id = ''
        ) OR (
            coordination_draft_plan_id IS NOT NULL AND
            owner_identity IS NOT NULL AND owner_identity <> '' AND
            coordination_draft_revision = 1 AND
            coordination_draft_digest ~ '^[0-9a-f]{64}$' AND
            coordination_draft_node_id <> ''
        )
    ),
    ADD CONSTRAINT fk_workflow_coordination_draft_revision
    FOREIGN KEY (owner_identity, coordination_draft_plan_id, coordination_draft_revision, coordination_draft_digest)
    REFERENCES public.plan_graph_revisions (owner_identity, plan_id, revision, digest)
    ON UPDATE RESTRICT
    ON DELETE RESTRICT;

CREATE OR REPLACE FUNCTION public.hai_validate_workflow_coordination_draft_binding()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    bound_status character varying(24);
    bound_payload jsonb;
BEGIN
    IF NEW.coordination_draft_plan_id IS NULL THEN
        RETURN NEW;
    END IF;

    SELECT status, payload
      INTO bound_status, bound_payload
      FROM public.plan_graph_revisions
     WHERE owner_identity = NEW.owner_identity
       AND plan_id = NEW.coordination_draft_plan_id
       AND revision = NEW.coordination_draft_revision
       AND digest = NEW.coordination_draft_digest;

    IF bound_status IS DISTINCT FROM 'draft' THEN
        RAISE EXCEPTION 'workflow coordination draft must reference a draft plan revision';
    END IF;
    IF NOT EXISTS (
        SELECT 1
          FROM jsonb_array_elements(bound_payload -> 'nodes') AS node
         WHERE node ->> 'id' = NEW.coordination_draft_node_id
           AND node ->> 'owner' = NEW.owner_identity
           AND node -> 'bindings' ->> 'workflowId' = NEW.id::text
    ) THEN
        RAISE EXCEPTION 'workflow coordination draft node must bind the same workflow and owner';
    END IF;
    RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS trg_workflow_coordination_draft_binding ON public.workflow_items;
CREATE CONSTRAINT TRIGGER trg_workflow_coordination_draft_binding
AFTER INSERT OR UPDATE
ON public.workflow_items
DEFERRABLE INITIALLY IMMEDIATE
FOR EACH ROW EXECUTE FUNCTION public.hai_validate_workflow_coordination_draft_binding();

CREATE INDEX idx_workflow_items_owner_coordination_draft
    ON public.workflow_items (owner_identity, coordination_draft_plan_id, coordination_draft_revision)
    WHERE coordination_draft_plan_id IS NOT NULL;

COMMENT ON COLUMN public.workflow_items.coordination_draft_plan_id IS
    'Exact immutable advisory draft projected from workflow intake. It grants no approval or execution authority.';
