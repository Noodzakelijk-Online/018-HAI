CREATE EXTENSION IF NOT EXISTS pgcrypto;

ALTER TABLE public.outcome_monitor_runs
    ADD CONSTRAINT uq_outcome_monitor_run_owner_workspace_id
    UNIQUE (owner_identity, workspace_key, run_id);
ALTER TABLE public.outcome_observation_records
    ADD CONSTRAINT uq_outcome_observation_owner_workspace_id
    UNIQUE (owner_identity, workspace_key, observation_id);

CREATE TABLE public.outcome_monitor_composition_deliveries (
    delivery_id uuid PRIMARY KEY,
    owner_identity character varying(255) NOT NULL,
    workspace_key character varying(255) NOT NULL,
    target_id uuid NOT NULL,
    run_id uuid NOT NULL,
    run_digest character(64) NOT NULL,
    observation_id uuid,
    observation_digest character(64),
    status character varying(20) NOT NULL DEFAULT 'pending',
    revision bigint NOT NULL DEFAULT 1,
    lease_generation bigint NOT NULL DEFAULT 0,
    attempt_count integer NOT NULL DEFAULT 0,
    max_attempts integer NOT NULL DEFAULT 5,
    base_backoff_seconds integer NOT NULL DEFAULT 30,
    max_backoff_seconds integer NOT NULL DEFAULT 3600,
    next_attempt_at timestamp with time zone,
    lease_id uuid,
    lease_owner character varying(255),
    lease_until timestamp with time zone,
    last_attempt_at timestamp with time zone,
    last_failure_code character varying(80),
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL,
    completed_at timestamp with time zone,
    binding_digest character(64) NOT NULL,
    authority character varying(80) NOT NULL DEFAULT 'advisory_monitor_only',
    can_execute boolean NOT NULL DEFAULT false,
    delivery_authorized boolean NOT NULL DEFAULT false,
    execution_authorized boolean NOT NULL DEFAULT false,
    notification_authorized boolean NOT NULL DEFAULT false,
    external_effects_authorized boolean NOT NULL DEFAULT false,
    learning_mutation_authorized boolean NOT NULL DEFAULT false,
    CONSTRAINT uq_outcome_monitor_composition_delivery_scope_id
        UNIQUE (owner_identity, workspace_key, delivery_id),
    CONSTRAINT uq_outcome_monitor_composition_delivery_run
        UNIQUE (owner_identity, workspace_key, run_id),
    CONSTRAINT fk_outcome_monitor_composition_delivery_target
        FOREIGN KEY (owner_identity, workspace_key, target_id)
        REFERENCES public.outcome_monitor_targets (owner_identity, workspace_key, target_id)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT fk_outcome_monitor_composition_delivery_run
        FOREIGN KEY (owner_identity, workspace_key, run_id)
        REFERENCES public.outcome_monitor_runs (owner_identity, workspace_key, run_id)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT fk_outcome_monitor_composition_delivery_observation
        FOREIGN KEY (owner_identity, workspace_key, observation_id)
        REFERENCES public.outcome_observation_records
            (owner_identity, workspace_key, observation_id)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT chk_outcome_monitor_composition_delivery_identity CHECK (
        char_length(btrim(owner_identity)) BETWEEN 1 AND 255
        AND owner_identity = btrim(owner_identity)
        AND char_length(btrim(workspace_key)) BETWEEN 1 AND 255
        AND workspace_key = btrim(workspace_key)
        AND run_digest ~ '^[0-9a-f]{64}$'
        AND binding_digest ~ '^[0-9a-f]{64}$'
        AND (
            (observation_id IS NULL AND observation_digest IS NULL)
            OR (observation_id IS NOT NULL AND observation_digest ~ '^[0-9a-f]{64}$')
        )
    ),
    CONSTRAINT chk_outcome_monitor_composition_delivery_retry_policy CHECK (
        revision >= 1
        AND lease_generation >= 0
        AND lease_generation <= revision
        AND attempt_count BETWEEN 0 AND max_attempts
        AND max_attempts BETWEEN 1 AND 20
        AND base_backoff_seconds BETWEEN 1 AND 86400
        AND max_backoff_seconds BETWEEN base_backoff_seconds AND 604800
    ),
    CONSTRAINT chk_outcome_monitor_composition_delivery_lease CHECK (
        (lease_id IS NULL AND lease_owner IS NULL AND lease_until IS NULL)
        OR (
            status = 'pending'
            AND lease_id IS NOT NULL
            AND lease_owner IS NOT NULL
            AND lease_owner ~ '^[A-Za-z0-9][A-Za-z0-9._:@/-]{0,254}$'
            AND lease_until IS NOT NULL
            AND lease_generation >= 1
            AND lease_until > updated_at
            AND lease_until <= updated_at + interval '1 hour'
        )
    ),
    CONSTRAINT chk_outcome_monitor_composition_delivery_state CHECK (
        status IN ('pending', 'succeeded', 'dead_lettered')
        AND created_at <= updated_at
        AND (
            (
                status = 'pending'
                AND completed_at IS NULL
                AND next_attempt_at IS NOT NULL
                AND attempt_count < max_attempts
            )
            OR (
                status IN ('succeeded', 'dead_lettered')
                AND completed_at IS NOT NULL
                AND completed_at <= updated_at
                AND lease_id IS NULL
                AND lease_owner IS NULL
                AND lease_until IS NULL
            )
        )
    ),
    CONSTRAINT chk_outcome_monitor_composition_delivery_attempt_projection CHECK (
        (
            attempt_count = 0
            AND last_attempt_at IS NULL
            AND last_failure_code IS NULL
        )
        OR (
            attempt_count > 0
            AND last_attempt_at IS NOT NULL
            AND last_attempt_at <= updated_at
            AND (
                last_failure_code IS NULL
                OR last_failure_code ~ '^[a-z][a-z0-9_]{0,79}$'
            )
        )
    ),
    CONSTRAINT chk_outcome_monitor_composition_delivery_advisory_only CHECK (
        authority = 'advisory_monitor_only'
        AND can_execute = false
        AND delivery_authorized = false
        AND execution_authorized = false
        AND notification_authorized = false
        AND external_effects_authorized = false
        AND learning_mutation_authorized = false
    )
);

CREATE INDEX idx_outcome_monitor_composition_deliveries_due
    ON public.outcome_monitor_composition_deliveries
    (next_attempt_at, lease_until, owner_identity, workspace_key, delivery_id)
    WHERE status = 'pending';
CREATE INDEX idx_outcome_monitor_composition_deliveries_scope_history
    ON public.outcome_monitor_composition_deliveries
    (owner_identity, workspace_key, status, updated_at DESC, delivery_id DESC);

CREATE TABLE public.outcome_monitor_composition_attempts (
    attempt_id uuid PRIMARY KEY,
    delivery_id uuid NOT NULL,
    owner_identity character varying(255) NOT NULL,
    workspace_key character varying(255) NOT NULL,
    target_id uuid NOT NULL,
    run_id uuid NOT NULL,
    run_digest character(64) NOT NULL,
    attempt_number integer NOT NULL,
    queue_revision bigint NOT NULL DEFAULT 0,
    lease_generation bigint NOT NULL,
    claim_id uuid NOT NULL,
    worker_id character varying(255) NOT NULL,
    status character varying(20) NOT NULL,
    failure_code character varying(80),
    started_at timestamp with time zone NOT NULL,
    finished_at timestamp with time zone NOT NULL,
    request_digest character(64) NOT NULL,
    record_digest character(64) NOT NULL,
    authority character varying(80) NOT NULL DEFAULT 'advisory_monitor_only',
    can_execute boolean NOT NULL DEFAULT false,
    delivery_authorized boolean NOT NULL DEFAULT false,
    execution_authorized boolean NOT NULL DEFAULT false,
    notification_authorized boolean NOT NULL DEFAULT false,
    external_effects_authorized boolean NOT NULL DEFAULT false,
    learning_mutation_authorized boolean NOT NULL DEFAULT false,
    CONSTRAINT uq_outcome_monitor_composition_attempt_number
        UNIQUE (owner_identity, workspace_key, delivery_id, attempt_number),
    CONSTRAINT uq_outcome_monitor_composition_attempt_digest
        UNIQUE (owner_identity, workspace_key, record_digest),
    CONSTRAINT fk_outcome_monitor_composition_attempt_delivery
        FOREIGN KEY (owner_identity, workspace_key, delivery_id)
        REFERENCES public.outcome_monitor_composition_deliveries
            (owner_identity, workspace_key, delivery_id)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT fk_outcome_monitor_composition_attempt_target
        FOREIGN KEY (owner_identity, workspace_key, target_id)
        REFERENCES public.outcome_monitor_targets (owner_identity, workspace_key, target_id)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT fk_outcome_monitor_composition_attempt_run
        FOREIGN KEY (owner_identity, workspace_key, run_id)
        REFERENCES public.outcome_monitor_runs (owner_identity, workspace_key, run_id)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT chk_outcome_monitor_composition_attempt_identity CHECK (
        char_length(btrim(owner_identity)) BETWEEN 1 AND 255
        AND owner_identity = btrim(owner_identity)
        AND char_length(btrim(workspace_key)) BETWEEN 1 AND 255
        AND workspace_key = btrim(workspace_key)
        AND attempt_number >= 1
        AND queue_revision >= 1
        AND lease_generation >= 1
        AND worker_id ~ '^[A-Za-z0-9][A-Za-z0-9._:@/-]{0,254}$'
        AND run_digest ~ '^[0-9a-f]{64}$'
    ),
    CONSTRAINT chk_outcome_monitor_composition_attempt_result CHECK (
        status IN ('succeeded', 'failed')
        AND (
            (
                status = 'succeeded'
                AND failure_code IS NULL
            )
            OR (
                status = 'failed'
                AND failure_code ~ '^[a-z][a-z0-9_]{0,79}$'
            )
        )
    ),
    CONSTRAINT chk_outcome_monitor_composition_attempt_time CHECK (
        started_at <= finished_at
    ),
    CONSTRAINT chk_outcome_monitor_composition_attempt_digests CHECK (
        request_digest ~ '^[0-9a-f]{64}$'
        AND record_digest ~ '^[0-9a-f]{64}$'
    ),
    CONSTRAINT chk_outcome_monitor_composition_attempt_advisory_only CHECK (
        authority = 'advisory_monitor_only'
        AND can_execute = false
        AND delivery_authorized = false
        AND execution_authorized = false
        AND notification_authorized = false
        AND external_effects_authorized = false
        AND learning_mutation_authorized = false
    )
);

CREATE INDEX idx_outcome_monitor_composition_attempts_history
    ON public.outcome_monitor_composition_attempts
    (owner_identity, workspace_key, delivery_id, attempt_number DESC, finished_at DESC);

CREATE OR REPLACE FUNCTION public.hai_reject_outcome_monitor_composition_attempt_mutation()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE EXCEPTION 'outcome monitor composition attempts are append-only'
        USING ERRCODE = '55000';
END;
$$;

CREATE OR REPLACE FUNCTION public.hai_validate_outcome_monitor_composition_delivery_insert()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    run_record public.outcome_monitor_runs%ROWTYPE;
    observation_record public.outcome_observation_records%ROWTYPE;
BEGIN
    SELECT * INTO run_record
      FROM public.outcome_monitor_runs
     WHERE owner_identity = NEW.owner_identity
       AND workspace_key = NEW.workspace_key
       AND run_id = NEW.run_id;
    IF NOT FOUND
       OR run_record.status <> 'succeeded'
       OR run_record.target_id <> NEW.target_id
       OR run_record.record_digest <> NEW.run_digest
       OR NEW.delivery_id <> NEW.run_id THEN
        RAISE EXCEPTION 'composition delivery requires the exact owner-scoped successful monitor run'
            USING ERRCODE = 'foreign_key_violation';
    END IF;
    IF NEW.observation_id IS NOT NULL THEN
        SELECT * INTO observation_record
          FROM public.outcome_observation_records
         WHERE owner_identity = NEW.owner_identity
           AND workspace_key = NEW.workspace_key
           AND observation_id = NEW.observation_id;
        IF NOT FOUND OR observation_record.record_digest <> NEW.observation_digest THEN
            RAISE EXCEPTION 'composition delivery observation digest does not match owner-scoped evidence'
                USING ERRCODE = 'foreign_key_violation';
        END IF;
    END IF;
    IF NEW.status <> 'pending'
       OR NEW.revision <> 1
       OR NEW.lease_generation <> 0
       OR NEW.lease_id IS NOT NULL
       OR NEW.lease_owner IS NOT NULL
       OR NEW.lease_until IS NOT NULL
       OR NEW.attempt_count <> 0
       OR NEW.last_attempt_at IS NOT NULL
       OR NEW.last_failure_code IS NOT NULL
       OR NEW.completed_at IS NOT NULL
       OR NEW.created_at <> NEW.updated_at
       OR NEW.next_attempt_at < run_record.completed_at
       OR NEW.binding_digest !~ '^[0-9a-f]{64}$' THEN
        RAISE EXCEPTION 'new composition delivery must start as an unclaimed pending revision one'
            USING ERRCODE = 'integrity_constraint_violation';
    END IF;
    RETURN NEW;
END;
$$;

CREATE OR REPLACE FUNCTION public.hai_validate_outcome_monitor_composition_attempt_insert()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    delivery_record public.outcome_monitor_composition_deliveries%ROWTYPE;
BEGIN
    SELECT * INTO delivery_record
      FROM public.outcome_monitor_composition_deliveries
     WHERE owner_identity = NEW.owner_identity
       AND workspace_key = NEW.workspace_key
       AND delivery_id = NEW.delivery_id
     FOR UPDATE;
    IF NEW.queue_revision = 0 THEN
        NEW.queue_revision := delivery_record.revision;
    END IF;
    IF NOT FOUND
       OR delivery_record.status <> 'pending'
       OR delivery_record.revision <> NEW.queue_revision
       OR delivery_record.lease_generation <> NEW.lease_generation
       OR delivery_record.lease_id IS NULL
       OR delivery_record.lease_id <> NEW.claim_id
       OR delivery_record.lease_owner IS DISTINCT FROM NEW.worker_id
       OR delivery_record.lease_until IS NULL
       OR delivery_record.lease_until < NEW.finished_at
       OR delivery_record.target_id <> NEW.target_id
       OR delivery_record.run_id <> NEW.run_id
       OR delivery_record.run_digest <> NEW.run_digest
       OR NEW.attempt_number <> delivery_record.attempt_count + 1
       OR NEW.attempt_number > delivery_record.max_attempts THEN
        RAISE EXCEPTION 'composition attempt does not match the current owner-scoped fenced lease'
            USING ERRCODE = 'serialization_failure';
    END IF;
    RETURN NEW;
END;
$$;

CREATE OR REPLACE FUNCTION public.hai_validate_outcome_monitor_composition_delivery_write()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    attempt_record public.outcome_monitor_composition_attempts%ROWTYPE;
BEGIN
    IF TG_OP = 'DELETE' THEN
        RAISE EXCEPTION 'outcome monitor composition deliveries cannot be deleted'
            USING ERRCODE = '55000';
    END IF;
    IF NEW.delivery_id <> OLD.delivery_id
       OR NEW.owner_identity <> OLD.owner_identity
       OR NEW.workspace_key <> OLD.workspace_key
       OR NEW.target_id <> OLD.target_id
       OR NEW.run_id <> OLD.run_id
       OR NEW.run_digest <> OLD.run_digest
       OR NEW.binding_digest <> OLD.binding_digest
       OR NEW.observation_id IS DISTINCT FROM OLD.observation_id
       OR NEW.observation_digest IS DISTINCT FROM OLD.observation_digest
       OR NEW.max_attempts <> OLD.max_attempts
       OR NEW.base_backoff_seconds <> OLD.base_backoff_seconds
       OR NEW.max_backoff_seconds <> OLD.max_backoff_seconds
       OR NEW.created_at <> OLD.created_at
       OR NEW.authority <> OLD.authority
       OR NEW.can_execute <> OLD.can_execute
       OR NEW.delivery_authorized <> OLD.delivery_authorized
       OR NEW.execution_authorized <> OLD.execution_authorized
       OR NEW.notification_authorized <> OLD.notification_authorized
       OR NEW.external_effects_authorized <> OLD.external_effects_authorized
       OR NEW.learning_mutation_authorized <> OLD.learning_mutation_authorized THEN
        RAISE EXCEPTION 'composition delivery identity, evidence, policy, and capabilities are immutable'
            USING ERRCODE = 'integrity_constraint_violation';
    END IF;
    IF OLD.status <> 'pending' THEN
        RAISE EXCEPTION 'terminal composition delivery cannot transition'
            USING ERRCODE = '55000';
    END IF;
    IF NEW.revision <> OLD.revision + 1 OR NEW.updated_at <= OLD.updated_at THEN
        RAISE EXCEPTION 'composition delivery update must advance revision and time exactly once'
            USING ERRCODE = 'serialization_failure';
    END IF;

    IF OLD.lease_id IS NULL AND NEW.lease_id IS NOT NULL THEN
        IF NEW.status <> 'pending'
           OR NEW.lease_generation <> OLD.lease_generation + 1
           OR NEW.attempt_count <> OLD.attempt_count
           OR NEW.last_attempt_at IS DISTINCT FROM OLD.last_attempt_at
           OR NEW.last_failure_code IS DISTINCT FROM OLD.last_failure_code
           OR NEW.next_attempt_at IS DISTINCT FROM OLD.next_attempt_at
           OR OLD.next_attempt_at > NEW.updated_at THEN
            RAISE EXCEPTION 'composition delivery claim must fence one due pending revision'
                USING ERRCODE = 'serialization_failure';
        END IF;
        RETURN NEW;
    END IF;

    IF OLD.lease_id IS NULL OR NEW.lease_id IS NOT NULL THEN
        RAISE EXCEPTION 'composition delivery update must be a fenced claim or receipt-backed settlement'
            USING ERRCODE = 'serialization_failure';
    END IF;
    IF OLD.lease_until <= NEW.updated_at
       AND NEW.status = 'pending'
       AND NEW.lease_generation = OLD.lease_generation
       AND NEW.attempt_count = OLD.attempt_count
       AND NEW.next_attempt_at IS NOT DISTINCT FROM OLD.next_attempt_at
       AND NEW.last_attempt_at IS NOT DISTINCT FROM OLD.last_attempt_at
       AND NEW.last_failure_code IS NOT DISTINCT FROM OLD.last_failure_code
       AND NEW.completed_at IS NOT DISTINCT FROM OLD.completed_at THEN
        RETURN NEW;
    END IF;
    SELECT * INTO attempt_record
      FROM public.outcome_monitor_composition_attempts
     WHERE owner_identity = OLD.owner_identity
       AND workspace_key = OLD.workspace_key
       AND delivery_id = OLD.delivery_id
       AND attempt_number = OLD.attempt_count + 1;
    IF NOT FOUND
       OR attempt_record.queue_revision <> OLD.revision
       OR attempt_record.lease_generation <> OLD.lease_generation
       OR attempt_record.claim_id <> OLD.lease_id
       OR attempt_record.worker_id IS DISTINCT FROM OLD.lease_owner
       OR NEW.attempt_count <> attempt_record.attempt_number
       OR NEW.last_attempt_at <> attempt_record.finished_at
       OR NEW.updated_at < attempt_record.finished_at
       OR NEW.lease_generation <> OLD.lease_generation THEN
        RAISE EXCEPTION 'composition delivery settlement does not match immutable attempt receipt'
            USING ERRCODE = 'integrity_constraint_violation';
    END IF;

    IF attempt_record.status = 'succeeded' THEN
        IF NEW.status <> 'succeeded'
           OR NEW.completed_at <> attempt_record.finished_at
           OR NEW.last_failure_code IS NOT NULL THEN
            RAISE EXCEPTION 'successful composition receipt requires an exact terminal projection'
                USING ERRCODE = 'integrity_constraint_violation';
        END IF;
    ELSIF NEW.status = 'dead_lettered' THEN
        IF NEW.completed_at <> attempt_record.finished_at
           OR NEW.last_failure_code IS DISTINCT FROM attempt_record.failure_code THEN
            RAISE EXCEPTION 'failed composition receipt requires an exact dead-letter projection'
                USING ERRCODE = 'integrity_constraint_violation';
        END IF;
    ELSE
        IF NEW.status <> 'pending'
           OR NEW.completed_at IS NOT NULL
           OR NEW.attempt_count >= NEW.max_attempts
           OR NEW.next_attempt_at < NEW.updated_at + make_interval(secs => OLD.base_backoff_seconds)
           OR NEW.next_attempt_at > NEW.updated_at + make_interval(secs => OLD.max_backoff_seconds)
           OR NEW.last_failure_code IS DISTINCT FROM attempt_record.failure_code THEN
            RAISE EXCEPTION 'failed composition receipt requires bounded retry or dead-letter projection'
                USING ERRCODE = 'integrity_constraint_violation';
        END IF;
    END IF;
    RETURN NEW;
END;
$$;

CREATE OR REPLACE FUNCTION public.hai_enqueue_outcome_monitor_composition_delivery()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    matched_observation_id uuid;
    matched_observation_digest character(64);
    derived_binding_digest character(64);
BEGIN
    IF NEW.status = 'succeeded' THEN
        SELECT observation.observation_id, observation.record_digest
          INTO matched_observation_id, matched_observation_digest
          FROM public.outcome_observation_records AS observation
          JOIN public.outcome_monitor_targets AS target
            ON target.owner_identity = NEW.owner_identity
           AND target.workspace_key = NEW.workspace_key
           AND target.target_id = NEW.target_id
         WHERE observation.owner_identity = NEW.owner_identity
           AND observation.workspace_key = NEW.workspace_key
           AND observation.outcome_key = target.outcome_key
           AND observation.indicator_key = target.indicator_key
           AND observation.source_kind = target.source_kind
         ORDER BY
            (observation.idempotency_key = NEW.idempotency_key) DESC,
            (observation.recorded_at BETWEEN NEW.started_at AND NEW.completed_at) DESC,
            observation.recorded_at DESC,
            observation.observation_id DESC
         LIMIT 1;
        derived_binding_digest := encode(digest(concat_ws('|',
            'composition_binding_v1', NEW.owner_identity, NEW.workspace_key,
            NEW.run_id::text, NEW.target_id::text, NEW.record_digest,
            COALESCE(matched_observation_id::text, ''),
            COALESCE(matched_observation_digest, '')
        ), 'sha256'), 'hex');
        INSERT INTO public.outcome_monitor_composition_deliveries (
            delivery_id, owner_identity, workspace_key, target_id, run_id,
            run_digest, observation_id, observation_digest,
            status, revision, lease_generation, attempt_count, max_attempts,
            base_backoff_seconds, max_backoff_seconds, next_attempt_at,
            created_at, updated_at, binding_digest
        ) VALUES (
            NEW.run_id, NEW.owner_identity, NEW.workspace_key, NEW.target_id, NEW.run_id,
            NEW.record_digest, matched_observation_id, matched_observation_digest,
            'pending', 1, 0, 0, 5, 30, 3600, NEW.completed_at,
            NEW.completed_at, NEW.completed_at, derived_binding_digest
        ) ON CONFLICT (delivery_id) DO NOTHING;
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_outcome_monitor_composition_delivery_validate_insert
    BEFORE INSERT ON public.outcome_monitor_composition_deliveries
    FOR EACH ROW EXECUTE FUNCTION public.hai_validate_outcome_monitor_composition_delivery_insert();
CREATE TRIGGER trg_outcome_monitor_composition_delivery_validate_update
    BEFORE UPDATE ON public.outcome_monitor_composition_deliveries
    FOR EACH ROW EXECUTE FUNCTION public.hai_validate_outcome_monitor_composition_delivery_write();
CREATE TRIGGER trg_outcome_monitor_composition_delivery_reject_delete
    BEFORE DELETE ON public.outcome_monitor_composition_deliveries
    FOR EACH ROW EXECUTE FUNCTION public.hai_validate_outcome_monitor_composition_delivery_write();
CREATE TRIGGER trg_outcome_monitor_composition_delivery_no_truncate
    BEFORE TRUNCATE ON public.outcome_monitor_composition_deliveries
    FOR EACH STATEMENT EXECUTE FUNCTION public.hai_reject_outcome_monitor_composition_attempt_mutation();

CREATE TRIGGER trg_outcome_monitor_composition_attempt_validate_insert
    BEFORE INSERT ON public.outcome_monitor_composition_attempts
    FOR EACH ROW EXECUTE FUNCTION public.hai_validate_outcome_monitor_composition_attempt_insert();
CREATE TRIGGER trg_outcome_monitor_composition_attempt_immutable
    BEFORE UPDATE OR DELETE ON public.outcome_monitor_composition_attempts
    FOR EACH ROW EXECUTE FUNCTION public.hai_reject_outcome_monitor_composition_attempt_mutation();
CREATE TRIGGER trg_outcome_monitor_composition_attempt_no_truncate
    BEFORE TRUNCATE ON public.outcome_monitor_composition_attempts
    FOR EACH STATEMENT EXECUTE FUNCTION public.hai_reject_outcome_monitor_composition_attempt_mutation();

INSERT INTO public.outcome_monitor_composition_deliveries (
    delivery_id, owner_identity, workspace_key, target_id, run_id,
    run_digest, observation_id, observation_digest,
    status, revision, lease_generation, attempt_count, max_attempts,
    base_backoff_seconds, max_backoff_seconds, next_attempt_at,
    created_at, updated_at, binding_digest
)
SELECT
    run.run_id, run.owner_identity, run.workspace_key, run.target_id, run.run_id,
    run.record_digest, observation.observation_id, observation.record_digest,
    'pending', 1, 0, 0, 5, 30, 3600, run.completed_at,
    run.completed_at, run.completed_at,
    encode(digest(concat_ws('|',
        'composition_binding_v1', run.owner_identity, run.workspace_key,
        run.run_id::text, run.target_id::text, run.record_digest,
        COALESCE(observation.observation_id::text, ''),
        COALESCE(observation.record_digest, '')
    ), 'sha256'), 'hex')
FROM public.outcome_monitor_runs AS run
LEFT JOIN LATERAL (
    SELECT candidate.observation_id, candidate.record_digest
      FROM public.outcome_observation_records AS candidate
      JOIN public.outcome_monitor_targets AS target
        ON target.owner_identity = run.owner_identity
       AND target.workspace_key = run.workspace_key
       AND target.target_id = run.target_id
     WHERE candidate.owner_identity = run.owner_identity
       AND candidate.workspace_key = run.workspace_key
       AND candidate.outcome_key = target.outcome_key
       AND candidate.indicator_key = target.indicator_key
       AND candidate.source_kind = target.source_kind
     ORDER BY
        (candidate.idempotency_key = run.idempotency_key) DESC,
        (candidate.recorded_at BETWEEN run.started_at AND run.completed_at) DESC,
        candidate.recorded_at DESC,
        candidate.observation_id DESC
     LIMIT 1
) AS observation ON true
WHERE run.status = 'succeeded'
ON CONFLICT (delivery_id) DO NOTHING;

CREATE CONSTRAINT TRIGGER trg_outcome_monitor_run_enqueue_composition_delivery
    AFTER INSERT ON public.outcome_monitor_runs
    DEFERRABLE INITIALLY DEFERRED
    FOR EACH ROW EXECUTE FUNCTION public.hai_enqueue_outcome_monitor_composition_delivery();

COMMENT ON TABLE public.outcome_monitor_composition_deliveries IS
    'One advisory-only durable composition queue row per successful owner/workspace-scoped outcome monitor run.';
COMMENT ON TABLE public.outcome_monitor_composition_attempts IS
    'Immutable, revision-fenced receipts for advisory-only outcome monitor composition attempts.';
