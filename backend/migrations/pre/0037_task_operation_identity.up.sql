CREATE TABLE IF NOT EXISTS public.task_operations (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    owner_identity character varying(255) NOT NULL,
    idempotency_key character varying(120) NOT NULL,
    request_digest character(64) NOT NULL,
    mode character varying(16) NOT NULL,
    status character varying(32) NOT NULL,
    task_plan_id character varying(160) DEFAULT '' NOT NULL,
    lease_owner character varying(120) DEFAULT '' NOT NULL,
    lease_generation bigint DEFAULT 0 NOT NULL,
    leased_at timestamp with time zone,
    last_error character varying(1024) DEFAULT '' NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    completed_at timestamp with time zone,
    CONSTRAINT task_operations_pkey PRIMARY KEY (id),
    CONSTRAINT uq_task_operations_owner_key UNIQUE (owner_identity, idempotency_key),
    CONSTRAINT chk_task_operations_owner CHECK (btrim(owner_identity) <> ''),
    CONSTRAINT chk_task_operations_key CHECK (
        char_length(idempotency_key) BETWEEN 1 AND 120
        AND idempotency_key ~ '^[A-Za-z0-9._:-]+$'
    ),
    CONSTRAINT chk_task_operations_digest CHECK (request_digest ~ '^[0-9a-f]{64}$'),
    CONSTRAINT chk_task_operations_mode CHECK (mode IN ('plan', 'run')),
    CONSTRAINT chk_task_operations_status CHECK (status IN ('running', 'completed', 'needs_review')),
    CONSTRAINT chk_task_operations_generation CHECK (lease_generation >= 1),
    CONSTRAINT chk_task_operations_lifecycle CHECK (
        (status = 'running'
            AND btrim(lease_owner) <> ''
            AND leased_at IS NOT NULL
            AND task_plan_id = ''
            AND completed_at IS NULL
            AND last_error = '')
        OR
        (status = 'completed'
            AND lease_owner = ''
            AND leased_at IS NULL
            AND btrim(task_plan_id) <> ''
            AND completed_at IS NOT NULL
            AND last_error = '')
        OR
        (status = 'needs_review'
            AND lease_owner = ''
            AND leased_at IS NULL
            AND completed_at IS NULL
            AND btrim(last_error) <> '')
    )
);

CREATE INDEX IF NOT EXISTS idx_task_operations_owner_updated
    ON public.task_operations USING btree (owner_identity, updated_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_task_operations_status_lease
    ON public.task_operations USING btree (status, leased_at);
CREATE INDEX IF NOT EXISTS idx_task_operations_task_plan
    ON public.task_operations USING btree (task_plan_id)
    WHERE task_plan_id <> '';

CREATE OR REPLACE FUNCTION public.task_operations_guard_identity()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    IF NEW.id IS DISTINCT FROM OLD.id
       OR NEW.owner_identity IS DISTINCT FROM OLD.owner_identity
       OR NEW.idempotency_key IS DISTINCT FROM OLD.idempotency_key
       OR NEW.request_digest IS DISTINCT FROM OLD.request_digest
       OR NEW.mode IS DISTINCT FROM OLD.mode
       OR NEW.created_at IS DISTINCT FROM OLD.created_at
       OR NEW.lease_generation IS DISTINCT FROM OLD.lease_generation THEN
        RAISE EXCEPTION 'task operation identity is immutable';
    END IF;
    IF OLD.status <> 'running' THEN
        RAISE EXCEPTION 'terminal task operation state is immutable';
    END IF;
    IF NEW.status NOT IN ('running', 'completed', 'needs_review') THEN
        RAISE EXCEPTION 'invalid task operation transition';
    END IF;
    RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS task_operations_guard_identity ON public.task_operations;
CREATE TRIGGER task_operations_guard_identity
BEFORE UPDATE ON public.task_operations
FOR EACH ROW EXECUTE FUNCTION public.task_operations_guard_identity();

CREATE OR REPLACE FUNCTION public.task_operations_reject_removal()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE EXCEPTION 'task operation records are durable audit state';
END;
$$;

DROP TRIGGER IF EXISTS task_operations_reject_delete ON public.task_operations;
CREATE TRIGGER task_operations_reject_delete
BEFORE DELETE ON public.task_operations
FOR EACH ROW EXECUTE FUNCTION public.task_operations_reject_removal();

DROP TRIGGER IF EXISTS task_operations_reject_truncate ON public.task_operations;
CREATE TRIGGER task_operations_reject_truncate
BEFORE TRUNCATE ON public.task_operations
FOR EACH STATEMENT EXECUTE FUNCTION public.task_operations_reject_removal();
