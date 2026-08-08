CREATE TABLE public.outcome_observation_records (
    observation_id uuid PRIMARY KEY,
    owner_identity character varying(255) NOT NULL,
    workspace_key character varying(255) NOT NULL,
    outcome_key character varying(255) NOT NULL,
    indicator_key character varying(255) NOT NULL,
    source_kind character varying(80) NOT NULL,
    source_key character varying(512) NOT NULL,
    source_digest character(64) NOT NULL,
    numeric_value numeric NOT NULL,
    unit character varying(80) NOT NULL,
    idempotency_key character varying(160) NOT NULL,
    request_digest character(64) NOT NULL,
    record_digest character(64) NOT NULL,
    observed_at timestamp with time zone NOT NULL,
    recorded_at timestamp with time zone NOT NULL,
    authority character varying(80) NOT NULL DEFAULT 'advisory_monitor_only',
    can_execute boolean NOT NULL DEFAULT false,
    delivery_authorized boolean NOT NULL DEFAULT false,
    execution_authorized boolean NOT NULL DEFAULT false,
    CONSTRAINT uq_outcome_observation_owner_idempotency
        UNIQUE (owner_identity, workspace_key, idempotency_key),
    CONSTRAINT uq_outcome_observation_owner_record_digest
        UNIQUE (owner_identity, workspace_key, record_digest),
    CONSTRAINT uq_outcome_observation_source_revision
        UNIQUE (
            owner_identity, workspace_key, outcome_key, indicator_key,
            source_kind, source_key, source_digest
        ),
    CONSTRAINT chk_outcome_observation_identity CHECK (
        char_length(btrim(owner_identity)) BETWEEN 1 AND 255
        AND owner_identity = btrim(owner_identity)
        AND char_length(btrim(workspace_key)) BETWEEN 1 AND 255
        AND char_length(btrim(outcome_key)) BETWEEN 1 AND 255
        AND char_length(btrim(indicator_key)) BETWEEN 1 AND 255
        AND workspace_key = btrim(workspace_key)
        AND outcome_key = btrim(outcome_key)
        AND indicator_key = btrim(indicator_key)
        AND idempotency_key ~ '^[A-Za-z0-9._:-]{1,160}$'
    ),
    CONSTRAINT chk_outcome_observation_source CHECK (
        source_kind ~ '^[a-z][a-z0-9_]{0,79}$'
        AND char_length(btrim(source_key)) BETWEEN 1 AND 512
        AND source_key = btrim(source_key)
        AND source_digest ~ '^[0-9a-f]{64}$'
    ),
    CONSTRAINT chk_outcome_observation_numeric CHECK (
        numeric_value NOT IN ('NaN'::numeric, 'Infinity'::numeric, '-Infinity'::numeric)
        AND char_length(btrim(unit)) BETWEEN 1 AND 80
        AND unit = btrim(unit)
    ),
    CONSTRAINT chk_outcome_observation_digests CHECK (
        request_digest ~ '^[0-9a-f]{64}$'
        AND record_digest ~ '^[0-9a-f]{64}$'
    ),
    CONSTRAINT chk_outcome_observation_time CHECK (
        observed_at <= recorded_at
    ),
    CONSTRAINT chk_outcome_observation_advisory_only CHECK (
        authority = 'advisory_monitor_only'
        AND can_execute = false
        AND delivery_authorized = false
        AND execution_authorized = false
    )
);

CREATE INDEX idx_outcome_observation_scope_history
    ON public.outcome_observation_records
    (owner_identity, workspace_key, outcome_key, indicator_key, observed_at DESC, observation_id DESC);
CREATE INDEX idx_outcome_observation_source_history
    ON public.outcome_observation_records
    (owner_identity, source_kind, source_key, observed_at DESC, observation_id DESC);

CREATE TABLE public.outcome_monitor_targets (
    target_id uuid PRIMARY KEY,
    owner_identity character varying(255) NOT NULL,
    workspace_key character varying(255) NOT NULL,
    outcome_key character varying(255) NOT NULL,
    indicator_key character varying(255) NOT NULL,
    source_kind character varying(80) NOT NULL,
    cadence_seconds integer NOT NULL,
    next_run_at timestamp with time zone NOT NULL,
    enabled boolean NOT NULL DEFAULT true,
    revision bigint NOT NULL DEFAULT 1,
    lease_id uuid,
    lease_owner character varying(255),
    lease_until timestamp with time zone,
    last_run_at timestamp with time zone,
    last_result character varying(20),
    last_digest character(64),
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL,
    CONSTRAINT uq_outcome_monitor_target_owner_scope
        UNIQUE (owner_identity, workspace_key, outcome_key, indicator_key, source_kind),
    CONSTRAINT uq_outcome_monitor_target_owner_workspace_id
        UNIQUE (owner_identity, workspace_key, target_id),
    CONSTRAINT uq_outcome_monitor_target_owner_id_revision
        UNIQUE (owner_identity, target_id, revision),
    CONSTRAINT chk_outcome_monitor_target_identity CHECK (
        char_length(btrim(owner_identity)) BETWEEN 1 AND 255
        AND owner_identity = btrim(owner_identity)
        AND char_length(btrim(workspace_key)) BETWEEN 1 AND 255
        AND char_length(btrim(outcome_key)) BETWEEN 1 AND 255
        AND char_length(btrim(indicator_key)) BETWEEN 1 AND 255
        AND workspace_key = btrim(workspace_key)
        AND outcome_key = btrim(outcome_key)
        AND indicator_key = btrim(indicator_key)
        AND source_kind ~ '^[a-z][a-z0-9_]{0,79}$'
    ),
    CONSTRAINT chk_outcome_monitor_target_schedule CHECK (
        cadence_seconds BETWEEN 60 AND 2592000
        AND revision >= 1
        AND created_at <= updated_at
    ),
    CONSTRAINT chk_outcome_monitor_target_lease CHECK (
        (lease_id IS NULL AND lease_owner IS NULL AND lease_until IS NULL)
        OR (
            lease_id IS NOT NULL
            AND lease_owner IS NOT NULL
            AND lease_owner ~ '^[A-Za-z0-9][A-Za-z0-9._:@/-]{0,127}$'
            AND lease_until IS NOT NULL
            AND enabled
            AND lease_until > updated_at
            AND lease_until <= updated_at + interval '1 hour'
        )
    ),
    CONSTRAINT chk_outcome_monitor_target_last_result CHECK (
        (
            last_run_at IS NULL
            AND last_result IS NULL
            AND last_digest IS NULL
        )
        OR (
            last_run_at IS NOT NULL
            AND last_result IN ('succeeded', 'failed', 'skipped')
            AND last_digest ~ '^[0-9a-f]{64}$'
            AND last_run_at <= updated_at
        )
    )
);

CREATE INDEX idx_outcome_monitor_targets_due_claim
    ON public.outcome_monitor_targets
    (next_run_at, lease_until, owner_identity, target_id)
    WHERE enabled = true;
CREATE INDEX idx_outcome_monitor_targets_owner_schedule
    ON public.outcome_monitor_targets
    (owner_identity, enabled, next_run_at, target_id);

CREATE TABLE public.outcome_monitor_commands (
    owner_identity character varying(255) NOT NULL,
    workspace_key character varying(255) NOT NULL,
    operation character varying(40) NOT NULL,
    idempotency_key character varying(160) NOT NULL,
    request_digest character(64) NOT NULL,
    target_id uuid NOT NULL,
    result_revision bigint NOT NULL,
    result_lease_generation bigint NOT NULL,
    result_lease_id uuid,
    result_lease_owner character varying(255),
    result_lease_until timestamp with time zone,
    result_enabled boolean NOT NULL,
    result_next_run_at timestamp with time zone NOT NULL,
    result_updated_at timestamp with time zone NOT NULL,
    recorded_at timestamp with time zone NOT NULL,
    PRIMARY KEY (owner_identity, workspace_key, operation, idempotency_key),
    CONSTRAINT fk_outcome_monitor_command_target
        FOREIGN KEY (owner_identity, workspace_key, target_id)
        REFERENCES public.outcome_monitor_targets (owner_identity, workspace_key, target_id)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT chk_outcome_monitor_command_identity CHECK (
        char_length(btrim(owner_identity)) BETWEEN 1 AND 255
        AND owner_identity = btrim(owner_identity)
        AND char_length(btrim(workspace_key)) BETWEEN 1 AND 255
        AND workspace_key = btrim(workspace_key)
        AND operation IN ('create_target', 'set_enabled')
        AND idempotency_key ~ '^[A-Za-z0-9._:-]{1,160}$'
        AND request_digest ~ '^[0-9a-f]{64}$'
    ),
    CONSTRAINT chk_outcome_monitor_command_result CHECK (
        result_revision >= 1
        AND result_lease_generation >= 0
        AND result_lease_generation <= result_revision
        AND (
            (result_lease_id IS NULL AND result_lease_owner IS NULL AND result_lease_until IS NULL)
            OR (
                result_lease_id IS NOT NULL
                AND result_lease_owner IS NOT NULL
                AND result_lease_owner ~ '^[A-Za-z0-9][A-Za-z0-9._:@/-]{0,127}$'
                AND result_lease_until IS NOT NULL
                AND result_lease_generation >= 1
                AND result_updated_at < result_lease_until
            )
        )
        AND result_updated_at <= recorded_at
    )
);

CREATE INDEX idx_outcome_monitor_commands_target_history
    ON public.outcome_monitor_commands
    (owner_identity, workspace_key, target_id, recorded_at DESC, operation, idempotency_key);

CREATE TABLE public.outcome_monitor_runs (
    run_id uuid PRIMARY KEY,
    target_id uuid NOT NULL,
    owner_identity character varying(255) NOT NULL,
    workspace_key character varying(255) NOT NULL,
    target_revision bigint NOT NULL,
    claim_id uuid NOT NULL,
    claimed_at timestamp with time zone NOT NULL,
    status character varying(20) NOT NULL,
    observation_count integer NOT NULL DEFAULT 0,
    signal_count integer NOT NULL DEFAULT 0,
    error_message_redacted text,
    error_was_redacted boolean NOT NULL DEFAULT false,
    idempotency_key character varying(160) NOT NULL,
    request_digest character(64) NOT NULL,
    record_digest character(64) NOT NULL,
    started_at timestamp with time zone NOT NULL,
    completed_at timestamp with time zone NOT NULL,
    CONSTRAINT uq_outcome_monitor_run_owner_idempotency
        UNIQUE (owner_identity, workspace_key, idempotency_key),
    CONSTRAINT uq_outcome_monitor_run_owner_record_digest
        UNIQUE (owner_identity, workspace_key, record_digest),
    CONSTRAINT fk_outcome_monitor_run_target
        FOREIGN KEY (owner_identity, workspace_key, target_id)
        REFERENCES public.outcome_monitor_targets (owner_identity, workspace_key, target_id)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT chk_outcome_monitor_run_identity CHECK (
        char_length(btrim(owner_identity)) BETWEEN 1 AND 255
        AND owner_identity = btrim(owner_identity)
        AND char_length(btrim(workspace_key)) BETWEEN 1 AND 255
        AND workspace_key = btrim(workspace_key)
        AND target_revision >= 1
        AND idempotency_key ~ '^[A-Za-z0-9._:-]{1,160}$'
    ),
    CONSTRAINT chk_outcome_monitor_run_status CHECK (
        status IN ('succeeded', 'failed', 'skipped')
        AND observation_count >= 0
        AND signal_count >= 0
        AND (status <> 'skipped' OR (observation_count = 0 AND signal_count = 0))
    ),
    CONSTRAINT chk_outcome_monitor_run_error CHECK (
        (
            status = 'failed'
            AND error_message_redacted IS NOT NULL
            AND error_was_redacted
            AND char_length(btrim(error_message_redacted)) BETWEEN 1 AND 2000
            AND error_message_redacted !~ '[[:cntrl:]]'
        )
        OR (
            status <> 'failed'
            AND error_message_redacted IS NULL
            AND error_was_redacted = false
        )
    ),
    CONSTRAINT chk_outcome_monitor_run_digests CHECK (
        request_digest ~ '^[0-9a-f]{64}$'
        AND record_digest ~ '^[0-9a-f]{64}$'
    ),
    CONSTRAINT chk_outcome_monitor_run_time CHECK (
        claimed_at <= started_at
        AND started_at <= completed_at
    )
);

CREATE INDEX idx_outcome_monitor_runs_target_history
    ON public.outcome_monitor_runs
    (owner_identity, workspace_key, target_id, started_at DESC, run_id DESC);
CREATE INDEX idx_outcome_monitor_runs_status_history
    ON public.outcome_monitor_runs
    (owner_identity, workspace_key, status, completed_at DESC, run_id DESC);

CREATE OR REPLACE FUNCTION public.hai_reject_outcome_monitor_ledger_mutation()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE EXCEPTION 'outcome attention monitor ledgers are append-only'
        USING ERRCODE = '55000';
END;
$$;

CREATE OR REPLACE FUNCTION public.hai_validate_outcome_monitor_target_write()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    run_record public.outcome_monitor_runs%ROWTYPE;
BEGIN
    IF TG_OP = 'DELETE' THEN
        RAISE EXCEPTION 'outcome monitor targets must be disabled, not deleted'
            USING ERRCODE = '55000';
    END IF;

    IF TG_OP = 'INSERT' THEN
        IF NEW.revision <> 1
           OR NEW.created_at <> NEW.updated_at
           OR NEW.lease_id IS NOT NULL
           OR NEW.lease_owner IS NOT NULL
           OR NEW.lease_until IS NOT NULL
           OR NEW.last_run_at IS NOT NULL
           OR NEW.last_result IS NOT NULL
           OR NEW.last_digest IS NOT NULL THEN
            RAISE EXCEPTION 'new outcome monitor target must start at an unclaimed revision one'
                USING ERRCODE = 'integrity_constraint_violation';
        END IF;
        RETURN NEW;
    END IF;

    IF NEW.target_id <> OLD.target_id
       OR NEW.owner_identity <> OLD.owner_identity
       OR NEW.workspace_key <> OLD.workspace_key
       OR NEW.outcome_key <> OLD.outcome_key
       OR NEW.indicator_key <> OLD.indicator_key
       OR NEW.source_kind <> OLD.source_kind
       OR NEW.created_at <> OLD.created_at THEN
        RAISE EXCEPTION 'outcome monitor target owner and scope are immutable'
            USING ERRCODE = 'integrity_constraint_violation';
    END IF;
    IF NEW.revision <> OLD.revision + 1 OR NEW.updated_at <= OLD.updated_at THEN
        RAISE EXCEPTION 'outcome monitor target update must advance revision and time exactly once'
            USING ERRCODE = 'serialization_failure';
    END IF;
    IF NOT NEW.enabled AND (NEW.lease_id IS NOT NULL OR NEW.lease_owner IS NOT NULL OR NEW.lease_until IS NOT NULL) THEN
        RAISE EXCEPTION 'disabled outcome monitor target cannot retain a lease'
            USING ERRCODE = 'integrity_constraint_violation';
    END IF;
    IF OLD.lease_id IS NOT NULL
       AND OLD.lease_until > NEW.updated_at
       AND (
           NEW.lease_id IS DISTINCT FROM OLD.lease_id
           OR NEW.lease_owner IS DISTINCT FROM OLD.lease_owner
           OR NEW.lease_until IS DISTINCT FROM OLD.lease_until
       )
       AND NOT (
           NEW.lease_id IS NULL
           AND NEW.lease_owner IS NULL
           AND (
               (NOT NEW.enabled AND OLD.enabled)
               OR (
                   NEW.last_run_at IS NOT NULL
                   AND NEW.last_run_at IS DISTINCT FROM OLD.last_run_at
               )
           )
       ) THEN
        RAISE EXCEPTION 'active outcome monitor lease cannot be replaced before expiry'
            USING ERRCODE = 'lock_not_available';
    END IF;
    IF NEW.last_run_at IS NOT DISTINCT FROM OLD.last_run_at
       AND (
           NEW.last_result IS DISTINCT FROM OLD.last_result
           OR NEW.last_digest IS DISTINCT FROM OLD.last_digest
       ) THEN
        RAISE EXCEPTION 'outcome monitor result cannot change without a new run time'
            USING ERRCODE = 'integrity_constraint_violation';
    END IF;
    IF NEW.last_run_at IS DISTINCT FROM OLD.last_run_at
       AND (
           NEW.last_run_at IS NULL
           OR (OLD.last_run_at IS NOT NULL AND NEW.last_run_at <= OLD.last_run_at)
           OR NEW.last_digest IS NOT DISTINCT FROM OLD.last_digest
       ) THEN
        RAISE EXCEPTION 'outcome monitor run projection must advance monotonically'
            USING ERRCODE = 'integrity_constraint_violation';
    END IF;
    IF NEW.last_run_at IS DISTINCT FROM OLD.last_run_at THEN
        SELECT * INTO run_record
          FROM public.outcome_monitor_runs
         WHERE owner_identity = OLD.owner_identity
           AND workspace_key = OLD.workspace_key
           AND target_id = OLD.target_id
         ORDER BY completed_at DESC, run_id DESC
         LIMIT 1;
        IF NOT FOUND
           OR run_record.target_revision <> OLD.revision
           OR run_record.claim_id IS DISTINCT FROM OLD.lease_id
           OR run_record.completed_at <> NEW.last_run_at
           OR run_record.status <> NEW.last_result
           OR run_record.record_digest <> NEW.last_digest THEN
            RAISE EXCEPTION 'outcome monitor run projection does not match immutable run history'
                USING ERRCODE = 'integrity_constraint_violation';
        END IF;
    END IF;
    RETURN NEW;
END;
$$;

CREATE OR REPLACE FUNCTION public.hai_validate_outcome_monitor_run_insert()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    target_record public.outcome_monitor_targets%ROWTYPE;
BEGIN
    SELECT * INTO target_record
      FROM public.outcome_monitor_targets
     WHERE target_id = NEW.target_id
     FOR UPDATE;
    IF NOT FOUND
       OR target_record.owner_identity <> NEW.owner_identity
       OR target_record.workspace_key <> NEW.workspace_key
       OR target_record.revision <> NEW.target_revision
       OR NOT target_record.enabled
       OR target_record.lease_id IS NULL
       OR target_record.lease_id <> NEW.claim_id
       OR target_record.lease_until IS NULL
       OR target_record.updated_at <> NEW.claimed_at
       OR target_record.lease_until < NEW.completed_at THEN
        RAISE EXCEPTION 'outcome monitor run does not match the current owner-scoped claim'
            USING ERRCODE = 'foreign_key_violation';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_outcome_observation_records_immutable
    BEFORE UPDATE OR DELETE ON public.outcome_observation_records
    FOR EACH ROW EXECUTE FUNCTION public.hai_reject_outcome_monitor_ledger_mutation();
CREATE TRIGGER trg_outcome_observation_records_no_truncate
    BEFORE TRUNCATE ON public.outcome_observation_records
    FOR EACH STATEMENT EXECUTE FUNCTION public.hai_reject_outcome_monitor_ledger_mutation();

CREATE TRIGGER trg_outcome_monitor_commands_immutable
    BEFORE UPDATE OR DELETE ON public.outcome_monitor_commands
    FOR EACH ROW EXECUTE FUNCTION public.hai_reject_outcome_monitor_ledger_mutation();
CREATE TRIGGER trg_outcome_monitor_commands_no_truncate
    BEFORE TRUNCATE ON public.outcome_monitor_commands
    FOR EACH STATEMENT EXECUTE FUNCTION public.hai_reject_outcome_monitor_ledger_mutation();

CREATE TRIGGER trg_outcome_monitor_targets_validate_insert
    BEFORE INSERT ON public.outcome_monitor_targets
    FOR EACH ROW EXECUTE FUNCTION public.hai_validate_outcome_monitor_target_write();
CREATE TRIGGER trg_outcome_monitor_targets_validate_update
    BEFORE UPDATE ON public.outcome_monitor_targets
    FOR EACH ROW EXECUTE FUNCTION public.hai_validate_outcome_monitor_target_write();
CREATE TRIGGER trg_outcome_monitor_targets_reject_delete
    BEFORE DELETE ON public.outcome_monitor_targets
    FOR EACH ROW EXECUTE FUNCTION public.hai_validate_outcome_monitor_target_write();

CREATE TRIGGER trg_outcome_monitor_runs_validate_insert
    BEFORE INSERT ON public.outcome_monitor_runs
    FOR EACH ROW EXECUTE FUNCTION public.hai_validate_outcome_monitor_run_insert();
CREATE TRIGGER trg_outcome_monitor_runs_immutable
    BEFORE UPDATE OR DELETE ON public.outcome_monitor_runs
    FOR EACH ROW EXECUTE FUNCTION public.hai_reject_outcome_monitor_ledger_mutation();
CREATE TRIGGER trg_outcome_monitor_runs_no_truncate
    BEFORE TRUNCATE ON public.outcome_monitor_runs
    FOR EACH STATEMENT EXECUTE FUNCTION public.hai_reject_outcome_monitor_ledger_mutation();
