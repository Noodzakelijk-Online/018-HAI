ALTER TABLE public.plan_graph_revisions
    ADD CONSTRAINT uq_plan_graph_owner_plan_revision_digest
    UNIQUE (owner_identity, plan_id, revision, digest);

ALTER TABLE public.workflow_items
    ADD COLUMN coordination_plan_id uuid,
    ADD COLUMN coordination_plan_revision bigint NOT NULL DEFAULT 0,
    ADD COLUMN coordination_plan_digest character(64) NOT NULL DEFAULT '',
    ADD COLUMN coordination_plan_node_id character varying(160) NOT NULL DEFAULT '';

ALTER TABLE public.workflow_items
    ADD CONSTRAINT chk_workflow_coordination_plan_binding_shape CHECK (
        (
            coordination_plan_id IS NULL AND
            coordination_plan_revision = 0 AND
            coordination_plan_digest = '' AND
            coordination_plan_node_id = ''
        ) OR (
            coordination_plan_id IS NOT NULL AND
            owner_identity IS NOT NULL AND owner_identity <> '' AND
            coordination_plan_revision > 0 AND
            coordination_plan_digest ~ '^[0-9a-f]{64}$' AND
            coordination_plan_node_id <> ''
        )
    ),
    ADD CONSTRAINT fk_workflow_coordination_plan_revision
    FOREIGN KEY (owner_identity, coordination_plan_id, coordination_plan_revision, coordination_plan_digest)
    REFERENCES public.plan_graph_revisions (owner_identity, plan_id, revision, digest)
    ON UPDATE RESTRICT
    ON DELETE RESTRICT;

CREATE INDEX idx_workflow_items_owner_coordination_plan
    ON public.workflow_items (owner_identity, coordination_plan_id, coordination_plan_revision)
    WHERE coordination_plan_id IS NOT NULL;

COMMENT ON COLUMN public.workflow_items.coordination_plan_id IS
    'Optional exact accepted advisory plan provenance. This binding grants no execution authority.';
