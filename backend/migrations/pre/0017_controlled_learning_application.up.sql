DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM public.controlled_learning_proposals
        WHERE proposal_status IN ('approved', 'governance_review')
    ) THEN
        RAISE EXCEPTION
            'controlled learning has legacy applied labels without application evidence; reconcile them before migration'
            USING ERRCODE = 'integrity_constraint_violation';
    END IF;
END;
$$;

CREATE TABLE public.controlled_learning_applications (
    id uuid NOT NULL,
    protocol_version character varying(32) NOT NULL,
    owner_identity character varying(256) NOT NULL,
    proposal_id uuid NOT NULL,
    proposal_revision bigint NOT NULL,
    proposal_digest character varying(71) NOT NULL,
    idempotency_key character varying(256) NOT NULL,
    intent_digest character varying(71) NOT NULL,
    application_mode character varying(32) NOT NULL,
    application_status character varying(32) NOT NULL,
    target_kind character varying(64) NOT NULL,
    protected_target boolean NOT NULL,
    applier_id character varying(256) NOT NULL,
    current_version character varying(256) NOT NULL,
    proposed_version character varying(256) NOT NULL,
    attempt integer NOT NULL,
    lease_expires_at timestamp with time zone,
    definition_digest character varying(71) NOT NULL,
    result_digest character varying(71),
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL,
    completed_at timestamp with time zone,
    payload jsonb NOT NULL,
    CONSTRAINT controlled_learning_applications_pkey PRIMARY KEY (id),
    CONSTRAINT uq_controlled_learning_application_owner_id
        UNIQUE (owner_identity, id),
    CONSTRAINT uq_controlled_learning_application_idempotency
        UNIQUE (owner_identity, idempotency_key),
    CONSTRAINT uq_controlled_learning_application_proposal_mode
        UNIQUE (
            owner_identity,
            proposal_id,
            proposal_revision,
            application_mode
        ),
    CONSTRAINT fk_controlled_learning_application_proposal
        FOREIGN KEY (owner_identity, proposal_id)
        REFERENCES public.controlled_learning_proposals
            (owner_identity, id)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT chk_controlled_learning_application_identity CHECK (
        char_length(btrim(owner_identity)) BETWEEN 1 AND 256
        AND char_length(btrim(applier_id)) BETWEEN 1 AND 256
        AND char_length(btrim(idempotency_key)) BETWEEN 1 AND 256
    ),
    CONSTRAINT chk_controlled_learning_application_revision_attempt CHECK (
        proposal_revision > 0
        AND attempt > 0
    ),
    CONSTRAINT chk_controlled_learning_application_mode CHECK (
        application_mode IN ('apply', 'protected_handoff')
    ),
    CONSTRAINT chk_controlled_learning_application_status CHECK (
        application_status IN (
            'applying',
            'applied',
            'handoff_pending',
            'handoff_ready',
            'failed',
            'rollback_applying',
            'rolled_back',
            'rollback_failed'
        )
    ),
    CONSTRAINT chk_controlled_learning_application_protection CHECK (
        (application_mode = 'apply' AND NOT protected_target)
        OR (application_mode = 'protected_handoff' AND protected_target)
    ),
    CONSTRAINT chk_controlled_learning_application_digests CHECK (
        proposal_digest ~ '^sha256:[0-9a-f]{64}$'
        AND intent_digest ~ '^sha256:[0-9a-f]{64}$'
        AND definition_digest ~ '^sha256:[0-9a-f]{64}$'
        AND (
            result_digest IS NULL
            OR result_digest ~ '^sha256:[0-9a-f]{64}$'
        )
    ),
    CONSTRAINT chk_controlled_learning_application_payload CHECK (
        jsonb_typeof(payload) = 'object'
        AND octet_length(payload::text) BETWEEN 2 AND 262144
        AND payload #>> '{id}' = id::text
        AND payload #>> '{ownerIdentity}' = owner_identity
        AND payload #>> '{proposalId}' = proposal_id::text
        AND (payload #>> '{proposalRevision}')::bigint = proposal_revision
        AND payload #>> '{proposalDigest}' = proposal_digest
        AND payload #>> '{idempotencyKey}' = idempotency_key
        AND payload #>> '{intentDigest}' = intent_digest
        AND payload #>> '{mode}' = application_mode
        AND payload #>> '{status}' = application_status
        AND payload #>> '{target}' = target_kind
        AND (payload #>> '{protectedTarget}')::boolean = protected_target
        AND payload #>> '{applierId}' = applier_id
        AND payload #>> '{currentVersion}' = current_version
        AND payload #>> '{proposedVersion}' = proposed_version
        AND (payload #>> '{attempt}')::integer = attempt
        AND payload #>> '{definitionDigest}' = definition_digest
        AND COALESCE(payload #>> '{resultDigest}', '') =
            COALESCE(result_digest, '')
    )
);

CREATE INDEX idx_controlled_learning_applications_owner_updated
    ON public.controlled_learning_applications
    (owner_identity, updated_at DESC, id DESC);
CREATE INDEX idx_controlled_learning_applications_owner_status
    ON public.controlled_learning_applications
    (owner_identity, application_status, updated_at DESC);
CREATE INDEX idx_controlled_learning_applications_expired_lease
    ON public.controlled_learning_applications
    (lease_expires_at)
    WHERE application_status IN (
        'applying',
        'handoff_pending',
        'rollback_applying'
    );

CREATE TABLE public.controlled_learning_application_events (
    id uuid NOT NULL,
    owner_identity character varying(256) NOT NULL,
    application_id uuid NOT NULL,
    proposal_id uuid NOT NULL,
    attempt integer NOT NULL,
    event_kind character varying(32) NOT NULL,
    application_status character varying(32) NOT NULL,
    application_digest character varying(71) NOT NULL,
    event_digest character varying(71) NOT NULL,
    occurred_at timestamp with time zone NOT NULL,
    payload jsonb NOT NULL,
    CONSTRAINT controlled_learning_application_events_pkey PRIMARY KEY (id),
    CONSTRAINT uq_controlled_learning_application_event_owner_id
        UNIQUE (owner_identity, id),
    CONSTRAINT fk_controlled_learning_application_event_application
        FOREIGN KEY (owner_identity, application_id)
        REFERENCES public.controlled_learning_applications
            (owner_identity, id)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT fk_controlled_learning_application_event_proposal
        FOREIGN KEY (owner_identity, proposal_id)
        REFERENCES public.controlled_learning_proposals
            (owner_identity, id)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT chk_controlled_learning_application_event_attempt CHECK (
        attempt > 0
    ),
    CONSTRAINT chk_controlled_learning_application_event_kind CHECK (
        event_kind IN (
            'reserved',
            'attempt_started',
            'applied',
            'handoff_ready',
            'failed',
            'rollback_started',
            'rolled_back',
            'rollback_failed'
        )
    ),
    CONSTRAINT chk_controlled_learning_application_event_digests CHECK (
        application_digest ~ '^sha256:[0-9a-f]{64}$'
        AND event_digest ~ '^sha256:[0-9a-f]{64}$'
    ),
    CONSTRAINT chk_controlled_learning_application_event_payload CHECK (
        jsonb_typeof(payload) = 'object'
        AND octet_length(payload::text) BETWEEN 2 AND 262144
        AND payload #>> '{id}' = id::text
        AND payload #>> '{ownerIdentity}' = owner_identity
        AND payload #>> '{applicationId}' = application_id::text
        AND payload #>> '{proposalId}' = proposal_id::text
        AND (payload #>> '{attempt}')::integer = attempt
        AND payload #>> '{kind}' = event_kind
        AND payload #>> '{status}' = application_status
        AND payload #>> '{applicationDigest}' = application_digest
        AND payload #>> '{eventDigest}' = event_digest
    )
);

CREATE INDEX idx_controlled_learning_application_events_owner_application
    ON public.controlled_learning_application_events
    (owner_identity, application_id, occurred_at ASC, id ASC);

ALTER TABLE public.controlled_learning_review_decisions
    ADD COLUMN application_id uuid;

ALTER TABLE public.controlled_learning_review_decisions
    ADD CONSTRAINT fk_controlled_learning_review_decision_application
    FOREIGN KEY (owner_identity, application_id)
    REFERENCES public.controlled_learning_applications
        (owner_identity, id)
    ON UPDATE RESTRICT ON DELETE RESTRICT;

ALTER TABLE public.controlled_learning_review_decisions
    ADD CONSTRAINT chk_controlled_learning_review_decision_application CHECK (
        (
            decision_kind IN ('approve', 'escalate_governance')
            AND application_id IS NOT NULL
            AND payload #>> '{applicationId}' = application_id::text
        )
        OR (
            decision_kind IN ('reject', 'request_changes')
            AND application_id IS NULL
            AND COALESCE(payload #>> '{applicationId}', '') = ''
        )
    );

CREATE OR REPLACE FUNCTION public.hai_require_controlled_learning_application()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    backed boolean;
BEGIN
    IF NEW.proposal_status = 'approved' THEN
        SELECT EXISTS (
            SELECT 1
            FROM public.controlled_learning_applications AS application
            JOIN public.controlled_learning_review_decisions AS decision
              ON decision.owner_identity = application.owner_identity
             AND decision.application_id = application.id
            WHERE application.owner_identity = NEW.owner_identity
              AND application.proposal_id = NEW.id
              AND application.proposal_revision = OLD.revision
              AND application.proposal_digest = NEW.proposal_digest
              AND application.application_mode = 'apply'
              AND application.application_status = 'applied'
              AND application.result_digest IS NOT NULL
              AND decision.decision_kind = 'approve'
              AND decision.proposal_revision = OLD.revision
        ) INTO backed;
    ELSIF NEW.proposal_status = 'governance_review' THEN
        SELECT EXISTS (
            SELECT 1
            FROM public.controlled_learning_applications AS application
            JOIN public.controlled_learning_review_decisions AS decision
              ON decision.owner_identity = application.owner_identity
             AND decision.application_id = application.id
            WHERE application.owner_identity = NEW.owner_identity
              AND application.proposal_id = NEW.id
              AND application.proposal_revision = OLD.revision
              AND application.proposal_digest = NEW.proposal_digest
              AND application.application_mode = 'protected_handoff'
              AND application.application_status = 'handoff_ready'
              AND application.result_digest IS NOT NULL
              AND decision.decision_kind = 'escalate_governance'
              AND decision.proposal_revision = OLD.revision
        ) INTO backed;
    ELSE
        RETURN NEW;
    END IF;

    IF NOT backed THEN
        RAISE EXCEPTION
            'controlled learning approval requires a completed application or protected handoff'
            USING ERRCODE = 'integrity_constraint_violation';
    END IF;
    RETURN NEW;
END;
$$;

CREATE CONSTRAINT TRIGGER trg_controlled_learning_proposals_require_application
    AFTER UPDATE ON public.controlled_learning_proposals
    DEFERRABLE INITIALLY DEFERRED
    FOR EACH ROW
    EXECUTE FUNCTION public.hai_require_controlled_learning_application();

CREATE TRIGGER trg_controlled_learning_application_events_immutable
    BEFORE UPDATE OR DELETE
    ON public.controlled_learning_application_events
    FOR EACH ROW
    EXECUTE FUNCTION public.hai_reject_controlled_learning_mutation();

CREATE TRIGGER trg_controlled_learning_application_events_no_truncate
    BEFORE TRUNCATE
    ON public.controlled_learning_application_events
    FOR EACH STATEMENT
    EXECUTE FUNCTION public.hai_reject_controlled_learning_mutation();

CREATE OR REPLACE FUNCTION public.hai_guard_controlled_learning_application_definition()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    IF NEW.id IS DISTINCT FROM OLD.id
        OR NEW.protocol_version IS DISTINCT FROM OLD.protocol_version
        OR NEW.owner_identity IS DISTINCT FROM OLD.owner_identity
        OR NEW.proposal_id IS DISTINCT FROM OLD.proposal_id
        OR NEW.proposal_revision IS DISTINCT FROM OLD.proposal_revision
        OR NEW.proposal_digest IS DISTINCT FROM OLD.proposal_digest
        OR NEW.idempotency_key IS DISTINCT FROM OLD.idempotency_key
        OR NEW.intent_digest IS DISTINCT FROM OLD.intent_digest
        OR NEW.application_mode IS DISTINCT FROM OLD.application_mode
        OR NEW.target_kind IS DISTINCT FROM OLD.target_kind
        OR NEW.protected_target IS DISTINCT FROM OLD.protected_target
        OR NEW.applier_id IS DISTINCT FROM OLD.applier_id
        OR NEW.current_version IS DISTINCT FROM OLD.current_version
        OR NEW.proposed_version IS DISTINCT FROM OLD.proposed_version
        OR NEW.definition_digest IS DISTINCT FROM OLD.definition_digest
        OR NEW.created_at IS DISTINCT FROM OLD.created_at
        OR NEW.payload #>> '{rollbackPlan}'
            IS DISTINCT FROM OLD.payload #>> '{rollbackPlan}'
        OR NEW.payload #>> '{governanceReference}'
            IS DISTINCT FROM OLD.payload #>> '{governanceReference}'
    THEN
        RAISE EXCEPTION
            'controlled learning application definitions are immutable'
            USING ERRCODE = 'integrity_constraint_violation';
    END IF;
    IF NEW.attempt < OLD.attempt OR NEW.updated_at < OLD.updated_at THEN
        RAISE EXCEPTION
            'controlled learning application state must advance monotonically'
            USING ERRCODE = 'integrity_constraint_violation';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_controlled_learning_applications_guard_update
    BEFORE UPDATE ON public.controlled_learning_applications
    FOR EACH ROW
    EXECUTE FUNCTION public.hai_guard_controlled_learning_application_definition();

CREATE TRIGGER trg_controlled_learning_applications_no_delete
    BEFORE DELETE ON public.controlled_learning_applications
    FOR EACH ROW
    EXECUTE FUNCTION public.hai_reject_controlled_learning_mutation();

CREATE TRIGGER trg_controlled_learning_applications_no_truncate
    BEFORE TRUNCATE ON public.controlled_learning_applications
    FOR EACH STATEMENT
    EXECUTE FUNCTION public.hai_reject_controlled_learning_mutation();
