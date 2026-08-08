CREATE TABLE public.proactivity_feedback_records (
    owner_identity text NOT NULL CHECK (length(owner_identity) BETWEEN 1 AND 320),
    feedback_id uuid NOT NULL,
    idempotency_key text NOT NULL CHECK (length(idempotency_key) BETWEEN 1 AND 200),
    request_digest char(64) NOT NULL CHECK (request_digest ~ '^[a-f0-9]{64}$'),
    signal_id text NOT NULL CHECK (length(signal_id) BETWEEN 1 AND 200),
    open_loop_key text NOT NULL CHECK (length(open_loop_key) BETWEEN 1 AND 200),
    signal_digest char(64) NOT NULL CHECK (signal_digest ~ '^[a-f0-9]{64}$'),
    source_outcome text NOT NULL CHECK (source_outcome IN ('ambient', 'daily_brief', 'notify', 'require_review')),
    source_decision_at timestamptz NOT NULL,
    action text NOT NULL CHECK (action IN ('accept', 'dismiss', 'snooze', 'suppress', 'resume')),
    snoozed_until timestamptz,
    previous_record_digest char(64) CHECK (previous_record_digest ~ '^[a-f0-9]{64}$'),
    record_digest char(64) NOT NULL CHECK (record_digest ~ '^[a-f0-9]{64}$'),
    recorded_at timestamptz NOT NULL,
    authority text NOT NULL DEFAULT 'attention_feedback_only' CHECK (authority = 'attention_feedback_only'),
    can_execute boolean NOT NULL DEFAULT false CHECK (can_execute = false),
    delivery_authorized boolean NOT NULL DEFAULT false CHECK (delivery_authorized = false),
    execution_authorized boolean NOT NULL DEFAULT false CHECK (execution_authorized = false),
    payload jsonb NOT NULL CHECK (jsonb_typeof(payload) = 'object'),
    PRIMARY KEY (owner_identity, feedback_id),
    UNIQUE (owner_identity, idempotency_key),
    UNIQUE (owner_identity, open_loop_key, record_digest),
    CHECK ((action = 'snooze' AND snoozed_until IS NOT NULL AND snoozed_until > recorded_at)
        OR (action <> 'snooze' AND snoozed_until IS NULL))
);

CREATE INDEX proactivity_feedback_owner_history_idx
    ON public.proactivity_feedback_records(owner_identity, recorded_at DESC, feedback_id DESC);
CREATE INDEX proactivity_feedback_owner_open_loop_idx
    ON public.proactivity_feedback_records(owner_identity, open_loop_key, recorded_at DESC, feedback_id DESC);

CREATE OR REPLACE FUNCTION public.hai_validate_proactivity_feedback_insert()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    latest_decision record;
    latest_feedback record;
BEGIN
    PERFORM pg_advisory_xact_lock(hashtextextended(NEW.owner_identity || ':' || NEW.open_loop_key, 0));

    SELECT signal_id, open_loop_key, outcome, recorded_at,
           payload #>> '{decision,signalDigest}' AS signal_digest,
           (payload #>> '{decision,decidedAt}')::timestamptz AS decided_at
      INTO latest_decision
      FROM public.proactivity_decision_records
     WHERE owner_identity = NEW.owner_identity
       AND open_loop_key = NEW.open_loop_key
     ORDER BY recorded_at DESC, batch_idempotency_key DESC, ordinal DESC
     LIMIT 1;
    IF NOT FOUND
       OR latest_decision.signal_id <> NEW.signal_id
       OR latest_decision.open_loop_key <> NEW.open_loop_key
       OR latest_decision.outcome <> NEW.source_outcome
       OR latest_decision.outcome = 'suppress'
       OR latest_decision.signal_digest <> NEW.signal_digest
       OR abs(extract(epoch FROM (latest_decision.decided_at - NEW.source_decision_at))) >= 0.000001
    THEN
        RAISE EXCEPTION 'proactivity feedback source decision is stale or unavailable'
            USING ERRCODE = 'foreign_key_violation';
    END IF;

    SELECT record_digest, recorded_at
      INTO latest_feedback
      FROM public.proactivity_feedback_records
     WHERE owner_identity = NEW.owner_identity
       AND open_loop_key = NEW.open_loop_key
     ORDER BY recorded_at DESC, feedback_id DESC
     LIMIT 1;
    IF FOUND THEN
        IF NEW.previous_record_digest IS NULL
           OR NEW.previous_record_digest <> latest_feedback.record_digest
           OR NEW.recorded_at <= latest_feedback.recorded_at
        THEN
            RAISE EXCEPTION 'proactivity feedback chain tip does not match'
                USING ERRCODE = 'serialization_failure';
        END IF;
    ELSIF NEW.previous_record_digest IS NOT NULL THEN
        RAISE EXCEPTION 'first proactivity feedback record cannot name a predecessor'
            USING ERRCODE = 'integrity_constraint_violation';
    END IF;

    IF NEW.payload #>> '{contractVersion}' <> '1'
       OR NEW.payload #>> '{id}' <> NEW.feedback_id::text
       OR NEW.payload #>> '{ownerIdentity}' <> NEW.owner_identity
       OR NEW.payload #>> '{signalId}' <> NEW.signal_id
       OR NEW.payload #>> '{openLoopKey}' <> NEW.open_loop_key
       OR NEW.payload #>> '{signalDigest}' <> NEW.signal_digest
       OR NEW.payload #>> '{sourceOutcome}' <> NEW.source_outcome
       OR abs(extract(epoch FROM (((NEW.payload #>> '{sourceDecisionAt}')::timestamptz) - NEW.source_decision_at))) >= 0.000001
       OR NEW.payload #>> '{action}' <> NEW.action
       OR COALESCE(NEW.payload #>> '{previousRecordDigest}', '') <> COALESCE(NEW.previous_record_digest, '')
       OR NEW.payload #>> '{recordDigest}' <> NEW.record_digest
       OR abs(extract(epoch FROM (((NEW.payload #>> '{recordedAt}')::timestamptz) - NEW.recorded_at))) >= 0.000001
       OR NEW.payload #>> '{authority}' <> 'attention_feedback_only'
       OR NEW.payload #>> '{canExecute}' <> 'false'
       OR NEW.payload #>> '{deliveryAuthorized}' <> 'false'
       OR NEW.payload #>> '{executionAuthorized}' <> 'false'
       OR (NEW.snoozed_until IS NULL AND NEW.payload ? 'snoozedUntil')
       OR (NEW.snoozed_until IS NOT NULL AND (
            NOT (NEW.payload ? 'snoozedUntil')
            OR abs(extract(epoch FROM (((NEW.payload #>> '{snoozedUntil}')::timestamptz) - NEW.snoozed_until))) >= 0.000001
       ))
    THEN
        RAISE EXCEPTION 'proactivity feedback payload is inconsistent'
            USING ERRCODE = 'integrity_constraint_violation';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_proactivity_feedback_validate_insert
BEFORE INSERT ON public.proactivity_feedback_records
FOR EACH ROW EXECUTE FUNCTION public.hai_validate_proactivity_feedback_insert();
CREATE TRIGGER trg_proactivity_feedback_immutable
BEFORE UPDATE OR DELETE ON public.proactivity_feedback_records
FOR EACH ROW EXECUTE FUNCTION public.hai_reject_proactivity_mutation();
CREATE TRIGGER trg_proactivity_feedback_no_truncate
BEFORE TRUNCATE ON public.proactivity_feedback_records
FOR EACH STATEMENT EXECUTE FUNCTION public.hai_reject_proactivity_mutation();
