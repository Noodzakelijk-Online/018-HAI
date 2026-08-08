DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM public.outcome_monitor_composition_attempts LIMIT 1)
       OR EXISTS (SELECT 1 FROM public.outcome_monitor_composition_deliveries LIMIT 1) THEN
        RAISE EXCEPTION 'refusing to roll back non-empty outcome monitor composition snapshot ledgers';
    END IF;
END;
$$;

DROP TRIGGER IF EXISTS trg_outcome_monitor_composition_delivery_0051_snapshot_no_truncate
    ON public.outcome_monitor_composition_deliveries;
DROP TRIGGER IF EXISTS trg_outcome_monitor_composition_delivery_0051_snapshot_immutable
    ON public.outcome_monitor_composition_deliveries;
DROP TRIGGER IF EXISTS trg_outcome_monitor_composition_delivery_0051_snapshot_insert
    ON public.outcome_monitor_composition_deliveries;

ALTER TABLE public.outcome_monitor_composition_attempts
    DROP CONSTRAINT IF EXISTS fk_outcome_monitor_composition_attempt_snapshot,
    DROP CONSTRAINT IF EXISTS chk_outcome_monitor_composition_attempt_snapshot_digest;

ALTER TABLE public.outcome_monitor_composition_deliveries
    DROP CONSTRAINT IF EXISTS uq_outcome_monitor_composition_delivery_snapshot,
    DROP CONSTRAINT IF EXISTS fk_outcome_monitor_composition_snapshot_feedback,
    DROP CONSTRAINT IF EXISTS fk_outcome_monitor_composition_snapshot_policy,
    DROP CONSTRAINT IF EXISTS chk_outcome_monitor_composition_snapshot_shape,
    DROP CONSTRAINT IF EXISTS chk_outcome_monitor_composition_delivery_attempt_projection;

ALTER TABLE public.outcome_monitor_composition_deliveries
    ADD CONSTRAINT chk_outcome_monitor_composition_delivery_attempt_projection CHECK (
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
    );

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

ALTER TABLE public.outcome_monitor_composition_attempts
    DROP COLUMN IF EXISTS snapshot_digest;

ALTER TABLE public.outcome_monitor_composition_deliveries
    DROP COLUMN IF EXISTS snapshot_digest,
    DROP COLUMN IF EXISTS attention_snapshot_digest,
    DROP COLUMN IF EXISTS attention_snapshot,
    DROP COLUMN IF EXISTS feedback_watermark_digest,
    DROP COLUMN IF EXISTS feedback_watermark_id,
    DROP COLUMN IF EXISTS feedback_watermark_at,
    DROP COLUMN IF EXISTS decision_watermark_key,
    DROP COLUMN IF EXISTS decision_watermark_at,
    DROP COLUMN IF EXISTS signal_watermark_key,
    DROP COLUMN IF EXISTS signal_watermark_at,
    DROP COLUMN IF EXISTS policy_recorded_at,
    DROP COLUMN IF EXISTS policy_payload_digest,
    DROP COLUMN IF EXISTS policy_idempotency_key,
    DROP COLUMN IF EXISTS outcome_audit_digest,
    DROP COLUMN IF EXISTS outcome_revision,
    DROP COLUMN IF EXISTS snapshot_captured_at,
    DROP COLUMN IF EXISTS composer_version,
    DROP COLUMN IF EXISTS snapshot_status;

DROP FUNCTION IF EXISTS public.hai_reject_outcome_monitor_composition_snapshot_mutation();
DROP FUNCTION IF EXISTS public.hai_pin_outcome_monitor_composition_snapshot();
DROP FUNCTION IF EXISTS public.hai_validate_outcome_monitor_attention_snapshot(
    text, timestamp with time zone, jsonb
);
DROP FUNCTION IF EXISTS public.hai_outcome_monitor_composition_snapshot_digest(
    text, text, timestamp with time zone, bigint, text, jsonb
);
DROP FUNCTION IF EXISTS public.hai_outcome_monitor_composition_binding_digest(
    text, text, uuid, uuid, uuid, text, uuid, text, text
);
DROP FUNCTION IF EXISTS public.hai_outcome_monitor_attention_snapshot_json(jsonb);
DROP FUNCTION IF EXISTS public.hai_snapshot_rfc3339_utc(timestamp with time zone);
