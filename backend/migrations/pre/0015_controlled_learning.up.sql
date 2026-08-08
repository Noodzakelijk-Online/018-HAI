CREATE TABLE public.controlled_learning_outcomes (
    id uuid NOT NULL,
    protocol_version character varying(32) NOT NULL,
    owner_identity character varying(256) NOT NULL,
    idempotency_key character varying(256) NOT NULL,
    operation_id character varying(256) NOT NULL,
    project_key character varying(256) NOT NULL DEFAULT '',
    basis character varying(32) NOT NULL,
    outcome_status character varying(32) NOT NULL,
    verification_status character varying(32) NOT NULL,
    evidence_digest character varying(71) NOT NULL,
    occurred_at timestamp with time zone NOT NULL,
    recorded_at timestamp with time zone NOT NULL,
    payload jsonb NOT NULL,
    CONSTRAINT controlled_learning_outcomes_pkey PRIMARY KEY (id),
    CONSTRAINT uq_controlled_learning_outcome_owner_id
        UNIQUE (owner_identity, id),
    CONSTRAINT uq_controlled_learning_outcome_idempotency
        UNIQUE (owner_identity, idempotency_key),
    CONSTRAINT chk_controlled_learning_outcome_identity CHECK (
        char_length(btrim(owner_identity)) BETWEEN 1 AND 256
        AND char_length(btrim(idempotency_key)) BETWEEN 1 AND 256
        AND char_length(btrim(operation_id)) BETWEEN 1 AND 256
        AND char_length(project_key) <= 256
    ),
    CONSTRAINT chk_controlled_learning_outcome_protocol CHECK (
        char_length(btrim(protocol_version)) BETWEEN 1 AND 32
    ),
    CONSTRAINT chk_controlled_learning_outcome_basis CHECK (
        basis IN ('verified_outcome', 'human_correction')
    ),
    CONSTRAINT chk_controlled_learning_outcome_status CHECK (
        outcome_status IN ('succeeded', 'partial', 'failed', 'corrected')
    ),
    CONSTRAINT chk_controlled_learning_outcome_verification CHECK (
        verification_status IN (
            'verified',
            'source_supported',
            'schema_validated',
            'test_passed',
            'human_approved'
        )
    ),
    CONSTRAINT chk_controlled_learning_outcome_digest CHECK (
        evidence_digest ~ '^sha256:[0-9a-f]{64}$'
    ),
    CONSTRAINT chk_controlled_learning_outcome_time CHECK (
        occurred_at <= recorded_at + interval '5 minutes'
    ),
    CONSTRAINT chk_controlled_learning_outcome_payload CHECK (
        jsonb_typeof(payload) = 'object'
        AND octet_length(payload::text) BETWEEN 2 AND 262144
        AND payload #>> '{id}' = id::text
        AND payload #>> '{ownerIdentity}' = owner_identity
        AND payload #>> '{idempotencyKey}' = idempotency_key
        AND payload #>> '{operationId}' = operation_id
        AND payload #>> '{protocolVersion}' = protocol_version
        AND payload #>> '{basis}' = basis
        AND payload #>> '{status}' = outcome_status
        AND payload #>> '{verification}' = verification_status
        AND payload #>> '{evidenceDigest}' = evidence_digest
    )
);

CREATE INDEX idx_controlled_learning_outcomes_owner_recorded
    ON public.controlled_learning_outcomes
    (owner_identity, recorded_at DESC, id DESC);
CREATE INDEX idx_controlled_learning_outcomes_owner_operation
    ON public.controlled_learning_outcomes
    (owner_identity, operation_id, recorded_at DESC, id DESC);
CREATE INDEX idx_controlled_learning_outcomes_owner_project
    ON public.controlled_learning_outcomes
    (owner_identity, project_key, recorded_at DESC, id DESC)
    WHERE project_key <> '';

CREATE TABLE public.controlled_learning_proposals (
    id uuid NOT NULL,
    protocol_version character varying(32) NOT NULL,
    owner_identity character varying(256) NOT NULL,
    idempotency_key character varying(256) NOT NULL,
    revision bigint NOT NULL,
    proposal_status character varying(32) NOT NULL,
    learning_method character varying(64) NOT NULL,
    target_kind character varying(64) NOT NULL,
    protected_target boolean NOT NULL,
    proposal_digest character varying(71) NOT NULL,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL,
    updated_at_unix_nano bigint NOT NULL,
    definition_payload jsonb NOT NULL,
    CONSTRAINT controlled_learning_proposals_pkey PRIMARY KEY (id),
    CONSTRAINT uq_controlled_learning_proposal_owner_id
        UNIQUE (owner_identity, id),
    CONSTRAINT uq_controlled_learning_proposal_idempotency
        UNIQUE (owner_identity, idempotency_key),
    CONSTRAINT chk_controlled_learning_proposal_identity CHECK (
        char_length(btrim(owner_identity)) BETWEEN 1 AND 256
        AND char_length(btrim(idempotency_key)) BETWEEN 1 AND 256
    ),
    CONSTRAINT chk_controlled_learning_proposal_protocol CHECK (
        char_length(btrim(protocol_version)) BETWEEN 1 AND 32
    ),
    CONSTRAINT chk_controlled_learning_proposal_revision CHECK (
        revision > 0
    ),
    CONSTRAINT chk_controlled_learning_proposal_status CHECK (
        proposal_status IN (
            'review_required',
            'governance_required',
            'governance_review',
            'approved',
            'rejected',
            'changes_requested'
        )
    ),
    CONSTRAINT chk_controlled_learning_proposal_method CHECK (
        char_length(btrim(learning_method)) BETWEEN 1 AND 64
    ),
    CONSTRAINT chk_controlled_learning_proposal_target CHECK (
        char_length(btrim(target_kind)) BETWEEN 1 AND 64
    ),
    CONSTRAINT chk_controlled_learning_proposal_digest CHECK (
        proposal_digest ~ '^sha256:[0-9a-f]{64}$'
    ),
    CONSTRAINT chk_controlled_learning_proposal_time CHECK (
        updated_at >= created_at
        AND updated_at_unix_nano > 0
    ),
    CONSTRAINT chk_controlled_learning_proposal_payload CHECK (
        jsonb_typeof(definition_payload) = 'object'
        AND octet_length(definition_payload::text) BETWEEN 2 AND 131072
        AND definition_payload #>> '{id}' = id::text
        AND definition_payload #>> '{ownerIdentity}' = owner_identity
        AND definition_payload #>> '{idempotencyKey}' = idempotency_key
        AND definition_payload #>> '{protocolVersion}' = protocol_version
        AND definition_payload #>> '{method}' = learning_method
        AND definition_payload #>> '{target}' = target_kind
        AND (definition_payload #>> '{protectedTarget}')::boolean =
            protected_target
        AND definition_payload #>> '{proposalDigest}' = proposal_digest
    )
);

CREATE INDEX idx_controlled_learning_proposals_owner_updated
    ON public.controlled_learning_proposals
    (owner_identity, updated_at DESC, id DESC);
CREATE INDEX idx_controlled_learning_proposals_owner_status
    ON public.controlled_learning_proposals
    (owner_identity, proposal_status, updated_at DESC, id DESC);
CREATE INDEX idx_controlled_learning_proposals_owner_target
    ON public.controlled_learning_proposals
    (owner_identity, target_kind, updated_at DESC, id DESC);

CREATE TABLE public.controlled_learning_proposal_evidence (
    owner_identity character varying(256) NOT NULL,
    proposal_id uuid NOT NULL,
    outcome_id uuid NOT NULL,
    ordinal smallint NOT NULL,
    CONSTRAINT controlled_learning_proposal_evidence_pkey
        PRIMARY KEY (owner_identity, proposal_id, outcome_id),
    CONSTRAINT uq_controlled_learning_proposal_evidence_ordinal
        UNIQUE (owner_identity, proposal_id, ordinal),
    CONSTRAINT fk_controlled_learning_proposal_evidence_proposal
        FOREIGN KEY (owner_identity, proposal_id)
        REFERENCES public.controlled_learning_proposals
            (owner_identity, id)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT fk_controlled_learning_proposal_evidence_outcome
        FOREIGN KEY (owner_identity, outcome_id)
        REFERENCES public.controlled_learning_outcomes
            (owner_identity, id)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT chk_controlled_learning_proposal_evidence_owner CHECK (
        char_length(btrim(owner_identity)) BETWEEN 1 AND 256
    ),
    CONSTRAINT chk_controlled_learning_proposal_evidence_ordinal CHECK (
        ordinal BETWEEN 0 AND 99
    )
);

CREATE INDEX idx_controlled_learning_proposal_evidence_outcome
    ON public.controlled_learning_proposal_evidence
    (owner_identity, outcome_id, proposal_id);

CREATE TABLE public.controlled_learning_review_decisions (
    id uuid NOT NULL,
    owner_identity character varying(256) NOT NULL,
    proposal_id uuid NOT NULL,
    proposal_revision bigint NOT NULL,
    decision_kind character varying(32) NOT NULL,
    actor_identity character varying(256) NOT NULL,
    human_confirmed boolean NOT NULL,
    proposal_digest character varying(71) NOT NULL,
    decision_digest character varying(71) NOT NULL,
    decided_at timestamp with time zone NOT NULL,
    payload jsonb NOT NULL,
    CONSTRAINT controlled_learning_review_decisions_pkey PRIMARY KEY (id),
    CONSTRAINT uq_controlled_learning_review_decision_owner_id
        UNIQUE (owner_identity, id),
    CONSTRAINT uq_controlled_learning_review_decision_revision
        UNIQUE (owner_identity, proposal_id, proposal_revision),
    CONSTRAINT fk_controlled_learning_review_decision_proposal
        FOREIGN KEY (owner_identity, proposal_id)
        REFERENCES public.controlled_learning_proposals
            (owner_identity, id)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT chk_controlled_learning_review_decision_identity CHECK (
        char_length(btrim(owner_identity)) BETWEEN 1 AND 256
        AND char_length(btrim(actor_identity)) BETWEEN 1 AND 256
    ),
    CONSTRAINT chk_controlled_learning_review_decision_revision CHECK (
        proposal_revision > 0
    ),
    CONSTRAINT chk_controlled_learning_review_decision_kind CHECK (
        decision_kind IN (
            'approve',
            'reject',
            'request_changes',
            'escalate_governance'
        )
    ),
    CONSTRAINT chk_controlled_learning_review_decision_confirmation CHECK (
        human_confirmed
    ),
    CONSTRAINT chk_controlled_learning_review_decision_digests CHECK (
        proposal_digest ~ '^sha256:[0-9a-f]{64}$'
        AND decision_digest ~ '^sha256:[0-9a-f]{64}$'
    ),
    CONSTRAINT chk_controlled_learning_review_decision_payload CHECK (
        jsonb_typeof(payload) = 'object'
        AND octet_length(payload::text) BETWEEN 2 AND 65536
        AND payload #>> '{id}' = id::text
        AND payload #>> '{ownerIdentity}' = owner_identity
        AND payload #>> '{proposalId}' = proposal_id::text
        AND (payload #>> '{proposalRevision}')::bigint =
            proposal_revision
        AND payload #>> '{kind}' = decision_kind
        AND payload #>> '{actorIdentity}' = actor_identity
        AND (payload #>> '{humanConfirmed}')::boolean =
            human_confirmed
        AND payload #>> '{proposalDigest}' = proposal_digest
        AND payload #>> '{decisionDigest}' = decision_digest
    )
);

CREATE INDEX idx_controlled_learning_review_decisions_owner_proposal
    ON public.controlled_learning_review_decisions
    (owner_identity, proposal_id, proposal_revision ASC);
CREATE INDEX idx_controlled_learning_review_decisions_owner_decided
    ON public.controlled_learning_review_decisions
    (owner_identity, decided_at DESC, id DESC);

CREATE OR REPLACE FUNCTION public.hai_reject_controlled_learning_mutation()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE EXCEPTION 'controlled learning ledger records are append-only'
        USING ERRCODE = 'integrity_constraint_violation';
END;
$$;

CREATE OR REPLACE FUNCTION public.hai_guard_controlled_learning_proposal_state()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    IF NEW.id IS DISTINCT FROM OLD.id
        OR NEW.protocol_version IS DISTINCT FROM OLD.protocol_version
        OR NEW.owner_identity IS DISTINCT FROM OLD.owner_identity
        OR NEW.idempotency_key IS DISTINCT FROM OLD.idempotency_key
        OR NEW.learning_method IS DISTINCT FROM OLD.learning_method
        OR NEW.target_kind IS DISTINCT FROM OLD.target_kind
        OR NEW.protected_target IS DISTINCT FROM OLD.protected_target
        OR NEW.proposal_digest IS DISTINCT FROM OLD.proposal_digest
        OR NEW.created_at IS DISTINCT FROM OLD.created_at
        OR NEW.definition_payload IS DISTINCT FROM OLD.definition_payload
    THEN
        RAISE EXCEPTION 'controlled learning proposal definitions are immutable'
            USING ERRCODE = 'integrity_constraint_violation';
    END IF;

    IF NEW.revision <> OLD.revision + 1
        OR NEW.updated_at < OLD.updated_at
        OR NEW.updated_at_unix_nano < OLD.updated_at_unix_nano
    THEN
        RAISE EXCEPTION 'controlled learning proposal state transition is not monotonic'
            USING ERRCODE = 'integrity_constraint_violation';
    END IF;

    IF NOT (
        (OLD.proposal_status = 'review_required'
            AND NEW.proposal_status IN (
                'approved',
                'rejected',
                'changes_requested'
            ))
        OR (OLD.proposal_status = 'changes_requested'
            AND NEW.proposal_status IN ('approved', 'rejected'))
        OR (OLD.proposal_status = 'governance_required'
            AND NEW.proposal_status IN ('governance_review', 'rejected'))
    ) THEN
        RAISE EXCEPTION 'controlled learning proposal status transition is invalid'
            USING ERRCODE = 'integrity_constraint_violation';
    END IF;

    RETURN NEW;
END;
$$;

CREATE OR REPLACE FUNCTION public.hai_validate_controlled_learning_proposal_insert()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    IF NEW.revision <> 1
        OR NEW.definition_payload #>> '{status}' <> NEW.proposal_status
        OR (NEW.definition_payload #>> '{revision}')::bigint <> NEW.revision
    THEN
        RAISE EXCEPTION 'controlled learning proposal initial state is invalid'
            USING ERRCODE = 'integrity_constraint_violation';
    END IF;
    RETURN NEW;
END;
$$;

CREATE OR REPLACE FUNCTION public.hai_require_controlled_learning_review_pair()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    paired boolean;
BEGIN
    IF TG_TABLE_NAME = 'controlled_learning_proposals' THEN
        SELECT EXISTS (
            SELECT 1
            FROM public.controlled_learning_review_decisions AS decision
            WHERE decision.owner_identity = NEW.owner_identity
              AND decision.proposal_id = NEW.id
              AND decision.proposal_revision = OLD.revision
              AND decision.proposal_digest = NEW.proposal_digest
              AND (
                  (decision.decision_kind = 'approve'
                      AND NEW.proposal_status = 'approved')
                  OR (decision.decision_kind = 'reject'
                      AND NEW.proposal_status = 'rejected')
                  OR (decision.decision_kind = 'request_changes'
                      AND NEW.proposal_status = 'changes_requested')
                  OR (decision.decision_kind = 'escalate_governance'
                      AND NEW.proposal_status = 'governance_review')
              )
        ) INTO paired;
    ELSE
        SELECT EXISTS (
            SELECT 1
            FROM public.controlled_learning_proposals AS proposal
            WHERE proposal.owner_identity = NEW.owner_identity
              AND proposal.id = NEW.proposal_id
              AND proposal.revision = NEW.proposal_revision + 1
              AND proposal.proposal_digest = NEW.proposal_digest
              AND (
                  (NEW.decision_kind = 'approve'
                      AND proposal.proposal_status = 'approved')
                  OR (NEW.decision_kind = 'reject'
                      AND proposal.proposal_status = 'rejected')
                  OR (NEW.decision_kind = 'request_changes'
                      AND proposal.proposal_status = 'changes_requested')
                  OR (NEW.decision_kind = 'escalate_governance'
                      AND proposal.proposal_status = 'governance_review')
              )
        ) INTO paired;
    END IF;

    IF NOT paired THEN
        RAISE EXCEPTION 'controlled learning review decision and state transition must commit atomically'
            USING ERRCODE = 'integrity_constraint_violation';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_controlled_learning_outcomes_immutable
    BEFORE UPDATE OR DELETE ON public.controlled_learning_outcomes
    FOR EACH ROW
    EXECUTE FUNCTION public.hai_reject_controlled_learning_mutation();

CREATE TRIGGER trg_controlled_learning_outcomes_no_truncate
    BEFORE TRUNCATE ON public.controlled_learning_outcomes
    FOR EACH STATEMENT
    EXECUTE FUNCTION public.hai_reject_controlled_learning_mutation();

CREATE TRIGGER trg_controlled_learning_proposals_guard_update
    BEFORE UPDATE ON public.controlled_learning_proposals
    FOR EACH ROW
    EXECUTE FUNCTION public.hai_guard_controlled_learning_proposal_state();

CREATE TRIGGER trg_controlled_learning_proposals_validate_insert
    BEFORE INSERT ON public.controlled_learning_proposals
    FOR EACH ROW
    EXECUTE FUNCTION public.hai_validate_controlled_learning_proposal_insert();

CREATE TRIGGER trg_controlled_learning_proposals_no_delete
    BEFORE DELETE ON public.controlled_learning_proposals
    FOR EACH ROW
    EXECUTE FUNCTION public.hai_reject_controlled_learning_mutation();

CREATE TRIGGER trg_controlled_learning_proposals_no_truncate
    BEFORE TRUNCATE ON public.controlled_learning_proposals
    FOR EACH STATEMENT
    EXECUTE FUNCTION public.hai_reject_controlled_learning_mutation();

CREATE TRIGGER trg_controlled_learning_proposal_evidence_immutable
    BEFORE UPDATE OR DELETE ON public.controlled_learning_proposal_evidence
    FOR EACH ROW
    EXECUTE FUNCTION public.hai_reject_controlled_learning_mutation();

CREATE TRIGGER trg_controlled_learning_proposal_evidence_no_truncate
    BEFORE TRUNCATE ON public.controlled_learning_proposal_evidence
    FOR EACH STATEMENT
    EXECUTE FUNCTION public.hai_reject_controlled_learning_mutation();

CREATE TRIGGER trg_controlled_learning_review_decisions_immutable
    BEFORE UPDATE OR DELETE ON public.controlled_learning_review_decisions
    FOR EACH ROW
    EXECUTE FUNCTION public.hai_reject_controlled_learning_mutation();

CREATE TRIGGER trg_controlled_learning_review_decisions_no_truncate
    BEFORE TRUNCATE ON public.controlled_learning_review_decisions
    FOR EACH STATEMENT
    EXECUTE FUNCTION public.hai_reject_controlled_learning_mutation();

CREATE CONSTRAINT TRIGGER trg_controlled_learning_proposals_require_decision
    AFTER UPDATE ON public.controlled_learning_proposals
    DEFERRABLE INITIALLY DEFERRED
    FOR EACH ROW
    EXECUTE FUNCTION public.hai_require_controlled_learning_review_pair();

CREATE CONSTRAINT TRIGGER trg_controlled_learning_decisions_require_state
    AFTER INSERT ON public.controlled_learning_review_decisions
    DEFERRABLE INITIALLY DEFERRED
    FOR EACH ROW
    EXECUTE FUNCTION public.hai_require_controlled_learning_review_pair();
