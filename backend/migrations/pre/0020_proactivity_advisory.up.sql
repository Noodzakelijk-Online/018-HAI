CREATE TABLE public.proactivity_idempotency (
    owner_identity text NOT NULL CHECK (length(owner_identity) BETWEEN 1 AND 320),
    idempotency_key text NOT NULL CHECK (length(idempotency_key) BETWEEN 1 AND 200),
    record_kind text NOT NULL CHECK (record_kind IN ('policy', 'signals', 'decisions')),
    payload_digest char(64) NOT NULL CHECK (payload_digest ~ '^[a-f0-9]{64}$'),
    recorded_at timestamptz NOT NULL,
    CONSTRAINT proactivity_idempotency_pkey
        PRIMARY KEY (owner_identity, idempotency_key),
    CONSTRAINT uq_proactivity_idempotency_envelope
        UNIQUE (owner_identity, idempotency_key, record_kind, payload_digest)
);

CREATE TABLE public.proactivity_policy_records (
    owner_identity text NOT NULL,
    idempotency_key text NOT NULL,
    record_kind text NOT NULL DEFAULT 'policy' CHECK (record_kind = 'policy'),
    payload_digest char(64) NOT NULL CHECK (payload_digest ~ '^[a-f0-9]{64}$'),
    recorded_at timestamptz NOT NULL,
    payload jsonb NOT NULL,
    CONSTRAINT proactivity_policy_records_pkey
        PRIMARY KEY (owner_identity, idempotency_key),
    CONSTRAINT fk_proactivity_policy_idempotency
        FOREIGN KEY (owner_identity, idempotency_key, record_kind, payload_digest)
        REFERENCES public.proactivity_idempotency
            (owner_identity, idempotency_key, record_kind, payload_digest)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT chk_proactivity_policy_payload CHECK (
        jsonb_typeof(payload) = 'object'
        AND octet_length(payload::text) BETWEEN 2 AND 262144
        AND payload ?& ARRAY['contractVersion', 'ownerIdentity', 'policy', 'recordedAt']
        AND payload #>> '{contractVersion}' = '1'
        AND payload #>> '{ownerIdentity}' = owner_identity
        AND jsonb_typeof(payload -> 'policy') = 'object'
        AND payload #>> '{policy,contractVersion}' = '1'
        AND payload #>> '{policy,ownerIdentity}' = owner_identity
        AND abs(extract(epoch FROM (
            (payload #>> '{recordedAt}')::timestamptz - recorded_at
        ))) < 0.001
    )
);

CREATE INDEX proactivity_policy_owner_recorded_idx
    ON public.proactivity_policy_records
    (owner_identity, recorded_at DESC, idempotency_key DESC);

CREATE TABLE public.proactivity_signal_batches (
    owner_identity text NOT NULL,
    idempotency_key text NOT NULL,
    record_kind text NOT NULL DEFAULT 'signals' CHECK (record_kind = 'signals'),
    payload_digest char(64) NOT NULL CHECK (payload_digest ~ '^[a-f0-9]{64}$'),
    signal_count integer NOT NULL CHECK (signal_count BETWEEN 1 AND 256),
    recorded_at timestamptz NOT NULL,
    payload jsonb NOT NULL,
    CONSTRAINT proactivity_signal_batches_pkey
        PRIMARY KEY (owner_identity, idempotency_key),
    CONSTRAINT fk_proactivity_signal_batch_idempotency
        FOREIGN KEY (owner_identity, idempotency_key, record_kind, payload_digest)
        REFERENCES public.proactivity_idempotency
            (owner_identity, idempotency_key, record_kind, payload_digest)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT chk_proactivity_signal_batch_payload CHECK (
        jsonb_typeof(payload) = 'array'
        AND octet_length(payload::text) BETWEEN 2 AND 4194304
        AND jsonb_array_length(payload) = signal_count
    )
);

CREATE TABLE public.proactivity_signal_records (
    owner_identity text NOT NULL,
    batch_idempotency_key text NOT NULL,
    ordinal integer NOT NULL CHECK (ordinal BETWEEN 0 AND 255),
    signal_id text NOT NULL CHECK (length(signal_id) BETWEEN 1 AND 200),
    recorded_at timestamptz NOT NULL,
    payload jsonb NOT NULL,
    CONSTRAINT proactivity_signal_records_pkey
        PRIMARY KEY (owner_identity, batch_idempotency_key, ordinal),
    CONSTRAINT uq_proactivity_signal_batch_signal
        UNIQUE (owner_identity, batch_idempotency_key, signal_id),
    CONSTRAINT fk_proactivity_signal_batch
        FOREIGN KEY (owner_identity, batch_idempotency_key)
        REFERENCES public.proactivity_signal_batches (owner_identity, idempotency_key)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT chk_proactivity_signal_payload CHECK (
        jsonb_typeof(payload) = 'object'
        AND octet_length(payload::text) BETWEEN 2 AND 262144
        AND payload ?& ARRAY['contractVersion', 'ownerIdentity', 'signal', 'recordedAt']
        AND payload #>> '{contractVersion}' = '1'
        AND payload #>> '{ownerIdentity}' = owner_identity
        AND jsonb_typeof(payload -> 'signal') = 'object'
        AND payload #>> '{signal,contractVersion}' = '1'
        AND payload #>> '{signal,ownerIdentity}' = owner_identity
        AND payload #>> '{signal,id}' = signal_id
        AND abs(extract(epoch FROM (
            (payload #>> '{recordedAt}')::timestamptz - recorded_at
        ))) < 0.001
    )
);

CREATE INDEX proactivity_signal_owner_history_idx
    ON public.proactivity_signal_records
    (owner_identity, recorded_at DESC, batch_idempotency_key DESC, ordinal DESC);
CREATE INDEX proactivity_signal_owner_latest_idx
    ON public.proactivity_signal_records
    (owner_identity, signal_id, recorded_at DESC, batch_idempotency_key DESC, ordinal DESC);

CREATE OR REPLACE FUNCTION public.hai_proactivity_decisions_are_advisory(candidate jsonb)
RETURNS boolean
LANGUAGE plpgsql
IMMUTABLE
STRICT
AS $$
DECLARE
    decision jsonb;
BEGIN
    IF jsonb_typeof(candidate) <> 'array' THEN
        RETURN false;
    END IF;
    FOR decision IN SELECT value FROM jsonb_array_elements(candidate)
    LOOP
        IF jsonb_typeof(decision) <> 'object'
            OR NOT (decision ?& ARRAY[
                'contractVersion', 'ownerIdentity', 'signalId', 'openLoopKey',
                'outcome', 'executionAuthorized', 'deliveryAuthorized',
                'authorityGranted', 'decidedAt'
            ])
            OR decision #>> '{contractVersion}' <> '1'
            OR decision #>> '{executionAuthorized}' <> 'false'
            OR decision #>> '{deliveryAuthorized}' <> 'false'
            OR decision #>> '{authorityGranted}' <> 'false'
        THEN
            RETURN false;
        END IF;
    END LOOP;
    RETURN true;
END;
$$;

CREATE TABLE public.proactivity_decision_batches (
    owner_identity text NOT NULL,
    idempotency_key text NOT NULL,
    record_kind text NOT NULL DEFAULT 'decisions' CHECK (record_kind = 'decisions'),
    payload_digest char(64) NOT NULL CHECK (payload_digest ~ '^[a-f0-9]{64}$'),
    decision_count integer NOT NULL CHECK (decision_count BETWEEN 0 AND 256),
    recorded_at timestamptz NOT NULL,
    payload jsonb NOT NULL,
    CONSTRAINT proactivity_decision_batches_pkey
        PRIMARY KEY (owner_identity, idempotency_key),
    CONSTRAINT fk_proactivity_decision_batch_idempotency
        FOREIGN KEY (owner_identity, idempotency_key, record_kind, payload_digest)
        REFERENCES public.proactivity_idempotency
            (owner_identity, idempotency_key, record_kind, payload_digest)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT chk_proactivity_decision_batch_payload CHECK (
        jsonb_typeof(payload) = 'object'
        AND octet_length(payload::text) BETWEEN 2 AND 4194304
        AND payload ?& ARRAY['contractVersion', 'ownerIdentity', 'result', 'recordedAt']
        AND payload #>> '{contractVersion}' = '1'
        AND payload #>> '{ownerIdentity}' = owner_identity
        AND jsonb_typeof(payload -> 'result') = 'object'
        AND payload #>> '{result,contractVersion}' = '1'
        AND payload #>> '{result,ownerIdentity}' = owner_identity
        AND jsonb_typeof(payload #> '{result,decisions}') = 'array'
        AND jsonb_array_length(payload #> '{result,decisions}') = decision_count
        AND public.hai_proactivity_decisions_are_advisory(payload #> '{result,decisions}')
        AND abs(extract(epoch FROM (
            (payload #>> '{recordedAt}')::timestamptz - recorded_at
        ))) < 0.001
    )
);

CREATE TABLE public.proactivity_decision_records (
    owner_identity text NOT NULL,
    batch_idempotency_key text NOT NULL,
    ordinal integer NOT NULL CHECK (ordinal BETWEEN 0 AND 255),
    signal_id text NOT NULL CHECK (length(signal_id) BETWEEN 1 AND 200),
    open_loop_key text NOT NULL CHECK (length(open_loop_key) BETWEEN 1 AND 200),
    outcome text NOT NULL CHECK (outcome IN ('suppress', 'ambient', 'daily_brief', 'notify', 'require_review')),
    recorded_at timestamptz NOT NULL,
    payload jsonb NOT NULL,
    CONSTRAINT proactivity_decision_records_pkey
        PRIMARY KEY (owner_identity, batch_idempotency_key, ordinal),
    CONSTRAINT uq_proactivity_decision_batch_signal
        UNIQUE (owner_identity, batch_idempotency_key, signal_id),
    CONSTRAINT fk_proactivity_decision_batch
        FOREIGN KEY (owner_identity, batch_idempotency_key)
        REFERENCES public.proactivity_decision_batches (owner_identity, idempotency_key)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT chk_proactivity_decision_payload CHECK (
        jsonb_typeof(payload) = 'object'
        AND octet_length(payload::text) BETWEEN 2 AND 262144
        AND payload ?& ARRAY['contractVersion', 'ownerIdentity', 'decision', 'recordedAt']
        AND payload #>> '{contractVersion}' = '1'
        AND payload #>> '{ownerIdentity}' = owner_identity
        AND jsonb_typeof(payload -> 'decision') = 'object'
        AND payload #>> '{decision,contractVersion}' = '1'
        AND payload #>> '{decision,ownerIdentity}' = owner_identity
        AND payload #>> '{decision,signalId}' = signal_id
        AND payload #>> '{decision,openLoopKey}' = open_loop_key
        AND payload #>> '{decision,outcome}' = outcome
        AND payload #>> '{decision,executionAuthorized}' = 'false'
        AND payload #>> '{decision,deliveryAuthorized}' = 'false'
        AND payload #>> '{decision,authorityGranted}' = 'false'
        AND abs(extract(epoch FROM (
            (payload #>> '{recordedAt}')::timestamptz - recorded_at
        ))) < 0.001
    )
);

CREATE INDEX proactivity_decision_owner_history_idx
    ON public.proactivity_decision_records
    (owner_identity, recorded_at DESC, batch_idempotency_key DESC, ordinal DESC);

CREATE OR REPLACE FUNCTION public.hai_validate_proactivity_signal_batch()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    scoped_owner text;
    scoped_key text;
    expected_count integer;
    actual_count integer;
    batch_payload jsonb;
BEGIN
    scoped_owner := NEW.owner_identity;
    IF TG_TABLE_NAME = 'proactivity_signal_batches' THEN
        scoped_key := NEW.idempotency_key;
    ELSE
        scoped_key := NEW.batch_idempotency_key;
    END IF;

    SELECT signal_count, payload
      INTO expected_count, batch_payload
      FROM public.proactivity_signal_batches
     WHERE owner_identity = scoped_owner AND idempotency_key = scoped_key;
    IF NOT FOUND THEN
        RAISE EXCEPTION 'proactivity signal batch is missing'
            USING ERRCODE = 'foreign_key_violation';
    END IF;

    SELECT count(*) INTO actual_count
      FROM public.proactivity_signal_records
     WHERE owner_identity = scoped_owner AND batch_idempotency_key = scoped_key;
    IF actual_count <> expected_count THEN
        RAISE EXCEPTION 'proactivity signal batch child count is inconsistent'
            USING ERRCODE = 'integrity_constraint_violation';
    END IF;
    IF EXISTS (
        SELECT 1
          FROM public.proactivity_signal_records child
         WHERE child.owner_identity = scoped_owner
           AND child.batch_idempotency_key = scoped_key
           AND (
               child.ordinal >= expected_count
               OR batch_payload -> child.ordinal <> child.payload
               OR child.signal_id <> child.payload #>> '{signal,id}'
               OR abs(extract(epoch FROM (
                    (child.payload #>> '{recordedAt}')::timestamptz - child.recorded_at
               ))) >= 0.001
           )
    ) THEN
        RAISE EXCEPTION 'proactivity signal batch child payload is inconsistent'
            USING ERRCODE = 'integrity_constraint_violation';
    END IF;
    RETURN NULL;
END;
$$;

CREATE OR REPLACE FUNCTION public.hai_validate_proactivity_decision_batch()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    scoped_owner text;
    scoped_key text;
    expected_count integer;
    actual_count integer;
    batch_payload jsonb;
    batch_recorded_at timestamptz;
BEGIN
    scoped_owner := NEW.owner_identity;
    IF TG_TABLE_NAME = 'proactivity_decision_batches' THEN
        scoped_key := NEW.idempotency_key;
    ELSE
        scoped_key := NEW.batch_idempotency_key;
    END IF;

    SELECT decision_count, payload, recorded_at
      INTO expected_count, batch_payload, batch_recorded_at
      FROM public.proactivity_decision_batches
     WHERE owner_identity = scoped_owner AND idempotency_key = scoped_key;
    IF NOT FOUND THEN
        RAISE EXCEPTION 'proactivity decision batch is missing'
            USING ERRCODE = 'foreign_key_violation';
    END IF;

    SELECT count(*) INTO actual_count
      FROM public.proactivity_decision_records
     WHERE owner_identity = scoped_owner AND batch_idempotency_key = scoped_key;
    IF actual_count <> expected_count THEN
        RAISE EXCEPTION 'proactivity decision batch child count is inconsistent'
            USING ERRCODE = 'integrity_constraint_violation';
    END IF;
    IF EXISTS (
        SELECT 1
          FROM public.proactivity_decision_records child
         WHERE child.owner_identity = scoped_owner
           AND child.batch_idempotency_key = scoped_key
           AND (
               child.ordinal >= expected_count
               OR batch_payload #> ARRAY['result', 'decisions', child.ordinal::text]
                    <> child.payload -> 'decision'
               OR abs(extract(epoch FROM (
                    child.recorded_at - batch_recorded_at
               ))) >= 0.001
               OR child.signal_id <> child.payload #>> '{decision,signalId}'
               OR child.open_loop_key <> child.payload #>> '{decision,openLoopKey}'
               OR child.outcome <> child.payload #>> '{decision,outcome}'
           )
    ) THEN
        RAISE EXCEPTION 'proactivity decision batch child payload is inconsistent'
            USING ERRCODE = 'integrity_constraint_violation';
    END IF;
    RETURN NULL;
END;
$$;

CREATE CONSTRAINT TRIGGER trg_proactivity_signal_batches_consistent
AFTER INSERT ON public.proactivity_signal_batches
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION public.hai_validate_proactivity_signal_batch();
CREATE CONSTRAINT TRIGGER trg_proactivity_signal_records_consistent
AFTER INSERT ON public.proactivity_signal_records
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION public.hai_validate_proactivity_signal_batch();
CREATE CONSTRAINT TRIGGER trg_proactivity_decision_batches_consistent
AFTER INSERT ON public.proactivity_decision_batches
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION public.hai_validate_proactivity_decision_batch();
CREATE CONSTRAINT TRIGGER trg_proactivity_decision_records_consistent
AFTER INSERT ON public.proactivity_decision_records
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION public.hai_validate_proactivity_decision_batch();

CREATE OR REPLACE FUNCTION public.hai_reject_proactivity_mutation()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE EXCEPTION 'proactivity advisory records are append-only'
        USING ERRCODE = 'integrity_constraint_violation';
END;
$$;

CREATE TRIGGER trg_proactivity_idempotency_immutable
BEFORE UPDATE OR DELETE ON public.proactivity_idempotency
FOR EACH ROW EXECUTE FUNCTION public.hai_reject_proactivity_mutation();
CREATE TRIGGER trg_proactivity_policy_records_immutable
BEFORE UPDATE OR DELETE ON public.proactivity_policy_records
FOR EACH ROW EXECUTE FUNCTION public.hai_reject_proactivity_mutation();
CREATE TRIGGER trg_proactivity_signal_batches_immutable
BEFORE UPDATE OR DELETE ON public.proactivity_signal_batches
FOR EACH ROW EXECUTE FUNCTION public.hai_reject_proactivity_mutation();
CREATE TRIGGER trg_proactivity_signal_records_immutable
BEFORE UPDATE OR DELETE ON public.proactivity_signal_records
FOR EACH ROW EXECUTE FUNCTION public.hai_reject_proactivity_mutation();
CREATE TRIGGER trg_proactivity_decision_batches_immutable
BEFORE UPDATE OR DELETE ON public.proactivity_decision_batches
FOR EACH ROW EXECUTE FUNCTION public.hai_reject_proactivity_mutation();
CREATE TRIGGER trg_proactivity_decision_records_immutable
BEFORE UPDATE OR DELETE ON public.proactivity_decision_records
FOR EACH ROW EXECUTE FUNCTION public.hai_reject_proactivity_mutation();

CREATE TRIGGER trg_proactivity_idempotency_no_truncate
BEFORE TRUNCATE ON public.proactivity_idempotency
FOR EACH STATEMENT EXECUTE FUNCTION public.hai_reject_proactivity_mutation();
CREATE TRIGGER trg_proactivity_policy_records_no_truncate
BEFORE TRUNCATE ON public.proactivity_policy_records
FOR EACH STATEMENT EXECUTE FUNCTION public.hai_reject_proactivity_mutation();
CREATE TRIGGER trg_proactivity_signal_batches_no_truncate
BEFORE TRUNCATE ON public.proactivity_signal_batches
FOR EACH STATEMENT EXECUTE FUNCTION public.hai_reject_proactivity_mutation();
CREATE TRIGGER trg_proactivity_signal_records_no_truncate
BEFORE TRUNCATE ON public.proactivity_signal_records
FOR EACH STATEMENT EXECUTE FUNCTION public.hai_reject_proactivity_mutation();
CREATE TRIGGER trg_proactivity_decision_batches_no_truncate
BEFORE TRUNCATE ON public.proactivity_decision_batches
FOR EACH STATEMENT EXECUTE FUNCTION public.hai_reject_proactivity_mutation();
CREATE TRIGGER trg_proactivity_decision_records_no_truncate
BEFORE TRUNCATE ON public.proactivity_decision_records
FOR EACH STATEMENT EXECUTE FUNCTION public.hai_reject_proactivity_mutation();
