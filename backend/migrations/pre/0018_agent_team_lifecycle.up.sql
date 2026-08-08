CREATE TABLE public.agent_teams (
    owner_identity character varying(255) NOT NULL,
    team_id uuid NOT NULL,
    team_key character varying(128) NOT NULL,
    created_at timestamp with time zone NOT NULL,
    CONSTRAINT agent_teams_pkey PRIMARY KEY (owner_identity, team_id),
    CONSTRAINT uq_agent_teams_owner_key UNIQUE (owner_identity, team_key),
    CONSTRAINT uq_agent_teams_owner_id_key UNIQUE (owner_identity, team_id, team_key),
    CONSTRAINT chk_agent_teams_identity CHECK (
        char_length(btrim(owner_identity)) BETWEEN 1 AND 255
        AND char_length(btrim(team_key)) BETWEEN 1 AND 128
        AND team_key ~ '^[a-z0-9]+([_-][a-z0-9]+)*$'
    )
);

CREATE TABLE public.agent_team_contracts (
    owner_identity character varying(255) NOT NULL,
    team_id uuid NOT NULL,
    team_key character varying(128) NOT NULL,
    team_version character varying(64) NOT NULL,
    revision bigint NOT NULL,
    team_status character varying(32) NOT NULL,
    contract_digest character varying(64) NOT NULL,
    previous_version_digest character varying(64) NOT NULL DEFAULT '',
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL,
    payload jsonb NOT NULL,
    CONSTRAINT agent_team_contracts_pkey
        PRIMARY KEY (owner_identity, team_id, team_version),
    CONSTRAINT uq_agent_team_contracts_owner_key_version
        UNIQUE (owner_identity, team_key, team_version),
    CONSTRAINT fk_agent_team_contracts_identity
        FOREIGN KEY (owner_identity, team_id, team_key)
        REFERENCES public.agent_teams (owner_identity, team_id, team_key)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT chk_agent_team_contract_version CHECK (
        team_version ~ '^[0-9]+\.[0-9]+\.[0-9]+$'
    ),
    CONSTRAINT chk_agent_team_contract_revision CHECK (revision > 0),
    CONSTRAINT chk_agent_team_contract_status CHECK (
        team_status IN ('draft', 'active', 'suspended', 'retired', 'revoked')
    ),
    CONSTRAINT chk_agent_team_contract_digests CHECK (
        contract_digest ~ '^[0-9a-f]{64}$'
        AND (previous_version_digest = '' OR previous_version_digest ~ '^[0-9a-f]{64}$')
    ),
    CONSTRAINT chk_agent_team_contract_time CHECK (updated_at >= created_at),
    CONSTRAINT chk_agent_team_contract_payload CHECK (
        jsonb_typeof(payload) = 'object'
        AND octet_length(payload::text) BETWEEN 2 AND 1048576
        AND payload ?& ARRAY[
            'id', 'key', 'version', 'revision', 'status', 'contractDigest',
            'advisoryOnly', 'grantsExecutionAuthority',
            'executionAuthorizationRequired', 'createdAt', 'updatedAt'
        ]
        AND payload #>> '{id}' = team_id::text
        AND payload #>> '{key}' = team_key
        AND payload #>> '{version}' = team_version
        AND (payload #>> '{revision}')::bigint = revision
        AND payload #>> '{status}' = team_status
        AND payload #>> '{contractDigest}' = contract_digest
        AND COALESCE(payload #>> '{previousVersionDigest}', '') = previous_version_digest
        AND payload #>> '{advisoryOnly}' = 'true'
        AND payload #>> '{grantsExecutionAuthority}' = 'false'
        AND payload #>> '{executionAuthorizationRequired}' = 'true'
    )
);

CREATE INDEX idx_agent_team_contracts_owner_key
    ON public.agent_team_contracts (owner_identity, team_key, team_version);
CREATE INDEX idx_agent_team_contracts_owner_status
    ON public.agent_team_contracts (owner_identity, team_status, updated_at DESC);

CREATE TABLE public.agent_team_lifecycle_events (
    owner_identity character varying(255) NOT NULL,
    team_id uuid NOT NULL,
    team_version character varying(64) NOT NULL,
    sequence bigint NOT NULL,
    event_id uuid NOT NULL,
    revision bigint NOT NULL,
    event_type character varying(64) NOT NULL,
    event_digest character varying(64) NOT NULL,
    previous_event_digest character varying(64) NOT NULL DEFAULT '',
    occurred_at timestamp with time zone NOT NULL,
    payload jsonb NOT NULL,
    CONSTRAINT agent_team_lifecycle_events_pkey
        PRIMARY KEY (owner_identity, team_id, team_version, sequence),
    CONSTRAINT uq_agent_team_lifecycle_event_id
        UNIQUE (owner_identity, event_id),
    CONSTRAINT fk_agent_team_lifecycle_event_contract
        FOREIGN KEY (owner_identity, team_id, team_version)
        REFERENCES public.agent_team_contracts
            (owner_identity, team_id, team_version)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT chk_agent_team_lifecycle_event_sequence CHECK (
        sequence > 0 AND revision = sequence
    ),
    CONSTRAINT chk_agent_team_lifecycle_event_type CHECK (
        event_type IN (
            'team_created', 'version_created', 'team_activated',
            'team_suspended', 'team_retired', 'team_revoked',
            'member_added', 'membership_changed', 'consensus_recorded'
        )
    ),
    CONSTRAINT chk_agent_team_lifecycle_event_digests CHECK (
        event_digest ~ '^[0-9a-f]{64}$'
        AND (previous_event_digest = '' OR previous_event_digest ~ '^[0-9a-f]{64}$')
    ),
    CONSTRAINT chk_agent_team_lifecycle_event_payload CHECK (
        jsonb_typeof(payload) = 'object'
        AND octet_length(payload::text) BETWEEN 2 AND 262144
        AND payload ?& ARRAY[
            'sequence', 'id', 'teamId', 'teamVersion', 'revision',
            'type', 'actor', 'reason', 'provenanceDigest',
            'eventDigest', 'occurredAt'
        ]
        AND (payload #>> '{sequence}')::bigint = sequence
        AND payload #>> '{id}' = event_id::text
        AND payload #>> '{teamId}' = team_id::text
        AND payload #>> '{teamVersion}' = team_version
        AND (payload #>> '{revision}')::bigint = revision
        AND payload #>> '{type}' = event_type
        AND char_length(btrim(payload #>> '{actor}')) BETWEEN 1 AND 255
        AND char_length(btrim(payload #>> '{reason}')) BETWEEN 1 AND 4096
        AND payload #>> '{provenanceDigest}' ~ '^[0-9a-f]{64}$'
        AND payload #>> '{eventDigest}' = event_digest
        AND COALESCE(payload #>> '{previousEventDigest}', '') = previous_event_digest
    )
);

CREATE INDEX idx_agent_team_lifecycle_events_owner_time
    ON public.agent_team_lifecycle_events
    (owner_identity, team_id, team_version, occurred_at ASC, sequence ASC);

CREATE TABLE public.agent_team_coordination_messages (
    owner_identity character varying(255) NOT NULL,
    team_id uuid NOT NULL,
    team_version character varying(64) NOT NULL,
    message_id uuid NOT NULL,
    idempotency_key uuid NOT NULL,
    correlation_id uuid NOT NULL,
    payload_digest character varying(64) NOT NULL,
    created_at timestamp with time zone NOT NULL,
    payload jsonb NOT NULL,
    CONSTRAINT agent_team_coordination_messages_pkey
        PRIMARY KEY (owner_identity, team_id, team_version, message_id),
    CONSTRAINT uq_agent_team_coordination_message_idempotency
        UNIQUE (owner_identity, team_id, team_version, idempotency_key),
    CONSTRAINT fk_agent_team_coordination_message_contract
        FOREIGN KEY (owner_identity, team_id, team_version)
        REFERENCES public.agent_team_contracts
            (owner_identity, team_id, team_version)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT chk_agent_team_coordination_message_digest CHECK (
        payload_digest ~ '^[0-9a-fA-F]{64}$'
    ),
    CONSTRAINT chk_agent_team_coordination_message_payload CHECK (
        jsonb_typeof(payload) = 'object'
        AND octet_length(payload::text) BETWEEN 2 AND 1048576
        AND payload ?& ARRAY[
            'id', 'idempotencyKey', 'correlationId',
            'payloadDigest', 'createdAt'
        ]
        AND payload #>> '{id}' = message_id::text
        AND payload #>> '{idempotencyKey}' = idempotency_key::text
        AND payload #>> '{correlationId}' = correlation_id::text
        AND payload #>> '{payloadDigest}' = payload_digest
    )
);

CREATE INDEX idx_agent_team_coordination_messages_correlation
    ON public.agent_team_coordination_messages
    (owner_identity, team_id, team_version, correlation_id, created_at ASC, message_id ASC);

CREATE TABLE public.agent_team_consensus_outcomes (
    owner_identity character varying(255) NOT NULL,
    team_id uuid NOT NULL,
    team_version character varying(64) NOT NULL,
    outcome_id uuid NOT NULL,
    idempotency_key uuid NOT NULL,
    correlation_id uuid NOT NULL,
    team_revision bigint NOT NULL,
    outcome_status character varying(64) NOT NULL,
    outcome_digest character varying(64) NOT NULL,
    recorded_at timestamp with time zone NOT NULL,
    payload jsonb NOT NULL,
    CONSTRAINT agent_team_consensus_outcomes_pkey
        PRIMARY KEY (owner_identity, team_id, team_version, outcome_id),
    CONSTRAINT uq_agent_team_consensus_outcome_idempotency
        UNIQUE (owner_identity, team_id, team_version, idempotency_key),
    CONSTRAINT uq_agent_team_consensus_outcome_revision
        UNIQUE (owner_identity, team_id, team_version, team_revision),
    CONSTRAINT fk_agent_team_consensus_outcome_contract
        FOREIGN KEY (owner_identity, team_id, team_version)
        REFERENCES public.agent_team_contracts
            (owner_identity, team_id, team_version)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT chk_agent_team_consensus_outcome_revision CHECK (team_revision > 1),
    CONSTRAINT chk_agent_team_consensus_outcome_status CHECK (
        outcome_status IN (
            'reached', 'conflicted', 'escalated',
            'insufficient_participation'
        )
    ),
    CONSTRAINT chk_agent_team_consensus_outcome_digest CHECK (
        outcome_digest ~ '^[0-9a-f]{64}$'
    ),
    CONSTRAINT chk_agent_team_consensus_outcome_payload CHECK (
        jsonb_typeof(payload) = 'object'
        AND octet_length(payload::text) BETWEEN 2 AND 1048576
        AND payload ?& ARRAY[
            'id', 'teamId', 'teamVersion', 'idempotencyKey',
            'correlationId', 'status', 'outcomeDigest', 'recordedAt',
            'advisoryOnly', 'grantsExecutionAuthority',
            'executionAuthorizationRequired'
        ]
        AND payload #>> '{id}' = outcome_id::text
        AND payload #>> '{teamId}' = team_id::text
        AND payload #>> '{teamVersion}' = team_version
        AND payload #>> '{idempotencyKey}' = idempotency_key::text
        AND payload #>> '{correlationId}' = correlation_id::text
        AND payload #>> '{status}' = outcome_status
        AND payload #>> '{outcomeDigest}' = outcome_digest
        AND payload #>> '{advisoryOnly}' = 'true'
        AND payload #>> '{grantsExecutionAuthority}' = 'false'
        AND payload #>> '{executionAuthorizationRequired}' = 'true'
    )
);

CREATE INDEX idx_agent_team_consensus_outcomes_correlation
    ON public.agent_team_consensus_outcomes
    (owner_identity, team_id, team_version, correlation_id, recorded_at ASC);

CREATE OR REPLACE FUNCTION public.hai_reject_agent_team_append_only_mutation()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE EXCEPTION 'agent team ledger records are append-only'
        USING ERRCODE = 'integrity_constraint_violation';
END;
$$;

CREATE OR REPLACE FUNCTION public.hai_guard_agent_team_contract_revision()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    IF TG_OP = 'INSERT' THEN
        IF NEW.revision <> 1 THEN
            RAISE EXCEPTION 'new agent team contract revision must be 1'
                USING ERRCODE = 'integrity_constraint_violation';
        END IF;
        RETURN NEW;
    END IF;

    IF NEW.owner_identity IS DISTINCT FROM OLD.owner_identity
        OR NEW.team_id IS DISTINCT FROM OLD.team_id
        OR NEW.team_key IS DISTINCT FROM OLD.team_key
        OR NEW.team_version IS DISTINCT FROM OLD.team_version
        OR NEW.created_at IS DISTINCT FROM OLD.created_at THEN
        RAISE EXCEPTION 'agent team contract identity is immutable'
            USING ERRCODE = 'integrity_constraint_violation';
    END IF;
    IF NEW.revision <> OLD.revision + 1
        OR NEW.updated_at < OLD.updated_at
        OR NEW.contract_digest = OLD.contract_digest THEN
        RAISE EXCEPTION 'agent team contract must advance exactly one revision'
            USING ERRCODE = 'integrity_constraint_violation';
    END IF;
    RETURN NEW;
END;
$$;

CREATE OR REPLACE FUNCTION public.hai_require_agent_team_revision_event()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM public.agent_team_lifecycle_events event
        WHERE event.owner_identity = NEW.owner_identity
          AND event.team_id = NEW.team_id
          AND event.team_version = NEW.team_version
          AND event.sequence = NEW.revision
          AND event.revision = NEW.revision
    ) THEN
        RAISE EXCEPTION 'agent team revision requires a matching lifecycle event'
            USING ERRCODE = 'integrity_constraint_violation';
    END IF;
    RETURN NULL;
END;
$$;

CREATE OR REPLACE FUNCTION public.hai_validate_agent_team_event_chain()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    current_revision bigint;
    prior_digest character varying(64);
BEGIN
    SELECT contract.revision
      INTO current_revision
      FROM public.agent_team_contracts contract
     WHERE contract.owner_identity = NEW.owner_identity
       AND contract.team_id = NEW.team_id
       AND contract.team_version = NEW.team_version;

    IF current_revision IS NULL OR current_revision <> NEW.revision
        OR NEW.sequence <> NEW.revision THEN
        RAISE EXCEPTION 'agent team event revision does not match current contract'
            USING ERRCODE = 'integrity_constraint_violation';
    END IF;

    IF NEW.sequence = 1 THEN
        IF NEW.previous_event_digest <> '' THEN
            RAISE EXCEPTION 'first agent team event cannot have a previous digest'
                USING ERRCODE = 'integrity_constraint_violation';
        END IF;
    ELSE
        SELECT event.event_digest
          INTO prior_digest
          FROM public.agent_team_lifecycle_events event
         WHERE event.owner_identity = NEW.owner_identity
           AND event.team_id = NEW.team_id
           AND event.team_version = NEW.team_version
           AND event.sequence = NEW.sequence - 1;
        IF prior_digest IS NULL OR NEW.previous_event_digest <> prior_digest THEN
            RAISE EXCEPTION 'agent team lifecycle event hash chain is invalid'
                USING ERRCODE = 'integrity_constraint_violation';
        END IF;
    END IF;
    RETURN NEW;
END;
$$;

CREATE OR REPLACE FUNCTION public.hai_validate_agent_team_consensus_revision()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    current_revision bigint;
BEGIN
    SELECT contract.revision
      INTO current_revision
      FROM public.agent_team_contracts contract
     WHERE contract.owner_identity = NEW.owner_identity
       AND contract.team_id = NEW.team_id
       AND contract.team_version = NEW.team_version;
    IF current_revision IS NULL OR current_revision <> NEW.team_revision THEN
        RAISE EXCEPTION 'consensus outcome revision does not match current team contract'
            USING ERRCODE = 'integrity_constraint_violation';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_agent_teams_immutable
BEFORE UPDATE OR DELETE ON public.agent_teams
FOR EACH ROW EXECUTE FUNCTION public.hai_reject_agent_team_append_only_mutation();

CREATE TRIGGER trg_agent_team_contracts_revision
BEFORE INSERT OR UPDATE ON public.agent_team_contracts
FOR EACH ROW EXECUTE FUNCTION public.hai_guard_agent_team_contract_revision();

CREATE TRIGGER trg_agent_team_contracts_no_delete
BEFORE DELETE ON public.agent_team_contracts
FOR EACH ROW EXECUTE FUNCTION public.hai_reject_agent_team_append_only_mutation();

CREATE CONSTRAINT TRIGGER trg_agent_team_contracts_require_event
AFTER INSERT OR UPDATE ON public.agent_team_contracts
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION public.hai_require_agent_team_revision_event();

CREATE TRIGGER trg_agent_team_lifecycle_events_chain
BEFORE INSERT ON public.agent_team_lifecycle_events
FOR EACH ROW EXECUTE FUNCTION public.hai_validate_agent_team_event_chain();

CREATE TRIGGER trg_agent_team_lifecycle_events_immutable
BEFORE UPDATE OR DELETE ON public.agent_team_lifecycle_events
FOR EACH ROW EXECUTE FUNCTION public.hai_reject_agent_team_append_only_mutation();

CREATE TRIGGER trg_agent_team_coordination_messages_immutable
BEFORE UPDATE OR DELETE ON public.agent_team_coordination_messages
FOR EACH ROW EXECUTE FUNCTION public.hai_reject_agent_team_append_only_mutation();

CREATE TRIGGER trg_agent_team_consensus_outcomes_revision
BEFORE INSERT ON public.agent_team_consensus_outcomes
FOR EACH ROW EXECUTE FUNCTION public.hai_validate_agent_team_consensus_revision();

CREATE TRIGGER trg_agent_team_consensus_outcomes_immutable
BEFORE UPDATE OR DELETE ON public.agent_team_consensus_outcomes
FOR EACH ROW EXECUTE FUNCTION public.hai_reject_agent_team_append_only_mutation();

CREATE TRIGGER trg_agent_teams_no_truncate
BEFORE TRUNCATE ON public.agent_teams
FOR EACH STATEMENT EXECUTE FUNCTION public.hai_reject_agent_team_append_only_mutation();
CREATE TRIGGER trg_agent_team_contracts_no_truncate
BEFORE TRUNCATE ON public.agent_team_contracts
FOR EACH STATEMENT EXECUTE FUNCTION public.hai_reject_agent_team_append_only_mutation();
CREATE TRIGGER trg_agent_team_lifecycle_events_no_truncate
BEFORE TRUNCATE ON public.agent_team_lifecycle_events
FOR EACH STATEMENT EXECUTE FUNCTION public.hai_reject_agent_team_append_only_mutation();
CREATE TRIGGER trg_agent_team_coordination_messages_no_truncate
BEFORE TRUNCATE ON public.agent_team_coordination_messages
FOR EACH STATEMENT EXECUTE FUNCTION public.hai_reject_agent_team_append_only_mutation();
CREATE TRIGGER trg_agent_team_consensus_outcomes_no_truncate
BEFORE TRUNCATE ON public.agent_team_consensus_outcomes
FOR EACH STATEMENT EXECUTE FUNCTION public.hai_reject_agent_team_append_only_mutation();
