CREATE TABLE IF NOT EXISTS public.task_completion_plan_logs (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    owner_identity character varying(255) NOT NULL,
    task_plan_id character varying(160) NOT NULL,
    completion_status character varying(80) NOT NULL,
    verification_status character varying(80) DEFAULT 'not_run' NOT NULL,
    payload_json jsonb NOT NULL,
    payload_digest character(64) NOT NULL,
    provenance_source character varying(80) NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    CONSTRAINT task_completion_plan_logs_pkey PRIMARY KEY (id),
    CONSTRAINT chk_task_completion_plan_logs_owner CHECK (btrim(owner_identity) <> ''),
    CONSTRAINT chk_task_completion_plan_logs_plan CHECK (btrim(task_plan_id) <> ''),
    CONSTRAINT chk_task_completion_plan_logs_status CHECK (btrim(completion_status) <> ''),
    CONSTRAINT chk_task_completion_plan_logs_verification CHECK (btrim(verification_status) <> ''),
    CONSTRAINT chk_task_completion_plan_logs_payload_object CHECK (
        jsonb_typeof(payload_json) = 'object'
    ),
    CONSTRAINT chk_task_completion_plan_logs_payload_size CHECK (
        octet_length(payload_json::text) <= 2097152
    ),
    CONSTRAINT chk_task_completion_plan_logs_digest CHECK (
        payload_digest ~ '^[0-9a-f]{64}$'
    ),
    CONSTRAINT chk_task_completion_plan_logs_source CHECK (
        provenance_source = 'task-success-engine'
    )
);

CREATE INDEX IF NOT EXISTS idx_task_completion_plan_logs_owner_created
    ON public.task_completion_plan_logs USING btree (owner_identity, created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_task_completion_plan_logs_task_plan
    ON public.task_completion_plan_logs USING btree (task_plan_id);
CREATE INDEX IF NOT EXISTS idx_task_completion_plan_logs_status
    ON public.task_completion_plan_logs USING btree (completion_status);
CREATE INDEX IF NOT EXISTS idx_task_completion_plan_logs_verification
    ON public.task_completion_plan_logs USING btree (verification_status);
CREATE INDEX IF NOT EXISTS idx_task_completion_plan_logs_payload_digest
    ON public.task_completion_plan_logs USING btree (payload_digest);
CREATE UNIQUE INDEX IF NOT EXISTS idx_task_completion_plan_logs_idempotency
    ON public.task_completion_plan_logs USING btree (
        owner_identity,
        task_plan_id,
        payload_digest
    );

CREATE TABLE IF NOT EXISTS public.task_review_items (
    id uuid NOT NULL,
    owner_identity character varying(255) NOT NULL,
    original_task_plan_id character varying(160) NOT NULL,
    current_task_plan_id character varying(160) NOT NULL,
    request_digest character(64) NOT NULL,
    request_json jsonb NOT NULL,
    reason text NOT NULL,
    priority character varying(32) DEFAULT 'normal' NOT NULL,
    status character varying(32) NOT NULL,
    review_revision integer DEFAULT 1 NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    resolved_at timestamp with time zone,
    CONSTRAINT task_review_items_pkey PRIMARY KEY (id),
    CONSTRAINT chk_task_review_items_owner CHECK (btrim(owner_identity) <> ''),
    CONSTRAINT chk_task_review_items_original_plan CHECK (btrim(original_task_plan_id) <> ''),
    CONSTRAINT chk_task_review_items_current_plan CHECK (btrim(current_task_plan_id) <> ''),
    CONSTRAINT chk_task_review_items_digest CHECK (request_digest ~ '^[0-9a-f]{64}$'),
    CONSTRAINT chk_task_review_items_request_object CHECK (
        jsonb_typeof(request_json) = 'object'
    ),
    CONSTRAINT chk_task_review_items_request_size CHECK (
        octet_length(request_json::text) <= 2097152
    ),
    CONSTRAINT chk_task_review_items_reason CHECK (btrim(reason) <> ''),
    CONSTRAINT chk_task_review_items_reason_size CHECK (
        char_length(reason) <= 4096
    ),
    CONSTRAINT chk_task_review_items_priority CHECK (
        priority IN ('low', 'normal', 'high', 'critical')
    ),
    CONSTRAINT chk_task_review_items_status CHECK (
        status IN ('open', 'needs_review', 'approved', 'rejected', 'completed')
    ),
    CONSTRAINT chk_task_review_items_revision CHECK (review_revision >= 1),
    CONSTRAINT chk_task_review_items_timestamps CHECK (
        updated_at >= created_at
    ),
    CONSTRAINT chk_task_review_items_resolution_time CHECK (
        (
            status IN ('open', 'needs_review') AND
            resolved_at IS NULL
        ) OR (
            status IN ('approved', 'rejected', 'completed') AND
            resolved_at IS NOT NULL AND
            resolved_at >= created_at
        )
    )
);

CREATE INDEX IF NOT EXISTS idx_task_review_items_owner_created
    ON public.task_review_items USING btree (owner_identity, created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_task_review_items_original_plan
    ON public.task_review_items USING btree (original_task_plan_id);
CREATE INDEX IF NOT EXISTS idx_task_review_items_current_plan
    ON public.task_review_items USING btree (current_task_plan_id);
CREATE INDEX IF NOT EXISTS idx_task_review_items_request_digest
    ON public.task_review_items USING btree (request_digest);
CREATE INDEX IF NOT EXISTS idx_task_review_items_priority
    ON public.task_review_items USING btree (priority);
CREATE INDEX IF NOT EXISTS idx_task_review_items_status
    ON public.task_review_items USING btree (status);
CREATE INDEX IF NOT EXISTS idx_task_review_items_resolved_at
    ON public.task_review_items USING btree (resolved_at);

CREATE TABLE IF NOT EXISTS public.task_review_decisions (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    review_item_id uuid NOT NULL,
    review_revision integer NOT NULL,
    owner_identity character varying(255) NOT NULL,
    task_plan_id character varying(160) NOT NULL,
    decision character varying(32) NOT NULL,
    resolution_note character varying(512) DEFAULT '' NOT NULL,
    resolved_by character varying(255) NOT NULL,
    approval_source character varying(80) NOT NULL,
    approval_source_id character varying(160) NOT NULL,
    request_digest character(64) NOT NULL,
    resolved_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    CONSTRAINT task_review_decisions_pkey PRIMARY KEY (id),
    CONSTRAINT task_review_decisions_review_item_fkey FOREIGN KEY (review_item_id)
        REFERENCES public.task_review_items(id) ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT chk_task_review_decisions_owner CHECK (btrim(owner_identity) <> ''),
    CONSTRAINT chk_task_review_decisions_revision CHECK (review_revision >= 1),
    CONSTRAINT chk_task_review_decisions_plan CHECK (btrim(task_plan_id) <> ''),
    CONSTRAINT chk_task_review_decisions_decision CHECK (
        decision IN ('approved', 'rejected')
    ),
    CONSTRAINT chk_task_review_decisions_resolved_by CHECK (btrim(resolved_by) <> ''),
    CONSTRAINT chk_task_review_decisions_resolver_owner CHECK (
        resolved_by = owner_identity
    ),
    CONSTRAINT chk_task_review_decisions_source CHECK (
        approval_source = 'task-review'
    ),
    CONSTRAINT chk_task_review_decisions_source_id CHECK (
        approval_source_id = 'task-review:' || review_item_id::text
    ),
    CONSTRAINT chk_task_review_decisions_digest CHECK (
        request_digest ~ '^[0-9a-f]{64}$'
    )
);

CREATE INDEX IF NOT EXISTS idx_task_review_decisions_item_resolved
    ON public.task_review_decisions USING btree (review_item_id, resolved_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_task_review_decisions_owner_resolved
    ON public.task_review_decisions USING btree (owner_identity, resolved_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_task_review_decisions_task_plan
    ON public.task_review_decisions USING btree (task_plan_id);
CREATE INDEX IF NOT EXISTS idx_task_review_decisions_decision
    ON public.task_review_decisions USING btree (decision);
CREATE INDEX IF NOT EXISTS idx_task_review_decisions_approval_source_id
    ON public.task_review_decisions USING btree (approval_source_id);
CREATE INDEX IF NOT EXISTS idx_task_review_decisions_request_digest
    ON public.task_review_decisions USING btree (request_digest);
CREATE UNIQUE INDEX IF NOT EXISTS idx_task_review_decisions_item_revision
    ON public.task_review_decisions USING btree (review_item_id, review_revision);

CREATE OR REPLACE FUNCTION public.hai_reject_task_audit_mutation()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE EXCEPTION 'task audit rows are immutable'
        USING ERRCODE = '55000';
END;
$$;

CREATE OR REPLACE FUNCTION public.hai_reject_task_audit_truncate()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE EXCEPTION 'task audit history cannot be truncated'
        USING ERRCODE = '55000';
END;
$$;

CREATE OR REPLACE FUNCTION public.hai_enforce_task_review_item_provenance()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    IF
        NEW.id IS DISTINCT FROM OLD.id OR
        NEW.owner_identity IS DISTINCT FROM OLD.owner_identity OR
        NEW.original_task_plan_id IS DISTINCT FROM OLD.original_task_plan_id OR
        NEW.request_digest IS DISTINCT FROM OLD.request_digest OR
        NEW.request_json IS DISTINCT FROM OLD.request_json OR
        NEW.created_at IS DISTINCT FROM OLD.created_at
    THEN
        RAISE EXCEPTION 'task review request provenance is immutable'
            USING ERRCODE = '55000';
    END IF;
    RETURN NEW;
END;
$$;

CREATE OR REPLACE FUNCTION public.hai_enforce_task_review_item_insert()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    IF
        NEW.status NOT IN ('open', 'needs_review') OR
        NEW.review_revision <> 1 OR
        NEW.current_task_plan_id IS DISTINCT FROM NEW.original_task_plan_id OR
        NEW.resolved_at IS NOT NULL OR
        NEW.updated_at IS DISTINCT FROM NEW.created_at
    THEN
        RAISE EXCEPTION 'new task review items must begin as an unresolved first review cycle'
            USING ERRCODE = '23514';
    END IF;
    RETURN NEW;
END;
$$;

CREATE OR REPLACE FUNCTION public.hai_enforce_task_review_item_transition()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    IF NEW.updated_at < OLD.updated_at THEN
        RAISE EXCEPTION 'task review state timestamp cannot move backwards'
            USING ERRCODE = '23514';
    END IF;
    IF NEW.priority IS DISTINCT FROM OLD.priority THEN
        RAISE EXCEPTION 'task review priority is immutable after intake'
            USING ERRCODE = '55000';
    END IF;

    IF
        OLD.status IN ('open', 'needs_review') AND
        NEW.status IN ('approved', 'rejected')
    THEN
        IF
            NEW.current_task_plan_id IS DISTINCT FROM OLD.current_task_plan_id OR
            NEW.reason IS DISTINCT FROM OLD.reason OR
            NEW.review_revision IS DISTINCT FROM OLD.review_revision OR
            NEW.resolved_at IS NULL OR
            NEW.resolved_at IS DISTINCT FROM NEW.updated_at
        THEN
            RAISE EXCEPTION 'invalid task review resolution transition'
                USING ERRCODE = '23514';
        END IF;
    ELSIF OLD.status = 'approved' AND NEW.status = 'needs_review' THEN
        IF
            NEW.review_revision <> OLD.review_revision + 1 OR
            NEW.resolved_at IS NOT NULL
        THEN
            RAISE EXCEPTION 'invalid task review retry transition'
                USING ERRCODE = '23514';
        END IF;
    ELSIF OLD.status = 'approved' AND NEW.status = 'completed' THEN
        IF
            NEW.review_revision IS DISTINCT FROM OLD.review_revision OR
            NEW.resolved_at IS NULL OR
            NEW.resolved_at IS DISTINCT FROM NEW.updated_at
        THEN
            RAISE EXCEPTION 'invalid task review completion transition'
                USING ERRCODE = '23514';
        END IF;
    ELSE
        RAISE EXCEPTION 'invalid task review state transition from % to %',
            OLD.status,
            NEW.status
            USING ERRCODE = '23514';
    END IF;
    RETURN NEW;
END;
$$;

CREATE OR REPLACE FUNCTION public.hai_require_task_review_resolution_state()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM public.task_review_items item
        WHERE item.id = NEW.review_item_id
          AND item.owner_identity = NEW.owner_identity
          AND item.review_revision = NEW.review_revision
          AND item.request_digest = NEW.request_digest
          AND item.current_task_plan_id = NEW.task_plan_id
          AND item.status = NEW.decision
          AND item.resolved_at = NEW.resolved_at
    ) THEN
        RAISE EXCEPTION 'task review decision was not committed with its resolved queue state'
            USING ERRCODE = '23514';
    END IF;
    RETURN NULL;
END;
$$;

CREATE OR REPLACE FUNCTION public.hai_enforce_task_review_decision_binding()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM public.task_review_items item
        WHERE item.id = NEW.review_item_id
          AND item.owner_identity = NEW.owner_identity
          AND item.review_revision = NEW.review_revision
          AND item.request_digest = NEW.request_digest
          AND item.current_task_plan_id = NEW.task_plan_id
          AND item.status IN ('open', 'needs_review')
          AND NEW.resolved_by = item.owner_identity
          AND NEW.resolved_at >= item.updated_at
    ) THEN
        RAISE EXCEPTION 'task review decision does not match stored owner and request provenance'
            USING ERRCODE = '23514';
    END IF;
    RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS trg_task_completion_plan_logs_immutable
    ON public.task_completion_plan_logs;
CREATE TRIGGER trg_task_completion_plan_logs_immutable
    BEFORE UPDATE OR DELETE ON public.task_completion_plan_logs
    FOR EACH ROW
    EXECUTE FUNCTION public.hai_reject_task_audit_mutation();

DROP TRIGGER IF EXISTS trg_task_completion_plan_logs_no_truncate
    ON public.task_completion_plan_logs;
CREATE TRIGGER trg_task_completion_plan_logs_no_truncate
    BEFORE TRUNCATE ON public.task_completion_plan_logs
    FOR EACH STATEMENT
    EXECUTE FUNCTION public.hai_reject_task_audit_truncate();

DROP TRIGGER IF EXISTS trg_task_review_items_provenance
    ON public.task_review_items;
CREATE TRIGGER trg_task_review_items_provenance
    BEFORE UPDATE ON public.task_review_items
    FOR EACH ROW
    EXECUTE FUNCTION public.hai_enforce_task_review_item_provenance();

DROP TRIGGER IF EXISTS trg_task_review_items_insert
    ON public.task_review_items;
CREATE TRIGGER trg_task_review_items_insert
    BEFORE INSERT ON public.task_review_items
    FOR EACH ROW
    EXECUTE FUNCTION public.hai_enforce_task_review_item_insert();

DROP TRIGGER IF EXISTS trg_task_review_items_transition
    ON public.task_review_items;
CREATE TRIGGER trg_task_review_items_transition
    BEFORE UPDATE ON public.task_review_items
    FOR EACH ROW
    EXECUTE FUNCTION public.hai_enforce_task_review_item_transition();

DROP TRIGGER IF EXISTS trg_task_review_items_no_delete
    ON public.task_review_items;
CREATE TRIGGER trg_task_review_items_no_delete
    BEFORE DELETE ON public.task_review_items
    FOR EACH ROW
    EXECUTE FUNCTION public.hai_reject_task_audit_mutation();

DROP TRIGGER IF EXISTS trg_task_review_items_no_truncate
    ON public.task_review_items;
CREATE TRIGGER trg_task_review_items_no_truncate
    BEFORE TRUNCATE ON public.task_review_items
    FOR EACH STATEMENT
    EXECUTE FUNCTION public.hai_reject_task_audit_truncate();

DROP TRIGGER IF EXISTS trg_task_review_decisions_binding
    ON public.task_review_decisions;
CREATE TRIGGER trg_task_review_decisions_binding
    BEFORE INSERT ON public.task_review_decisions
    FOR EACH ROW
    EXECUTE FUNCTION public.hai_enforce_task_review_decision_binding();

DROP TRIGGER IF EXISTS trg_task_review_decisions_resolution_state
    ON public.task_review_decisions;
CREATE CONSTRAINT TRIGGER trg_task_review_decisions_resolution_state
    AFTER INSERT ON public.task_review_decisions
    DEFERRABLE INITIALLY DEFERRED
    FOR EACH ROW
    EXECUTE FUNCTION public.hai_require_task_review_resolution_state();

DROP TRIGGER IF EXISTS trg_task_review_decisions_immutable
    ON public.task_review_decisions;
CREATE TRIGGER trg_task_review_decisions_immutable
    BEFORE UPDATE OR DELETE ON public.task_review_decisions
    FOR EACH ROW
    EXECUTE FUNCTION public.hai_reject_task_audit_mutation();

DROP TRIGGER IF EXISTS trg_task_review_decisions_no_truncate
    ON public.task_review_decisions;
CREATE TRIGGER trg_task_review_decisions_no_truncate
    BEFORE TRUNCATE ON public.task_review_decisions
    FOR EACH STATEMENT
    EXECUTE FUNCTION public.hai_reject_task_audit_truncate();
