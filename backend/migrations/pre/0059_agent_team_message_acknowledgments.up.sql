CREATE TABLE public.agent_team_message_acknowledgments (
    owner_identity character varying(255) NOT NULL,
    team_id uuid NOT NULL,
    team_version character varying(64) NOT NULL,
    acknowledgment_id uuid NOT NULL,
    message_id uuid NOT NULL,
    correlation_id uuid NOT NULL,
    recipient_id character varying(255) NOT NULL,
    status character varying(32) NOT NULL,
    idempotency_key uuid NOT NULL,
    acknowledgment_digest character(64) NOT NULL,
    created_at timestamp with time zone NOT NULL,
    retry_after timestamp with time zone,
    payload jsonb NOT NULL,
    CONSTRAINT agent_team_message_acknowledgments_pkey
        PRIMARY KEY (owner_identity, team_id, team_version, acknowledgment_id),
    CONSTRAINT uq_agent_team_message_acknowledgment_idempotency
        UNIQUE (owner_identity, team_id, team_version, idempotency_key),
    CONSTRAINT fk_agent_team_message_acknowledgment_message
        FOREIGN KEY (owner_identity, team_id, team_version, message_id)
        REFERENCES public.agent_team_coordination_messages
            (owner_identity, team_id, team_version, message_id)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT chk_agent_team_message_acknowledgment_status CHECK (
        status IN ('accepted', 'rejected', 'deferred')
    ),
    CONSTRAINT chk_agent_team_message_acknowledgment_digest CHECK (
        acknowledgment_digest ~ '^[0-9a-f]{64}$'
    ),
    CONSTRAINT chk_agent_team_message_acknowledgment_retry CHECK (
        (status = 'deferred' AND retry_after IS NOT NULL AND retry_after > created_at)
        OR (status <> 'deferred')
    ),
    CONSTRAINT chk_agent_team_message_acknowledgment_payload CHECK (
        jsonb_typeof(payload) = 'object'
        AND octet_length(payload::text) BETWEEN 2 AND 65536
        AND payload ?& ARRAY[
            'id', 'messageId', 'correlationId', 'recipientId',
            'status', 'createdAt', 'idempotencyKey'
        ]
        AND payload #>> '{id}' = acknowledgment_id::text
        AND payload #>> '{messageId}' = message_id::text
        AND payload #>> '{correlationId}' = correlation_id::text
        AND payload #>> '{recipientId}' = recipient_id
        AND payload #>> '{status}' = status
        AND payload #>> '{idempotencyKey}' = idempotency_key::text
    )
);

CREATE INDEX idx_agent_team_message_acknowledgments_stream
    ON public.agent_team_message_acknowledgments
    (owner_identity, team_id, team_version, message_id, created_at ASC, acknowledgment_id ASC);

CREATE OR REPLACE FUNCTION public.validate_agent_team_message_acknowledgment_insert()
RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    message_record public.agent_team_coordination_messages%ROWTYPE;
    previous_record public.agent_team_message_acknowledgments%ROWTYPE;
BEGIN
    SELECT * INTO message_record
      FROM public.agent_team_coordination_messages
      WHERE owner_identity = NEW.owner_identity
        AND team_id = NEW.team_id
        AND team_version = NEW.team_version
        AND message_id = NEW.message_id
      FOR KEY SHARE;
    IF message_record.message_id IS NULL
       OR message_record.correlation_id <> NEW.correlation_id
       OR message_record.payload #>> '{recipient,id}' <> NEW.recipient_id
       OR message_record.payload #>> '{requiresAck}' <> 'true'
       OR NEW.created_at < message_record.created_at THEN
        RAISE EXCEPTION 'acknowledgment must bind the exact stored acknowledgment-required message and recipient';
    END IF;

    SELECT * INTO previous_record
      FROM public.agent_team_message_acknowledgments
      WHERE owner_identity = NEW.owner_identity
        AND team_id = NEW.team_id
        AND team_version = NEW.team_version
        AND message_id = NEW.message_id
      ORDER BY created_at DESC, acknowledgment_id DESC
      LIMIT 1
      FOR UPDATE;
    IF previous_record.acknowledgment_id IS NOT NULL
       AND previous_record.status IN ('accepted', 'rejected') THEN
        RAISE EXCEPTION 'terminal message acknowledgment cannot be superseded';
    END IF;
    IF previous_record.acknowledgment_id IS NOT NULL
       AND NEW.created_at <= previous_record.created_at THEN
        RAISE EXCEPTION 'message acknowledgment time must advance';
    END IF;
    RETURN NEW;
END;
$$;

CREATE OR REPLACE FUNCTION public.reject_agent_team_message_acknowledgment_mutation()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'agent team message acknowledgments are append-only';
END;
$$;

CREATE TRIGGER agent_team_message_acknowledgments_validate_insert
    BEFORE INSERT ON public.agent_team_message_acknowledgments
    FOR EACH ROW EXECUTE FUNCTION public.validate_agent_team_message_acknowledgment_insert();
CREATE TRIGGER agent_team_message_acknowledgments_reject_update
    BEFORE UPDATE ON public.agent_team_message_acknowledgments
    FOR EACH ROW EXECUTE FUNCTION public.reject_agent_team_message_acknowledgment_mutation();
CREATE TRIGGER agent_team_message_acknowledgments_reject_delete
    BEFORE DELETE ON public.agent_team_message_acknowledgments
    FOR EACH ROW EXECUTE FUNCTION public.reject_agent_team_message_acknowledgment_mutation();
CREATE TRIGGER agent_team_message_acknowledgments_reject_truncate
    BEFORE TRUNCATE ON public.agent_team_message_acknowledgments
    FOR EACH STATEMENT EXECUTE FUNCTION public.reject_agent_team_message_acknowledgment_mutation();
