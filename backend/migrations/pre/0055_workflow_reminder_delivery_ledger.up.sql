CREATE TABLE public.workflow_reminder_delivery_authorizations (
    id uuid PRIMARY KEY,
    activation_request_id uuid NOT NULL,
    activation_decision_id uuid NOT NULL,
    owner_identity character varying(255) NOT NULL,
    workflow_id uuid NOT NULL,
    checklist_item_id uuid NOT NULL,
    reminder_at timestamp with time zone NOT NULL,
    reminder_digest character(64) NOT NULL,
    activation_request_digest character(64) NOT NULL,
    activation_decision_digest character(64) NOT NULL,
    channel character varying(40) NOT NULL,
    idempotency_key character varying(160) NOT NULL,
    authority character varying(80) NOT NULL,
    actor character varying(255) NOT NULL,
    confirmation character varying(120) NOT NULL,
    request_digest character(64) NOT NULL,
    record_digest character(64) NOT NULL,
    authorized_at timestamp with time zone NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    CONSTRAINT uq_workflow_reminder_delivery_owner_key UNIQUE (owner_identity, idempotency_key),
    CONSTRAINT uq_workflow_reminder_delivery_owner_id_digest UNIQUE (owner_identity, id, record_digest),
    CONSTRAINT fk_workflow_reminder_delivery_request
        FOREIGN KEY (owner_identity, activation_request_id, activation_request_digest)
        REFERENCES public.workflow_reminder_activation_requests (owner_identity, id, record_digest)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT fk_workflow_reminder_delivery_decision
        FOREIGN KEY (owner_identity, activation_decision_id, activation_decision_digest)
        REFERENCES public.workflow_reminder_activation_decisions (owner_identity, id, record_digest)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT chk_workflow_reminder_delivery_scope CHECK (
        actor = owner_identity
        AND channel = 'in_app'
        AND authority = 'internal_reminder_delivery_authorization'
        AND confirmation = 'AUTHORIZE ONE INTERNAL HAI REMINDER'
        AND idempotency_key ~ '^[A-Za-z0-9._:-]{1,160}$'
        AND reminder_digest ~ '^[0-9a-f]{64}$'
        AND request_digest ~ '^[0-9a-f]{64}$'
        AND record_digest ~ '^[0-9a-f]{64}$'
        AND authorized_at < expires_at
        AND expires_at <= authorized_at + interval '30 days'
    )
);

CREATE INDEX idx_workflow_reminder_delivery_due
    ON public.workflow_reminder_delivery_authorizations (reminder_at, expires_at, id);

CREATE TABLE public.workflow_reminder_delivery_attempts (
    id uuid PRIMARY KEY,
    authorization_id uuid NOT NULL,
    owner_identity character varying(255) NOT NULL,
    attempt_number integer NOT NULL,
    status character varying(40) NOT NULL,
    reason text NOT NULL,
    reminder_digest character(64) NOT NULL,
    authorization_digest character(64) NOT NULL,
    authority character varying(80) NOT NULL,
    record_digest character(64) NOT NULL,
    attempted_at timestamp with time zone NOT NULL,
    CONSTRAINT uq_workflow_reminder_delivery_attempt UNIQUE (authorization_id, attempt_number),
    CONSTRAINT fk_workflow_reminder_delivery_attempt_authorization
        FOREIGN KEY (owner_identity, authorization_id, authorization_digest)
        REFERENCES public.workflow_reminder_delivery_authorizations (owner_identity, id, record_digest)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT chk_workflow_reminder_delivery_attempt CHECK (
        attempt_number BETWEEN 1 AND 3
        AND status IN ('delivered', 'retryable_failure', 'suppressed')
        AND char_length(btrim(reason)) BETWEEN 1 AND 1000
        AND authority = 'internal_reminder_delivery_receipt'
        AND reminder_digest ~ '^[0-9a-f]{64}$'
        AND record_digest ~ '^[0-9a-f]{64}$'
    )
);

CREATE INDEX idx_workflow_reminder_delivery_attempt_owner_time
    ON public.workflow_reminder_delivery_attempts (owner_identity, attempted_at DESC, id DESC);

CREATE OR REPLACE FUNCTION public.validate_workflow_reminder_delivery_authorization_insert()
RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    request_record public.workflow_reminder_activation_requests%ROWTYPE;
    decision_record public.workflow_reminder_activation_decisions%ROWTYPE;
    latest_record public.workflow_reminder_activation_decisions%ROWTYPE;
BEGIN
    SELECT * INTO request_record FROM public.workflow_reminder_activation_requests
      WHERE id = NEW.activation_request_id FOR KEY SHARE;
    SELECT * INTO decision_record FROM public.workflow_reminder_activation_decisions
      WHERE id = NEW.activation_decision_id FOR KEY SHARE;
    SELECT * INTO latest_record FROM public.workflow_reminder_activation_decisions
      WHERE activation_request_id = NEW.activation_request_id AND owner_identity = NEW.owner_identity
      ORDER BY decided_at DESC, id DESC LIMIT 1;
    IF request_record.id IS NULL OR decision_record.id IS NULL OR latest_record.id IS NULL
       OR request_record.owner_identity <> NEW.owner_identity
       OR request_record.workflow_id <> NEW.workflow_id
       OR request_record.checklist_item_id <> NEW.checklist_item_id
       OR request_record.reminder_at <> NEW.reminder_at
       OR request_record.reminder_digest <> NEW.reminder_digest
       OR decision_record.activation_request_id <> request_record.id
       OR decision_record.id <> latest_record.id
       OR decision_record.decision <> 'approved'
       OR decision_record.expires_at IS NULL
       OR NEW.authorized_at > decision_record.expires_at THEN
        RAISE EXCEPTION 'reminder delivery authorization requires the current exact approved preparation';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER workflow_reminder_delivery_authorizations_validate_insert
    BEFORE INSERT ON public.workflow_reminder_delivery_authorizations
    FOR EACH ROW EXECUTE FUNCTION public.validate_workflow_reminder_delivery_authorization_insert();
CREATE TRIGGER workflow_reminder_delivery_authorizations_reject_update
    BEFORE UPDATE ON public.workflow_reminder_delivery_authorizations
    FOR EACH ROW EXECUTE FUNCTION public.reject_workflow_reminder_activation_mutation();
CREATE TRIGGER workflow_reminder_delivery_authorizations_reject_delete
    BEFORE DELETE ON public.workflow_reminder_delivery_authorizations
    FOR EACH ROW EXECUTE FUNCTION public.reject_workflow_reminder_activation_mutation();
CREATE TRIGGER workflow_reminder_delivery_authorizations_reject_truncate
    BEFORE TRUNCATE ON public.workflow_reminder_delivery_authorizations
    FOR EACH STATEMENT EXECUTE FUNCTION public.reject_workflow_reminder_activation_mutation();
CREATE TRIGGER workflow_reminder_delivery_attempts_reject_update
    BEFORE UPDATE ON public.workflow_reminder_delivery_attempts
    FOR EACH ROW EXECUTE FUNCTION public.reject_workflow_reminder_activation_mutation();
CREATE TRIGGER workflow_reminder_delivery_attempts_reject_delete
    BEFORE DELETE ON public.workflow_reminder_delivery_attempts
    FOR EACH ROW EXECUTE FUNCTION public.reject_workflow_reminder_activation_mutation();
CREATE TRIGGER workflow_reminder_delivery_attempts_reject_truncate
    BEFORE TRUNCATE ON public.workflow_reminder_delivery_attempts
    FOR EACH STATEMENT EXECUTE FUNCTION public.reject_workflow_reminder_activation_mutation();
