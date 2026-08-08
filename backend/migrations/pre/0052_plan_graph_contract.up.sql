CREATE TABLE IF NOT EXISTS public.plan_graph_revisions (
    row_id uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
    plan_id uuid NOT NULL,
    owner_identity character varying(255) NOT NULL,
    revision bigint NOT NULL CHECK (revision > 0),
    status character varying(24) NOT NULL CHECK (status IN ('draft', 'accepted')),
    digest character(64) NOT NULL CHECK (digest ~ '^[0-9a-f]{64}$'),
    parent_revision bigint NOT NULL DEFAULT 0 CHECK (parent_revision >= 0),
    parent_digest character varying(64) NOT NULL DEFAULT ''
        CHECK (parent_digest = '' OR parent_digest ~ '^[0-9a-f]{64}$'),
    idempotency_key text NOT NULL DEFAULT '',
    payload jsonb NOT NULL,
    created_by character varying(255) NOT NULL,
    created_at timestamp with time zone NOT NULL,
    accepted_at timestamp with time zone,
    CONSTRAINT uq_plan_graph_owner_plan_revision UNIQUE (owner_identity, plan_id, revision),
    CONSTRAINT chk_plan_graph_parent_shape CHECK (
        (revision = 1 AND parent_revision = 0 AND parent_digest = '') OR
        (revision > 1 AND parent_revision = revision - 1 AND parent_digest ~ '^[0-9a-f]{64}$')
    ),
    CONSTRAINT chk_plan_graph_acceptance_shape CHECK (
        (status = 'accepted' AND accepted_at IS NOT NULL) OR
        (status = 'draft' AND accepted_at IS NULL)
    ),
    CONSTRAINT chk_plan_graph_payload_object CHECK (jsonb_typeof(payload) = 'object'),
    CONSTRAINT chk_plan_graph_payload_required CHECK (
        payload ?& ARRAY['id', 'title', 'status', 'revision', 'digest', 'nodes', 'edges', 'createdBy', 'createdAt', 'canExecute']
    ),
    CONSTRAINT chk_plan_graph_payload_shape CHECK (
        jsonb_typeof(payload -> 'nodes') = 'array' AND
        jsonb_typeof(payload -> 'edges') = 'array'
    ),
    CONSTRAINT chk_plan_graph_payload_authority CHECK (
        COALESCE(payload -> 'canExecute', 'true'::jsonb) = 'false'::jsonb
    ),
    CONSTRAINT chk_plan_graph_payload_identity CHECK (
        COALESCE(payload ->> 'id', '') = plan_id::text AND
        COALESCE((payload ->> 'revision')::bigint, 0) = revision AND
        COALESCE(payload ->> 'status', '') = status AND
        COALESCE(payload ->> 'digest', '') = digest AND
        COALESCE((payload ->> 'parentRevision')::bigint, 0) = parent_revision AND
        COALESCE(payload ->> 'parentDigest', '') = parent_digest
    )
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_plan_graph_owner_idempotency
    ON public.plan_graph_revisions (owner_identity, idempotency_key)
    WHERE idempotency_key <> '';

CREATE INDEX IF NOT EXISTS idx_plan_graph_owner_latest
    ON public.plan_graph_revisions (owner_identity, plan_id, revision DESC);

CREATE INDEX IF NOT EXISTS idx_plan_graph_owner_created
    ON public.plan_graph_revisions (owner_identity, created_at DESC, plan_id);

CREATE OR REPLACE FUNCTION public.hai_validate_plan_graph_revision_insert()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    previous_digest character(64);
    latest_revision bigint;
BEGIN
    SELECT MAX(revision)
      INTO latest_revision
      FROM public.plan_graph_revisions
     WHERE owner_identity = NEW.owner_identity
       AND plan_id = NEW.plan_id;

    IF COALESCE(latest_revision, 0) + 1 <> NEW.revision THEN
        RAISE EXCEPTION 'plan graph revisions must be contiguous and append-only';
    END IF;

    IF NEW.revision > 1 THEN
        SELECT digest
          INTO previous_digest
          FROM public.plan_graph_revisions
         WHERE owner_identity = NEW.owner_identity
           AND plan_id = NEW.plan_id
           AND revision = NEW.revision - 1;
        IF previous_digest IS NULL OR previous_digest <> NEW.parent_digest THEN
            RAISE EXCEPTION 'plan graph parent digest must bind the previous revision';
        END IF;
    END IF;

    IF NEW.idempotency_key <> '' AND COALESCE(NEW.payload ->> 'requestDigest', '') !~ '^[0-9a-f]{64}$' THEN
        RAISE EXCEPTION 'idempotent plan graph revisions require a request digest';
    END IF;
    RETURN NEW;
END;
$$;

CREATE OR REPLACE FUNCTION public.hai_reject_plan_graph_mutation()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE EXCEPTION 'plan graph revisions are immutable';
END;
$$;

CREATE OR REPLACE FUNCTION public.hai_reject_plan_graph_truncate()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE EXCEPTION 'plan graph revision history cannot be truncated';
END;
$$;

DROP TRIGGER IF EXISTS trg_plan_graph_revision_insert ON public.plan_graph_revisions;
CREATE TRIGGER trg_plan_graph_revision_insert
BEFORE INSERT ON public.plan_graph_revisions
FOR EACH ROW EXECUTE FUNCTION public.hai_validate_plan_graph_revision_insert();

DROP TRIGGER IF EXISTS trg_plan_graph_revision_immutable ON public.plan_graph_revisions;
CREATE TRIGGER trg_plan_graph_revision_immutable
BEFORE UPDATE OR DELETE ON public.plan_graph_revisions
FOR EACH ROW EXECUTE FUNCTION public.hai_reject_plan_graph_mutation();

DROP TRIGGER IF EXISTS trg_plan_graph_revision_no_truncate ON public.plan_graph_revisions;
CREATE TRIGGER trg_plan_graph_revision_no_truncate
BEFORE TRUNCATE ON public.plan_graph_revisions
FOR EACH STATEMENT EXECUTE FUNCTION public.hai_reject_plan_graph_truncate();

COMMENT ON TABLE public.plan_graph_revisions IS
    'Owner-scoped immutable advisory plan graph revisions. Acceptance never grants execution authority.';
COMMENT ON COLUMN public.plan_graph_revisions.digest IS
    'Deterministic SHA-256 digest of the canonical hai-plan-graph-v1 envelope.';
