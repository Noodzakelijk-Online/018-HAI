CREATE TABLE IF NOT EXISTS public.agent_registry_agents (
    owner_identity character varying(256) NOT NULL,
    id character varying(256) NOT NULL,
    revision bigint NOT NULL,
    contract_version integer NOT NULL,
    agent_type character varying(40) NOT NULL,
    lifecycle_state character varying(40) NOT NULL,
    runtime_adapter_id character varying(256) NOT NULL,
    health_status character varying(40) NOT NULL,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL,
    payload jsonb NOT NULL,
    CONSTRAINT agent_registry_agents_pkey PRIMARY KEY (owner_identity, id),
    CONSTRAINT chk_agent_registry_agent_identity CHECK (
        btrim(owner_identity) <> '' AND btrim(id) <> ''
    ),
    CONSTRAINT chk_agent_registry_agent_revision CHECK (revision > 0),
    CONSTRAINT chk_agent_registry_agent_contract CHECK (contract_version > 0),
    CONSTRAINT chk_agent_registry_agent_type CHECK (
        agent_type IN (
            'planner', 'researcher', 'executor', 'reviewer',
            'specialist', 'orchestrator'
        )
    ),
    CONSTRAINT chk_agent_registry_agent_state CHECK (
        lifecycle_state IN (
            'registered', 'enabled', 'draining', 'disabled', 'quarantined'
        )
    ),
    CONSTRAINT chk_agent_registry_agent_health CHECK (
        health_status IN ('unknown', 'healthy', 'degraded', 'unhealthy')
    ),
    CONSTRAINT chk_agent_registry_agent_timestamps CHECK (updated_at >= created_at),
    CONSTRAINT chk_agent_registry_agent_payload CHECK (
        jsonb_typeof(payload) = 'object'
        AND payload ?& ARRAY[
            'ownerIdentity', 'id', 'revision', 'contractVersion',
            'type', 'state', 'runtime', 'health'
        ]
        AND jsonb_typeof(payload->'runtime') = 'object'
        AND payload->'runtime' ? 'id'
        AND jsonb_typeof(payload->'health') = 'object'
        AND payload->'health' ? 'status'
        AND payload->>'ownerIdentity' = owner_identity
        AND payload->>'id' = id
        AND (payload->>'revision')::bigint = revision
        AND (payload->>'contractVersion')::integer = contract_version
        AND payload->>'type' = agent_type
        AND payload->>'state' = lifecycle_state
        AND payload#>>'{runtime,id}' = runtime_adapter_id
        AND payload#>>'{health,status}' = health_status
    )
);

CREATE INDEX IF NOT EXISTS idx_agent_registry_agents_owner_state
    ON public.agent_registry_agents (owner_identity, lifecycle_state, id);
CREATE INDEX IF NOT EXISTS idx_agent_registry_agents_owner_type
    ON public.agent_registry_agents (owner_identity, agent_type, id);
CREATE INDEX IF NOT EXISTS idx_agent_registry_agents_runtime
    ON public.agent_registry_agents (runtime_adapter_id);
CREATE INDEX IF NOT EXISTS idx_agent_registry_agents_health
    ON public.agent_registry_agents (health_status);
CREATE INDEX IF NOT EXISTS idx_agent_registry_agents_updated
    ON public.agent_registry_agents (updated_at DESC);

CREATE TABLE IF NOT EXISTS public.agent_registry_revisions (
    owner_identity character varying(256) NOT NULL,
    agent_id character varying(256) NOT NULL,
    revision bigint NOT NULL,
    recorded_at timestamp with time zone NOT NULL,
    payload jsonb NOT NULL,
    CONSTRAINT agent_registry_revisions_pkey
        PRIMARY KEY (owner_identity, agent_id, revision),
    CONSTRAINT fk_agent_registry_revision_agent
        FOREIGN KEY (owner_identity, agent_id)
        REFERENCES public.agent_registry_agents (owner_identity, id)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT chk_agent_registry_revision_value CHECK (revision > 0),
    CONSTRAINT chk_agent_registry_revision_payload CHECK (
        jsonb_typeof(payload) = 'object'
        AND payload ?& ARRAY['ownerIdentity', 'id', 'revision']
        AND payload->>'ownerIdentity' = owner_identity
        AND payload->>'id' = agent_id
        AND (payload->>'revision')::bigint = revision
    )
);

CREATE INDEX IF NOT EXISTS idx_agent_registry_revisions_recorded
    ON public.agent_registry_revisions
    (owner_identity, agent_id, recorded_at DESC, revision DESC);

CREATE TABLE IF NOT EXISTS public.agent_registry_transitions (
    sequence_id bigint GENERATED ALWAYS AS IDENTITY,
    owner_identity character varying(256) NOT NULL,
    agent_id character varying(256) NOT NULL,
    revision bigint NOT NULL,
    from_state character varying(40) NOT NULL,
    to_state character varying(40) NOT NULL,
    occurred_at timestamp with time zone NOT NULL,
    payload jsonb NOT NULL,
    CONSTRAINT agent_registry_transitions_pkey PRIMARY KEY (sequence_id),
    CONSTRAINT uq_agent_registry_transition_revision
        UNIQUE (owner_identity, agent_id, revision),
    CONSTRAINT fk_agent_registry_transition_agent
        FOREIGN KEY (owner_identity, agent_id)
        REFERENCES public.agent_registry_agents (owner_identity, id)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT chk_agent_registry_transition_revision CHECK (revision >= 2),
    CONSTRAINT chk_agent_registry_transition_state CHECK (
        from_state IN (
            'registered', 'enabled', 'draining', 'disabled', 'quarantined'
        )
        AND to_state IN (
            'registered', 'enabled', 'draining', 'disabled', 'quarantined'
        )
        AND from_state <> to_state
    ),
    CONSTRAINT chk_agent_registry_transition_payload CHECK (
        jsonb_typeof(payload) = 'object'
        AND payload ?& ARRAY['revision', 'from', 'to']
        AND (payload->>'revision')::bigint = revision
        AND payload->>'from' = from_state
        AND payload->>'to' = to_state
    )
);

CREATE INDEX IF NOT EXISTS idx_agent_registry_transitions_agent_time
    ON public.agent_registry_transitions
    (owner_identity, agent_id, occurred_at DESC, sequence_id DESC);

CREATE TABLE IF NOT EXISTS public.agent_registry_assignments (
    owner_identity character varying(256) NOT NULL,
    id character varying(256) NOT NULL,
    task_id character varying(256) NOT NULL,
    agent_id character varying(256) NOT NULL,
    agent_revision bigint NOT NULL,
    granted_authority integer NOT NULL,
    granted_autonomy integer NOT NULL,
    assigned_at timestamp with time zone NOT NULL,
    payload jsonb NOT NULL,
    CONSTRAINT agent_registry_assignments_pkey PRIMARY KEY (owner_identity, id),
    CONSTRAINT uq_agent_registry_assignment_agent
        UNIQUE (owner_identity, id, agent_id),
    CONSTRAINT fk_agent_registry_assignment_agent
        FOREIGN KEY (owner_identity, agent_id)
        REFERENCES public.agent_registry_agents (owner_identity, id)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT fk_agent_registry_assignment_revision
        FOREIGN KEY (owner_identity, agent_id, agent_revision)
        REFERENCES public.agent_registry_revisions
            (owner_identity, agent_id, revision)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT chk_agent_registry_assignment_identity CHECK (
        btrim(owner_identity) <> ''
        AND btrim(id) <> ''
        AND btrim(task_id) <> ''
        AND btrim(agent_id) <> ''
    ),
    CONSTRAINT chk_agent_registry_assignment_revision CHECK (agent_revision > 0),
    CONSTRAINT chk_agent_registry_assignment_authority CHECK (
        granted_authority BETWEEN 0 AND 10
        AND granted_autonomy BETWEEN 0 AND 10
    ),
    CONSTRAINT chk_agent_registry_assignment_payload CHECK (
        jsonb_typeof(payload) = 'object'
        AND payload ?& ARRAY[
            'ownerIdentity', 'id', 'taskId', 'agentId',
            'agentRevision', 'grantedAuthority', 'grantedAutonomy'
        ]
        AND payload->>'ownerIdentity' = owner_identity
        AND payload->>'id' = id
        AND payload->>'taskId' = task_id
        AND payload->>'agentId' = agent_id
        AND (payload->>'agentRevision')::bigint = agent_revision
        AND (payload->>'grantedAuthority')::integer = granted_authority
        AND (payload->>'grantedAutonomy')::integer = granted_autonomy
    )
);

CREATE INDEX IF NOT EXISTS idx_agent_registry_assignments_task
    ON public.agent_registry_assignments (owner_identity, task_id, assigned_at DESC);
CREATE INDEX IF NOT EXISTS idx_agent_registry_assignments_agent
    ON public.agent_registry_assignments
    (owner_identity, agent_id, assigned_at DESC);

CREATE TABLE IF NOT EXISTS public.agent_registry_assignment_outcomes (
    owner_identity character varying(256) NOT NULL,
    assignment_id character varying(256) NOT NULL,
    agent_id character varying(256) NOT NULL,
    success boolean NOT NULL,
    latency_ns bigint NOT NULL,
    recorded_at timestamp with time zone NOT NULL,
    payload jsonb NOT NULL,
    CONSTRAINT agent_registry_assignment_outcomes_pkey
        PRIMARY KEY (owner_identity, assignment_id),
    CONSTRAINT fk_agent_registry_outcome_assignment
        FOREIGN KEY (owner_identity, assignment_id, agent_id)
        REFERENCES public.agent_registry_assignments
            (owner_identity, id, agent_id)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT chk_agent_registry_outcome_latency CHECK (latency_ns >= 0),
    CONSTRAINT chk_agent_registry_outcome_payload CHECK (
        jsonb_typeof(payload) = 'object'
        AND payload ?& ARRAY[
            'assignmentId', 'ownerIdentity', 'agentId',
            'success', 'latency', 'recordedAt'
        ]
        AND payload->>'assignmentId' = assignment_id
        AND payload->>'ownerIdentity' = owner_identity
        AND payload->>'agentId' = agent_id
        AND (payload->>'success')::boolean = success
        AND (payload->>'latency')::bigint = latency_ns
        AND (payload->>'recordedAt')::timestamp with time zone = recorded_at
    )
);

CREATE INDEX IF NOT EXISTS idx_agent_registry_outcomes_agent
    ON public.agent_registry_assignment_outcomes
    (owner_identity, agent_id, recorded_at DESC);

CREATE OR REPLACE FUNCTION public.hai_enforce_agent_registry_revision()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    IF NEW.owner_identity IS DISTINCT FROM OLD.owner_identity
       OR NEW.id IS DISTINCT FROM OLD.id
       OR NEW.created_at IS DISTINCT FROM OLD.created_at
       OR NEW.revision <> OLD.revision + 1 THEN
        RAISE EXCEPTION 'agent registry identity is immutable and revision must advance by one'
            USING ERRCODE = 'integrity_constraint_violation';
    END IF;
    RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS trg_agent_registry_agent_revision
    ON public.agent_registry_agents;
CREATE TRIGGER trg_agent_registry_agent_revision
    BEFORE UPDATE ON public.agent_registry_agents
    FOR EACH ROW
    EXECUTE FUNCTION public.hai_enforce_agent_registry_revision();

CREATE OR REPLACE FUNCTION public.hai_reject_agent_registry_audit_mutation()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE EXCEPTION 'agent registry audit records are append-only'
        USING ERRCODE = 'integrity_constraint_violation';
END;
$$;

DROP TRIGGER IF EXISTS trg_agent_registry_revisions_immutable
    ON public.agent_registry_revisions;
CREATE TRIGGER trg_agent_registry_revisions_immutable
    BEFORE UPDATE OR DELETE ON public.agent_registry_revisions
    FOR EACH ROW
    EXECUTE FUNCTION public.hai_reject_agent_registry_audit_mutation();

DROP TRIGGER IF EXISTS trg_agent_registry_revisions_no_truncate
    ON public.agent_registry_revisions;
CREATE TRIGGER trg_agent_registry_revisions_no_truncate
    BEFORE TRUNCATE ON public.agent_registry_revisions
    FOR EACH STATEMENT
    EXECUTE FUNCTION public.hai_reject_agent_registry_audit_mutation();

DROP TRIGGER IF EXISTS trg_agent_registry_transitions_immutable
    ON public.agent_registry_transitions;
CREATE TRIGGER trg_agent_registry_transitions_immutable
    BEFORE UPDATE OR DELETE ON public.agent_registry_transitions
    FOR EACH ROW
    EXECUTE FUNCTION public.hai_reject_agent_registry_audit_mutation();

DROP TRIGGER IF EXISTS trg_agent_registry_transitions_no_truncate
    ON public.agent_registry_transitions;
CREATE TRIGGER trg_agent_registry_transitions_no_truncate
    BEFORE TRUNCATE ON public.agent_registry_transitions
    FOR EACH STATEMENT
    EXECUTE FUNCTION public.hai_reject_agent_registry_audit_mutation();

DROP TRIGGER IF EXISTS trg_agent_registry_assignments_immutable
    ON public.agent_registry_assignments;
CREATE TRIGGER trg_agent_registry_assignments_immutable
    BEFORE UPDATE OR DELETE ON public.agent_registry_assignments
    FOR EACH ROW
    EXECUTE FUNCTION public.hai_reject_agent_registry_audit_mutation();

DROP TRIGGER IF EXISTS trg_agent_registry_assignments_no_truncate
    ON public.agent_registry_assignments;
CREATE TRIGGER trg_agent_registry_assignments_no_truncate
    BEFORE TRUNCATE ON public.agent_registry_assignments
    FOR EACH STATEMENT
    EXECUTE FUNCTION public.hai_reject_agent_registry_audit_mutation();

DROP TRIGGER IF EXISTS trg_agent_registry_outcomes_immutable
    ON public.agent_registry_assignment_outcomes;
CREATE TRIGGER trg_agent_registry_outcomes_immutable
    BEFORE UPDATE OR DELETE ON public.agent_registry_assignment_outcomes
    FOR EACH ROW
    EXECUTE FUNCTION public.hai_reject_agent_registry_audit_mutation();

DROP TRIGGER IF EXISTS trg_agent_registry_outcomes_no_truncate
    ON public.agent_registry_assignment_outcomes;
CREATE TRIGGER trg_agent_registry_outcomes_no_truncate
    BEFORE TRUNCATE ON public.agent_registry_assignment_outcomes
    FOR EACH STATEMENT
    EXECUTE FUNCTION public.hai_reject_agent_registry_audit_mutation();
