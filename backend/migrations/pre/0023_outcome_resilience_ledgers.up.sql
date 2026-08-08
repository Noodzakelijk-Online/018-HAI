CREATE OR REPLACE FUNCTION public.hai_reject_outcome_evaluation_mutation()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE EXCEPTION 'outcome evaluation history is append-only';
END;
$$;

CREATE TABLE public.outcome_evaluation_outcome_revisions (
    owner_identity text NOT NULL CHECK (char_length(owner_identity) BETWEEN 1 AND 256),
    workspace_id text NOT NULL CHECK (char_length(workspace_id) BETWEEN 1 AND 256),
    outcome_id text NOT NULL CHECK (char_length(outcome_id) BETWEEN 1 AND 256),
    revision bigint NOT NULL CHECK (revision > 0),
    idempotency_key text NOT NULL CHECK (char_length(idempotency_key) BETWEEN 1 AND 256),
    request_digest text NOT NULL CHECK (request_digest ~ '^[0-9a-f]{64}$'),
    audit_digest text NOT NULL CHECK (audit_digest ~ '^[0-9a-f]{64}$'),
    recorded_at timestamptz NOT NULL,
    payload jsonb NOT NULL CHECK (
        jsonb_typeof(payload) = 'object'
        AND octet_length(payload::text) BETWEEN 2 AND 1048576
    ),
    PRIMARY KEY (owner_identity, workspace_id, outcome_id, revision),
    UNIQUE (owner_identity, workspace_id, outcome_id, idempotency_key)
);

CREATE INDEX outcome_evaluation_outcome_revisions_latest_idx
    ON public.outcome_evaluation_outcome_revisions
    (owner_identity, workspace_id, outcome_id, revision DESC);

CREATE TABLE public.outcome_evaluation_evaluations (
    owner_identity text NOT NULL CHECK (char_length(owner_identity) BETWEEN 1 AND 256),
    workspace_id text NOT NULL CHECK (char_length(workspace_id) BETWEEN 1 AND 256),
    outcome_id text NOT NULL CHECK (char_length(outcome_id) BETWEEN 1 AND 256),
    evaluation_id text NOT NULL CHECK (char_length(evaluation_id) BETWEEN 1 AND 320),
    outcome_revision bigint NOT NULL CHECK (outcome_revision > 0),
    idempotency_key text NOT NULL CHECK (char_length(idempotency_key) BETWEEN 1 AND 256),
    request_digest text NOT NULL CHECK (request_digest ~ '^[0-9a-f]{64}$'),
    evaluation_audit_digest text NOT NULL CHECK (evaluation_audit_digest ~ '^[0-9a-f]{64}$'),
    record_digest text NOT NULL CHECK (record_digest ~ '^[0-9a-f]{64}$'),
    recorded_at timestamptz NOT NULL,
    payload jsonb NOT NULL CHECK (
        jsonb_typeof(payload) = 'object'
        AND octet_length(payload::text) BETWEEN 2 AND 1048576
    ),
    PRIMARY KEY (owner_identity, workspace_id, outcome_id, evaluation_id),
    UNIQUE (owner_identity, workspace_id, outcome_id, idempotency_key),
    FOREIGN KEY (owner_identity, workspace_id, outcome_id, outcome_revision)
        REFERENCES public.outcome_evaluation_outcome_revisions
        (owner_identity, workspace_id, outcome_id, revision)
        ON UPDATE RESTRICT ON DELETE RESTRICT
);

CREATE INDEX outcome_evaluation_evaluations_history_idx
    ON public.outcome_evaluation_evaluations
    (owner_identity, workspace_id, outcome_id, recorded_at DESC, evaluation_id DESC);

CREATE TABLE public.outcome_evaluation_corrections (
    owner_identity text NOT NULL CHECK (char_length(owner_identity) BETWEEN 1 AND 256),
    workspace_id text NOT NULL CHECK (char_length(workspace_id) BETWEEN 1 AND 256),
    outcome_id text NOT NULL CHECK (char_length(outcome_id) BETWEEN 1 AND 256),
    correction_id text NOT NULL CHECK (char_length(correction_id) BETWEEN 1 AND 256),
    observation_id text NOT NULL CHECK (char_length(observation_id) BETWEEN 1 AND 256),
    outcome_revision bigint NOT NULL CHECK (outcome_revision > 0),
    idempotency_key text NOT NULL CHECK (char_length(idempotency_key) BETWEEN 1 AND 256),
    request_digest text NOT NULL CHECK (request_digest ~ '^[0-9a-f]{64}$'),
    audit_digest text NOT NULL CHECK (audit_digest ~ '^[0-9a-f]{64}$'),
    recorded_at timestamptz NOT NULL,
    payload jsonb NOT NULL CHECK (
        jsonb_typeof(payload) = 'object'
        AND octet_length(payload::text) BETWEEN 2 AND 1048576
    ),
    PRIMARY KEY (owner_identity, workspace_id, outcome_id, correction_id),
    UNIQUE (owner_identity, workspace_id, outcome_id, idempotency_key),
    FOREIGN KEY (owner_identity, workspace_id, outcome_id, outcome_revision)
        REFERENCES public.outcome_evaluation_outcome_revisions
        (owner_identity, workspace_id, outcome_id, revision)
        ON UPDATE RESTRICT ON DELETE RESTRICT
);

CREATE INDEX outcome_evaluation_corrections_history_idx
    ON public.outcome_evaluation_corrections
    (owner_identity, workspace_id, outcome_id, recorded_at DESC, correction_id DESC);

DO $$
DECLARE
    table_name text;
BEGIN
    FOREACH table_name IN ARRAY ARRAY[
        'outcome_evaluation_outcome_revisions',
        'outcome_evaluation_evaluations',
        'outcome_evaluation_corrections'
    ]
    LOOP
        EXECUTE format(
            'CREATE TRIGGER %I BEFORE UPDATE OR DELETE ON public.%I FOR EACH ROW EXECUTE FUNCTION public.hai_reject_outcome_evaluation_mutation()',
            'trg_' || table_name || '_immutable', table_name
        );
        EXECUTE format(
            'CREATE TRIGGER %I BEFORE TRUNCATE ON public.%I FOR EACH STATEMENT EXECUTE FUNCTION public.hai_reject_outcome_evaluation_mutation()',
            'trg_' || table_name || '_no_truncate', table_name
        );
    END LOOP;
END;
$$;

CREATE OR REPLACE FUNCTION public.hai_reject_resilience_history_mutation()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE EXCEPTION 'resilience history is append-only';
END;
$$;

CREATE TABLE public.resilience_idempotency_records (
    owner_id text NOT NULL CHECK (char_length(owner_id) BETWEEN 1 AND 200),
    workspace_id text NOT NULL CHECK (char_length(workspace_id) BETWEEN 1 AND 200),
    idempotency_key char(64) NOT NULL CHECK (idempotency_key ~ '^[0-9a-f]{64}$'),
    work_id text NOT NULL CHECK (char_length(work_id) BETWEEN 1 AND 200),
    payload_hash char(64) NOT NULL CHECK (payload_hash ~ '^[0-9a-f]{64}$'),
    contract_version integer NOT NULL CHECK (contract_version = 1),
    recorded_at timestamptz NOT NULL,
    payload jsonb NOT NULL CHECK (
        jsonb_typeof(payload) = 'object'
        AND octet_length(payload::text) BETWEEN 2 AND 1048576
    ),
    PRIMARY KEY (owner_id, workspace_id, idempotency_key)
);

CREATE TABLE public.resilience_leases (
    owner_id text NOT NULL CHECK (char_length(owner_id) BETWEEN 1 AND 200),
    workspace_id text NOT NULL CHECK (char_length(workspace_id) BETWEEN 1 AND 200),
    work_id text NOT NULL CHECK (char_length(work_id) BETWEEN 1 AND 200),
    idempotency_key char(64) NOT NULL CHECK (idempotency_key ~ '^[0-9a-f]{64}$'),
    payload_hash char(64) NOT NULL CHECK (payload_hash ~ '^[0-9a-f]{64}$'),
    worker_id text NOT NULL CHECK (char_length(worker_id) BETWEEN 1 AND 200),
    generation numeric(20,0) NOT NULL CHECK (generation >= 1),
    lease_state text NOT NULL CHECK (lease_state IN ('active', 'released')),
    acquired_at timestamptz NOT NULL,
    last_heartbeat_at timestamptz NOT NULL,
    expires_at timestamptz NOT NULL,
    released_at timestamptz,
    contract_version integer NOT NULL CHECK (contract_version = 1),
    payload jsonb NOT NULL CHECK (
        jsonb_typeof(payload) = 'object'
        AND octet_length(payload::text) BETWEEN 2 AND 1048576
    ),
    PRIMARY KEY (owner_id, workspace_id, work_id)
);

CREATE INDEX resilience_leases_scope_work_idx
    ON public.resilience_leases (owner_id, workspace_id, work_id);

CREATE TABLE public.resilience_worker_heartbeats (
    owner_id text NOT NULL CHECK (char_length(owner_id) BETWEEN 1 AND 200),
    workspace_id text NOT NULL CHECK (char_length(workspace_id) BETWEEN 1 AND 200),
    worker_id text NOT NULL CHECK (char_length(worker_id) BETWEEN 1 AND 200),
    sequence numeric(20,0) NOT NULL CHECK (sequence >= 1),
    observed_at timestamptz NOT NULL,
    contract_version integer NOT NULL CHECK (contract_version = 1),
    payload jsonb NOT NULL CHECK (
        jsonb_typeof(payload) = 'object'
        AND octet_length(payload::text) BETWEEN 2 AND 1048576
    ),
    PRIMARY KEY (owner_id, workspace_id, worker_id)
);

CREATE INDEX resilience_worker_heartbeats_scope_worker_idx
    ON public.resilience_worker_heartbeats (owner_id, workspace_id, worker_id);

CREATE TABLE public.resilience_circuits (
    owner_id text NOT NULL CHECK (char_length(owner_id) BETWEEN 1 AND 200),
    workspace_id text NOT NULL CHECK (char_length(workspace_id) BETWEEN 1 AND 200),
    circuit_id text NOT NULL CHECK (char_length(circuit_id) BETWEEN 1 AND 200),
    revision numeric(20,0) NOT NULL CHECK (revision >= 1),
    circuit_phase text NOT NULL CHECK (circuit_phase IN ('closed', 'open', 'half_open')),
    updated_at timestamptz NOT NULL,
    contract_version integer NOT NULL CHECK (contract_version = 1),
    payload jsonb NOT NULL CHECK (
        jsonb_typeof(payload) = 'object'
        AND octet_length(payload::text) BETWEEN 2 AND 1048576
    ),
    PRIMARY KEY (owner_id, workspace_id, circuit_id)
);

CREATE INDEX resilience_circuits_scope_circuit_idx
    ON public.resilience_circuits (owner_id, workspace_id, circuit_id);

CREATE TABLE public.resilience_retry_records (
    owner_id text NOT NULL CHECK (char_length(owner_id) BETWEEN 1 AND 200),
    workspace_id text NOT NULL CHECK (char_length(workspace_id) BETWEEN 1 AND 200),
    work_id text NOT NULL CHECK (char_length(work_id) BETWEEN 1 AND 200),
    sequence numeric(20,0) NOT NULL CHECK (sequence >= 1),
    requested_at timestamptz NOT NULL,
    contract_version integer NOT NULL CHECK (contract_version = 1),
    payload jsonb NOT NULL CHECK (
        jsonb_typeof(payload) = 'object'
        AND octet_length(payload::text) BETWEEN 2 AND 1048576
    ),
    PRIMARY KEY (owner_id, workspace_id, work_id, sequence)
);

CREATE INDEX resilience_retries_scope_work_sequence_idx
    ON public.resilience_retry_records (owner_id, workspace_id, work_id, sequence DESC);
CREATE INDEX resilience_retries_scope_requested_idx
    ON public.resilience_retry_records (owner_id, workspace_id, requested_at DESC, work_id);

CREATE TABLE public.resilience_recovery_records (
    owner_id text NOT NULL CHECK (char_length(owner_id) BETWEEN 1 AND 200),
    workspace_id text NOT NULL CHECK (char_length(workspace_id) BETWEEN 1 AND 200),
    work_id text NOT NULL CHECK (char_length(work_id) BETWEEN 1 AND 200),
    sequence numeric(20,0) NOT NULL CHECK (sequence >= 1),
    requested_at timestamptz NOT NULL,
    contract_version integer NOT NULL CHECK (contract_version = 1),
    payload jsonb NOT NULL CHECK (
        jsonb_typeof(payload) = 'object'
        AND octet_length(payload::text) BETWEEN 2 AND 1048576
    ),
    PRIMARY KEY (owner_id, workspace_id, work_id, sequence)
);

CREATE INDEX resilience_recoveries_scope_work_sequence_idx
    ON public.resilience_recovery_records (owner_id, workspace_id, work_id, sequence DESC);
CREATE INDEX resilience_recoveries_scope_requested_idx
    ON public.resilience_recovery_records (owner_id, workspace_id, requested_at DESC, work_id);

CREATE TABLE public.resilience_event_records (
    owner_id text NOT NULL CHECK (char_length(owner_id) BETWEEN 1 AND 200),
    workspace_id text NOT NULL CHECK (char_length(workspace_id) BETWEEN 1 AND 200),
    sequence numeric(20,0) NOT NULL CHECK (sequence >= 1),
    event_hash char(64) NOT NULL CHECK (event_hash ~ '^[0-9a-f]{64}$'),
    previous_hash varchar(64) NOT NULL CHECK (previous_hash = '' OR previous_hash ~ '^[0-9a-f]{64}$'),
    event_type text NOT NULL CHECK (char_length(event_type) BETWEEN 1 AND 200),
    subject_id text NOT NULL CHECK (char_length(subject_id) BETWEEN 1 AND 200),
    occurred_at timestamptz NOT NULL,
    contract_version integer NOT NULL CHECK (contract_version = 1),
    payload jsonb NOT NULL CHECK (
        jsonb_typeof(payload) = 'object'
        AND octet_length(payload::text) BETWEEN 2 AND 1048576
    ),
    PRIMARY KEY (owner_id, workspace_id, sequence),
    UNIQUE (owner_id, workspace_id, event_hash)
);

CREATE INDEX resilience_events_scope_sequence_idx
    ON public.resilience_event_records (owner_id, workspace_id, sequence DESC);

DO $$
DECLARE
    table_name text;
BEGIN
    FOREACH table_name IN ARRAY ARRAY[
        'resilience_retry_records',
        'resilience_recovery_records',
        'resilience_event_records'
    ]
    LOOP
        EXECUTE format(
            'CREATE TRIGGER %I BEFORE UPDATE OR DELETE ON public.%I FOR EACH ROW EXECUTE FUNCTION public.hai_reject_resilience_history_mutation()',
            'trg_' || table_name || '_immutable', table_name
        );
        EXECUTE format(
            'CREATE TRIGGER %I BEFORE TRUNCATE ON public.%I FOR EACH STATEMENT EXECUTE FUNCTION public.hai_reject_resilience_history_mutation()',
            'trg_' || table_name || '_no_truncate', table_name
        );
    END LOOP;
END;
$$;
