CREATE TABLE IF NOT EXISTS public.standing_mandates (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    owner_identity character varying(255) NOT NULL,
    name character varying(255) NOT NULL,
    purpose text NOT NULL,
    status character varying(32) DEFAULT 'draft'::character varying NOT NULL,
    version character varying(64) NOT NULL,
    revision bigint DEFAULT 1 NOT NULL,
    scopes_json jsonb DEFAULT '[]'::jsonb NOT NULL,
    autonomy_ceiling smallint NOT NULL,
    approval_policy_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    stop_conditions_json jsonb DEFAULT '[]'::jsonb NOT NULL,
    source_references_json jsonb DEFAULT '[]'::jsonb NOT NULL,
    created_by character varying(255) NOT NULL,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL,
    activated_at timestamp with time zone,
    expires_at timestamp with time zone,
    revoked_at timestamp with time zone,
    revoked_by character varying(255) DEFAULT ''::character varying NOT NULL,
    revocation_reason text DEFAULT ''::text NOT NULL,
    CONSTRAINT standing_mandates_pkey PRIMARY KEY (id),
    CONSTRAINT uq_standing_mandates_owner_id UNIQUE (owner_identity, id),
    CONSTRAINT chk_standing_mandates_status
        CHECK (status::text = ANY (ARRAY['draft', 'active', 'revoked']::text[])),
    CONSTRAINT chk_standing_mandates_revision CHECK (revision > 0),
    CONSTRAINT chk_standing_mandates_autonomy CHECK (autonomy_ceiling BETWEEN 0 AND 10),
    CONSTRAINT chk_standing_mandates_scopes_array
        CHECK (jsonb_typeof(scopes_json) = 'array' AND jsonb_array_length(scopes_json) > 0),
    CONSTRAINT chk_standing_mandates_approval_object
        CHECK (jsonb_typeof(approval_policy_json) = 'object'),
    CONSTRAINT chk_standing_mandates_stops_array
        CHECK (jsonb_typeof(stop_conditions_json) = 'array'),
    CONSTRAINT chk_standing_mandates_sources_array
        CHECK (jsonb_typeof(source_references_json) = 'array'),
    CONSTRAINT chk_standing_mandates_expiry
        CHECK (expires_at IS NULL OR expires_at > created_at),
    CONSTRAINT chk_standing_mandates_lifecycle
        CHECK (
            (status = 'draft' AND activated_at IS NULL AND revoked_at IS NULL)
            OR
            (status = 'active' AND activated_at IS NOT NULL AND revoked_at IS NULL)
            OR
            (
                status = 'revoked'
                AND revoked_at IS NOT NULL
                AND length(btrim(revoked_by)) > 0
                AND length(btrim(revocation_reason)) > 0
            )
        )
);

CREATE INDEX IF NOT EXISTS idx_standing_mandates_owner_status
    ON public.standing_mandates USING btree (owner_identity, status);
CREATE INDEX IF NOT EXISTS idx_standing_mandates_expiry
    ON public.standing_mandates USING btree (expires_at)
    WHERE expires_at IS NOT NULL;

CREATE TABLE IF NOT EXISTS public.standing_mandate_authorization_decisions (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    mandate_id uuid NOT NULL,
    owner_identity character varying(255) NOT NULL,
    actor_identity character varying(255) NOT NULL,
    action character varying(256) NOT NULL,
    outcome character varying(32) NOT NULL,
    reason text NOT NULL,
    effective_autonomy smallint NOT NULL,
    approval_required boolean DEFAULT false NOT NULL,
    approval_satisfied boolean DEFAULT false NOT NULL,
    mandate_revision bigint NOT NULL,
    request_digest character(64) NOT NULL,
    mandate_digest character(64) NOT NULL,
    decision_digest character(64) NOT NULL,
    evidence_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    evaluated_at timestamp with time zone NOT NULL,
    CONSTRAINT standing_mandate_authorization_decisions_pkey PRIMARY KEY (id),
    CONSTRAINT uq_standing_mandate_authorization_decisions_digest UNIQUE (decision_digest),
    CONSTRAINT fk_standing_mandate_authorization_decisions_mandate
        FOREIGN KEY (owner_identity, mandate_id)
        REFERENCES public.standing_mandates(owner_identity, id)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT chk_standing_mandate_decisions_outcome
        CHECK (outcome::text = ANY (
            ARRAY['authorized', 'requires_approval', 'denied']::text[]
        )),
    CONSTRAINT chk_standing_mandate_decisions_autonomy
        CHECK (effective_autonomy BETWEEN 0 AND 10),
    CONSTRAINT chk_standing_mandate_decisions_revision CHECK (mandate_revision > 0),
    CONSTRAINT chk_standing_mandate_decisions_request_digest
        CHECK (request_digest ~ '^[0-9a-f]{64}$'),
    CONSTRAINT chk_standing_mandate_decisions_mandate_digest
        CHECK (mandate_digest ~ '^[0-9a-f]{64}$'),
    CONSTRAINT chk_standing_mandate_decisions_decision_digest
        CHECK (decision_digest ~ '^[0-9a-f]{64}$'),
    CONSTRAINT chk_standing_mandate_decisions_evidence_object
        CHECK (jsonb_typeof(evidence_json) = 'object'),
    CONSTRAINT chk_standing_mandate_decisions_approval
        CHECK (NOT approval_satisfied OR approval_required)
);

CREATE INDEX IF NOT EXISTS idx_standing_mandate_decisions_owner_evaluated
    ON public.standing_mandate_authorization_decisions
    USING btree (owner_identity, evaluated_at DESC);
CREATE INDEX IF NOT EXISTS idx_standing_mandate_decisions_mandate_evaluated
    ON public.standing_mandate_authorization_decisions
    USING btree (mandate_id, evaluated_at DESC);
CREATE INDEX IF NOT EXISTS idx_standing_mandate_decisions_action
    ON public.standing_mandate_authorization_decisions USING btree (action);
CREATE INDEX IF NOT EXISTS idx_standing_mandate_decisions_outcome
    ON public.standing_mandate_authorization_decisions USING btree (outcome);
CREATE INDEX IF NOT EXISTS idx_standing_mandate_decisions_request_digest
    ON public.standing_mandate_authorization_decisions USING btree (request_digest);
CREATE INDEX IF NOT EXISTS idx_standing_mandate_decisions_mandate_digest
    ON public.standing_mandate_authorization_decisions USING btree (mandate_digest);

CREATE OR REPLACE FUNCTION public.hai_enforce_standing_mandate_lifecycle()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    IF NEW.id <> OLD.id
        OR NEW.owner_identity <> OLD.owner_identity
        OR NEW.name <> OLD.name
        OR NEW.purpose <> OLD.purpose
        OR NEW.version <> OLD.version
        OR NEW.scopes_json <> OLD.scopes_json
        OR NEW.autonomy_ceiling <> OLD.autonomy_ceiling
        OR NEW.approval_policy_json <> OLD.approval_policy_json
        OR NEW.stop_conditions_json <> OLD.stop_conditions_json
        OR NEW.source_references_json <> OLD.source_references_json
        OR NEW.created_by <> OLD.created_by
        OR NEW.created_at <> OLD.created_at
        OR NEW.expires_at IS DISTINCT FROM OLD.expires_at
    THEN
        RAISE EXCEPTION 'standing mandate policy content is immutable; create a new version';
    END IF;

    IF NEW.revision <> OLD.revision + 1 OR NEW.updated_at <= OLD.updated_at THEN
        RAISE EXCEPTION 'standing mandate update must advance revision and timestamp exactly once';
    END IF;

    IF NOT (
        (OLD.status = 'draft' AND NEW.status IN ('active', 'revoked'))
        OR (OLD.status = 'active' AND NEW.status = 'revoked')
    ) THEN
        RAISE EXCEPTION 'invalid standing mandate lifecycle transition';
    END IF;

    IF NEW.status = 'active' AND (
        NEW.activated_at IS NULL
        OR NEW.revoked_at IS NOT NULL
        OR NEW.revoked_by <> ''
        OR NEW.revocation_reason <> ''
    ) THEN
        RAISE EXCEPTION 'invalid active standing mandate metadata';
    END IF;

    IF NEW.status = 'revoked' AND (
        NEW.revoked_at IS NULL
        OR length(btrim(NEW.revoked_by)) = 0
        OR length(btrim(NEW.revocation_reason)) = 0
    ) THEN
        RAISE EXCEPTION 'revocation metadata is required';
    END IF;

    RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS trg_standing_mandates_lifecycle
    ON public.standing_mandates;
CREATE TRIGGER trg_standing_mandates_lifecycle
    BEFORE UPDATE ON public.standing_mandates
    FOR EACH ROW EXECUTE FUNCTION public.hai_enforce_standing_mandate_lifecycle();

CREATE OR REPLACE FUNCTION public.hai_reject_standing_mandate_delete()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE EXCEPTION 'standing mandates are durable lifecycle records and cannot be deleted';
END;
$$;

DROP TRIGGER IF EXISTS trg_standing_mandates_no_delete
    ON public.standing_mandates;
CREATE TRIGGER trg_standing_mandates_no_delete
    BEFORE DELETE OR TRUNCATE ON public.standing_mandates
    FOR EACH STATEMENT EXECUTE FUNCTION public.hai_reject_standing_mandate_delete();

CREATE OR REPLACE FUNCTION public.hai_reject_standing_mandate_mutation()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE EXCEPTION 'standing mandate authorization decisions are immutable';
END;
$$;

DROP TRIGGER IF EXISTS trg_standing_mandate_decisions_immutable
    ON public.standing_mandate_authorization_decisions;
CREATE TRIGGER trg_standing_mandate_decisions_immutable
    BEFORE UPDATE OR DELETE ON public.standing_mandate_authorization_decisions
    FOR EACH ROW EXECUTE FUNCTION public.hai_reject_standing_mandate_mutation();

DROP TRIGGER IF EXISTS trg_standing_mandate_decisions_no_truncate
    ON public.standing_mandate_authorization_decisions;
CREATE TRIGGER trg_standing_mandate_decisions_no_truncate
    BEFORE TRUNCATE ON public.standing_mandate_authorization_decisions
    FOR EACH STATEMENT EXECUTE FUNCTION public.hai_reject_standing_mandate_mutation();
