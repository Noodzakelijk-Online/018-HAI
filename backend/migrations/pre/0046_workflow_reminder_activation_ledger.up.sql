CREATE TABLE public.workflow_reminder_activation_requests (
    id uuid PRIMARY KEY,
    owner_identity character varying(255) NOT NULL,
    workflow_id uuid NOT NULL,
    checklist_item_id uuid NOT NULL,
    activation_kind character varying(50) NOT NULL,
    workflow_state character varying(80) NOT NULL,
    checklist_status character varying(50) NOT NULL,
    reminder_at timestamp with time zone NOT NULL,
    due_at timestamp with time zone,
    reminder_digest character(64) NOT NULL,
    idempotency_key character varying(160) NOT NULL,
    authority character varying(80) NOT NULL,
    actor character varying(255) NOT NULL,
    confirmation character varying(120) NOT NULL,
    request_digest character(64) NOT NULL,
    record_digest character(64) NOT NULL,
    requested_at timestamp with time zone NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    CONSTRAINT uq_workflow_reminder_activation_request_owner_key
        UNIQUE (owner_identity, idempotency_key),
    CONSTRAINT uq_workflow_reminder_activation_request_owner_id_digest
        UNIQUE (owner_identity, id, record_digest),
    CONSTRAINT fk_workflow_reminder_activation_request_workflow
        FOREIGN KEY (workflow_id) REFERENCES public.workflow_items (id)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT fk_workflow_reminder_activation_request_checklist
        FOREIGN KEY (checklist_item_id) REFERENCES public.workflow_checklist_items (id)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT chk_workflow_reminder_activation_request_identity CHECK (
        char_length(btrim(owner_identity)) BETWEEN 1 AND 255
        AND actor = owner_identity
        AND idempotency_key ~ '^[A-Za-z0-9._:-]{1,160}$'
    ),
    CONSTRAINT chk_workflow_reminder_activation_request_scope CHECK (
        activation_kind = 'internal_notification'
        AND authority = 'reminder_activation_request_only'
        AND confirmation = 'PREPARE INTERNAL REMINDER ONLY'
        AND checklist_status = 'open'
        AND workflow_state NOT IN ('completed', 'archived')
    ),
    CONSTRAINT chk_workflow_reminder_activation_request_digests CHECK (
        reminder_digest ~ '^[0-9a-f]{64}$'
        AND request_digest ~ '^[0-9a-f]{64}$'
        AND record_digest ~ '^[0-9a-f]{64}$'
    ),
    CONSTRAINT chk_workflow_reminder_activation_request_expiry CHECK (
        requested_at < expires_at
        AND expires_at <= requested_at + interval '30 minutes'
    )
);

CREATE INDEX idx_workflow_reminder_activation_requests_owner_time
    ON public.workflow_reminder_activation_requests
    (owner_identity, requested_at DESC, id DESC);
CREATE INDEX idx_workflow_reminder_activation_requests_source
    ON public.workflow_reminder_activation_requests
    (owner_identity, checklist_item_id, requested_at DESC, id DESC);

CREATE TABLE public.workflow_reminder_activation_decisions (
    id uuid PRIMARY KEY,
    activation_request_id uuid NOT NULL,
    owner_identity character varying(255) NOT NULL,
    decision character varying(40) NOT NULL,
    reason text NOT NULL,
    actor character varying(255) NOT NULL,
    confirmation character varying(120) NOT NULL,
    activation_request_digest character(64) NOT NULL,
    previous_decision_id uuid,
    authority character varying(80) NOT NULL,
    request_digest character(64) NOT NULL,
    record_digest character(64) NOT NULL,
    decided_at timestamp with time zone NOT NULL,
    expires_at timestamp with time zone,
    CONSTRAINT uq_workflow_reminder_activation_decision_owner_request_digest
        UNIQUE (owner_identity, activation_request_id, request_digest),
    CONSTRAINT uq_workflow_reminder_activation_decision_owner_id_digest
        UNIQUE (owner_identity, id, record_digest),
    CONSTRAINT fk_workflow_reminder_activation_decision_request
        FOREIGN KEY (owner_identity, activation_request_id, activation_request_digest)
        REFERENCES public.workflow_reminder_activation_requests
            (owner_identity, id, record_digest)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT fk_workflow_reminder_activation_decision_previous
        FOREIGN KEY (previous_decision_id)
        REFERENCES public.workflow_reminder_activation_decisions (id)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT chk_workflow_reminder_activation_decision_identity CHECK (
        char_length(btrim(owner_identity)) BETWEEN 1 AND 255
        AND actor = owner_identity
        AND char_length(btrim(reason)) BETWEEN 1 AND 2000
    ),
    CONSTRAINT chk_workflow_reminder_activation_decision_contract CHECK (
        decision IN ('approved', 'rejected', 'needs_clarification', 'revoked')
        AND authority = 'reminder_activation_decision_only'
        AND confirmation = CASE decision
            WHEN 'approved' THEN 'APPROVE INTERNAL REMINDER PREPARATION'
            WHEN 'rejected' THEN 'REJECT INTERNAL REMINDER PREPARATION'
            WHEN 'needs_clarification' THEN 'REQUEST REMINDER CLARIFICATION'
            WHEN 'revoked' THEN 'REVOKE INTERNAL REMINDER PREPARATION'
        END
    ),
    CONSTRAINT chk_workflow_reminder_activation_decision_digests CHECK (
        activation_request_digest ~ '^[0-9a-f]{64}$'
        AND request_digest ~ '^[0-9a-f]{64}$'
        AND record_digest ~ '^[0-9a-f]{64}$'
    ),
    CONSTRAINT chk_workflow_reminder_activation_decision_expiry CHECK (
        (
            decision = 'approved'
            AND expires_at IS NOT NULL
            AND decided_at < expires_at
            AND expires_at <= decided_at + interval '15 minutes'
        )
        OR (decision <> 'approved' AND expires_at IS NULL)
    )
);

CREATE INDEX idx_workflow_reminder_activation_decisions_owner_request_time
    ON public.workflow_reminder_activation_decisions
    (owner_identity, activation_request_id, decided_at DESC, id DESC);
CREATE UNIQUE INDEX uq_workflow_reminder_activation_decision_chain_link
    ON public.workflow_reminder_activation_decisions
    (owner_identity, activation_request_id, previous_decision_id)
    WHERE previous_decision_id IS NOT NULL;

CREATE OR REPLACE FUNCTION public.validate_workflow_reminder_activation_request_insert()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    workflow_record public.workflow_items%ROWTYPE;
    checklist_record public.workflow_checklist_items%ROWTYPE;
BEGIN
    SELECT * INTO workflow_record
    FROM public.workflow_items
    WHERE id = NEW.workflow_id
    FOR KEY SHARE;
    SELECT * INTO checklist_record
    FROM public.workflow_checklist_items
    WHERE id = NEW.checklist_item_id
    FOR KEY SHARE;

    IF NOT FOUND
       OR workflow_record.id IS NULL
       OR workflow_record.owner_identity IS NULL
       OR btrim(workflow_record.owner_identity) = ''
       OR workflow_record.owner_identity <> NEW.owner_identity
       OR workflow_record.archived
       OR workflow_record.current_state IN ('completed', 'archived')
       OR workflow_record.current_state <> NEW.workflow_state
       OR checklist_record.workflow_id <> NEW.workflow_id
       OR checklist_record.status <> 'open'
       OR checklist_record.status <> NEW.checklist_status
       OR checklist_record.reminder_at IS NULL
       OR checklist_record.reminder_at <> NEW.reminder_at
       OR checklist_record.due_at IS DISTINCT FROM NEW.due_at THEN
        RAISE EXCEPTION 'reminder activation request does not match a current owner-scoped reminder';
    END IF;
    RETURN NEW;
END;
$$;

CREATE OR REPLACE FUNCTION public.validate_workflow_reminder_activation_decision_insert()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    request_record public.workflow_reminder_activation_requests%ROWTYPE;
    previous_record public.workflow_reminder_activation_decisions%ROWTYPE;
    latest_record public.workflow_reminder_activation_decisions%ROWTYPE;
BEGIN
    SELECT * INTO request_record
    FROM public.workflow_reminder_activation_requests
    WHERE id = NEW.activation_request_id
    FOR UPDATE;
    IF NOT FOUND
       OR request_record.owner_identity <> NEW.owner_identity
       OR request_record.record_digest <> NEW.activation_request_digest THEN
        RAISE EXCEPTION 'reminder activation decision does not match its immutable request';
    END IF;

    IF NEW.previous_decision_id IS NOT NULL THEN
        SELECT * INTO previous_record
        FROM public.workflow_reminder_activation_decisions
        WHERE id = NEW.previous_decision_id
        FOR KEY SHARE;
        IF NOT FOUND
           OR previous_record.activation_request_id <> NEW.activation_request_id
           OR previous_record.owner_identity <> NEW.owner_identity THEN
            RAISE EXCEPTION 'reminder activation decision chain is invalid';
        END IF;
    END IF;

    SELECT * INTO latest_record
    FROM public.workflow_reminder_activation_decisions
    WHERE activation_request_id = NEW.activation_request_id
      AND owner_identity = NEW.owner_identity
    ORDER BY decided_at DESC, id DESC
    LIMIT 1;

    IF latest_record.id IS NULL AND NEW.previous_decision_id IS NOT NULL THEN
        RAISE EXCEPTION 'first reminder activation decision cannot reference prior history';
    ELSIF latest_record.id IS NOT NULL
       AND NEW.previous_decision_id IS DISTINCT FROM latest_record.id THEN
        RAISE EXCEPTION 'reminder activation decision must extend the current chain tip';
    END IF;
    IF NEW.decision = 'revoked'
       AND (latest_record.id IS NULL OR latest_record.decision <> 'approved') THEN
        RAISE EXCEPTION 'only the latest approved reminder preparation can be revoked';
    END IF;
    RETURN NEW;
END;
$$;

CREATE OR REPLACE FUNCTION public.reject_workflow_reminder_activation_mutation()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE EXCEPTION 'workflow reminder activation ledgers are append-only'
        USING ERRCODE = '55000';
END;
$$;

CREATE TRIGGER workflow_reminder_activation_requests_validate_insert
    BEFORE INSERT ON public.workflow_reminder_activation_requests
    FOR EACH ROW EXECUTE FUNCTION public.validate_workflow_reminder_activation_request_insert();
CREATE TRIGGER workflow_reminder_activation_decisions_validate_insert
    BEFORE INSERT ON public.workflow_reminder_activation_decisions
    FOR EACH ROW EXECUTE FUNCTION public.validate_workflow_reminder_activation_decision_insert();

CREATE TRIGGER workflow_reminder_activation_requests_reject_update
    BEFORE UPDATE ON public.workflow_reminder_activation_requests
    FOR EACH ROW EXECUTE FUNCTION public.reject_workflow_reminder_activation_mutation();
CREATE TRIGGER workflow_reminder_activation_requests_reject_delete
    BEFORE DELETE ON public.workflow_reminder_activation_requests
    FOR EACH ROW EXECUTE FUNCTION public.reject_workflow_reminder_activation_mutation();
CREATE TRIGGER workflow_reminder_activation_requests_reject_truncate
    BEFORE TRUNCATE ON public.workflow_reminder_activation_requests
    FOR EACH STATEMENT EXECUTE FUNCTION public.reject_workflow_reminder_activation_mutation();
CREATE TRIGGER workflow_reminder_activation_decisions_reject_update
    BEFORE UPDATE ON public.workflow_reminder_activation_decisions
    FOR EACH ROW EXECUTE FUNCTION public.reject_workflow_reminder_activation_mutation();
CREATE TRIGGER workflow_reminder_activation_decisions_reject_delete
    BEFORE DELETE ON public.workflow_reminder_activation_decisions
    FOR EACH ROW EXECUTE FUNCTION public.reject_workflow_reminder_activation_mutation();
CREATE TRIGGER workflow_reminder_activation_decisions_reject_truncate
    BEFORE TRUNCATE ON public.workflow_reminder_activation_decisions
    FOR EACH STATEMENT EXECUTE FUNCTION public.reject_workflow_reminder_activation_mutation();
