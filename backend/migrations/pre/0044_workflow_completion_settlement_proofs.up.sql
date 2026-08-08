CREATE TABLE public.workflow_completion_attestations (
    id uuid PRIMARY KEY DEFAULT public.uuid_generate_v4(),
    workflow_id uuid NOT NULL UNIQUE,
    owner_identity character varying(255) NOT NULL,
    task_plan_id character varying(120) NOT NULL,
    completion_status character varying(32) NOT NULL,
    verification_status character varying(80) NOT NULL,
    runtime_id character varying(256) NOT NULL,
    runtime_evidence_uri character varying(2048) NOT NULL,
    runtime_evidence_digest character(64) NOT NULL,
    result_digest character(64) NOT NULL,
    record_digest character(64) NOT NULL,
    completed_at timestamp with time zone NOT NULL,
    attested_at timestamp with time zone NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT uq_workflow_completion_attestation_owner_id_digest
        UNIQUE (owner_identity, id, record_digest),
    CONSTRAINT uq_workflow_completion_attestation_owner_record_digest
        UNIQUE (owner_identity, record_digest),
    CONSTRAINT fk_workflow_completion_attestation_workflow
        FOREIGN KEY (workflow_id)
        REFERENCES public.workflow_items (id)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT chk_workflow_completion_attestation_identity CHECK (
        char_length(btrim(owner_identity)) BETWEEN 1 AND 255
        AND char_length(btrim(task_plan_id)) BETWEEN 1 AND 120
        AND char_length(btrim(runtime_id)) BETWEEN 1 AND 256
        AND char_length(btrim(runtime_evidence_uri)) BETWEEN 1 AND 2048
    ),
    CONSTRAINT chk_workflow_completion_attestation_status CHECK (
        completion_status = 'completed'
        AND verification_status IN ('verified', 'test_passed')
    ),
    CONSTRAINT chk_workflow_completion_attestation_digests CHECK (
        runtime_evidence_digest ~ '^[0-9a-f]{64}$'
        AND result_digest ~ '^[0-9a-f]{64}$'
        AND record_digest ~ '^[0-9a-f]{64}$'
    ),
    CONSTRAINT chk_workflow_completion_attestation_time CHECK (
        completed_at <= attested_at
    )
);

CREATE INDEX idx_workflow_completion_attestations_owner_time
    ON public.workflow_completion_attestations
    (owner_identity, attested_at DESC, id DESC);

CREATE OR REPLACE FUNCTION public.validate_workflow_completion_attestation_insert()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    workflow_owner character varying(255);
    workflow_state character varying(80);
    workflow_task_plan_id character varying(120);
    workflow_verification character varying(80);
    workflow_completed_at timestamp with time zone;
BEGIN
    SELECT
        owner_identity,
        current_state,
        last_task_plan_id,
        verification_status,
        completed_at
    INTO
        workflow_owner,
        workflow_state,
        workflow_task_plan_id,
        workflow_verification,
        workflow_completed_at
    FROM public.workflow_items
    WHERE id = NEW.workflow_id
    FOR KEY SHARE;

    IF NOT FOUND
       OR COALESCE(NULLIF(workflow_owner, ''), 'system') <> NEW.owner_identity
       OR workflow_state <> NEW.completion_status
       OR workflow_task_plan_id IS NULL
       OR workflow_task_plan_id <> NEW.task_plan_id
       OR workflow_verification IS NULL
       OR workflow_verification <> NEW.verification_status
       OR workflow_completed_at IS NULL
       OR workflow_completed_at <> NEW.completed_at THEN
        RAISE EXCEPTION
            'workflow completion attestation does not match the completed owner-scoped workflow';
    END IF;

    RETURN NEW;
END;
$$;

CREATE TRIGGER workflow_completion_attestations_validate_insert
    BEFORE INSERT ON public.workflow_completion_attestations
    FOR EACH ROW
    EXECUTE FUNCTION public.validate_workflow_completion_attestation_insert();

CREATE OR REPLACE FUNCTION public.reject_workflow_completion_settlement_proof_mutation()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE EXCEPTION 'workflow completion and portfolio settlement proofs are append-only'
        USING ERRCODE = '55000';
END;
$$;

CREATE TRIGGER workflow_completion_attestations_reject_update
    BEFORE UPDATE ON public.workflow_completion_attestations
    FOR EACH ROW
    EXECUTE FUNCTION public.reject_workflow_completion_settlement_proof_mutation();
CREATE TRIGGER workflow_completion_attestations_reject_delete
    BEFORE DELETE ON public.workflow_completion_attestations
    FOR EACH ROW
    EXECUTE FUNCTION public.reject_workflow_completion_settlement_proof_mutation();
CREATE TRIGGER workflow_completion_attestations_reject_truncate
    BEFORE TRUNCATE ON public.workflow_completion_attestations
    FOR EACH STATEMENT
    EXECUTE FUNCTION public.reject_workflow_completion_settlement_proof_mutation();

CREATE TABLE public.pursuit_portfolio_workflow_settlement_proofs (
    id uuid PRIMARY KEY DEFAULT public.uuid_generate_v4(),
    settlement_id uuid NOT NULL UNIQUE,
    settlement_digest character(64) NOT NULL,
    reservation_id uuid NOT NULL UNIQUE,
    pursuit_id uuid NOT NULL,
    owner_identity character varying(255) NOT NULL,
    proposal_item_id uuid NOT NULL UNIQUE,
    proposal_item_digest character(64) NOT NULL,
    approval_decision_id uuid NOT NULL,
    approval_decision_digest character(64) NOT NULL,
    authorization_receipt_id uuid NOT NULL,
    authorization_receipt_digest character(64) NOT NULL,
    authorization_consumption_digest character(64) NOT NULL,
    authorization_consumption_target character varying(1024) NOT NULL,
    workflow_id uuid NOT NULL UNIQUE,
    completion_attestation_id uuid NOT NULL UNIQUE,
    completion_attestation_digest character(64) NOT NULL,
    request_digest character(64) NOT NULL,
    record_digest character(64) NOT NULL,
    actual_effort_minutes bigint NOT NULL DEFAULT 0,
    actual_cost_micros bigint NOT NULL DEFAULT 0,
    currency character varying(3) NOT NULL DEFAULT '',
    actor character varying(255) NOT NULL,
    settled_at timestamp with time zone NOT NULL,
    CONSTRAINT uq_portfolio_workflow_settlement_proof_owner_request
        UNIQUE (owner_identity, request_digest),
    CONSTRAINT fk_portfolio_workflow_settlement_proof_settlement
        FOREIGN KEY (settlement_id)
        REFERENCES public.pursuit_resource_reservation_settlements (id)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT fk_portfolio_workflow_settlement_proof_reservation
        FOREIGN KEY (reservation_id)
        REFERENCES public.pursuit_resource_reservations (id)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT fk_portfolio_workflow_settlement_proof_pursuit
        FOREIGN KEY (pursuit_id)
        REFERENCES public.pursuits (id)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT fk_portfolio_workflow_settlement_proof_proposal_item
        FOREIGN KEY (proposal_item_id)
        REFERENCES public.pursuit_portfolio_execution_proposal_items (id)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT fk_portfolio_workflow_settlement_proof_approval_decision
        FOREIGN KEY (approval_decision_id)
        REFERENCES public.pursuit_portfolio_execution_proposal_decisions (id)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT fk_portfolio_workflow_settlement_proof_authorization_receipt
        FOREIGN KEY (owner_identity, authorization_receipt_id, authorization_receipt_digest)
        REFERENCES public.execution_authorization_receipts
            (owner_identity, id, decision_digest)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT fk_portfolio_workflow_settlement_proof_authorization_consumption
        FOREIGN KEY (
            owner_identity,
            authorization_receipt_id,
            authorization_consumption_digest
        )
        REFERENCES public.execution_authorization_consumptions
            (owner_identity, receipt_id, receipt_digest)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT fk_portfolio_workflow_settlement_proof_workflow
        FOREIGN KEY (workflow_id)
        REFERENCES public.workflow_items (id)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT fk_portfolio_workflow_settlement_proof_attestation
        FOREIGN KEY (
            owner_identity,
            completion_attestation_id,
            completion_attestation_digest
        )
        REFERENCES public.workflow_completion_attestations
            (owner_identity, id, record_digest)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT chk_portfolio_workflow_settlement_proof_identity CHECK (
        char_length(btrim(owner_identity)) BETWEEN 1 AND 255
        AND char_length(btrim(authorization_consumption_target)) BETWEEN 1 AND 1024
        AND char_length(btrim(actor)) BETWEEN 1 AND 255
    ),
    CONSTRAINT chk_portfolio_workflow_settlement_proof_digests CHECK (
        settlement_digest ~ '^[0-9a-f]{64}$'
        AND proposal_item_digest ~ '^[0-9a-f]{64}$'
        AND approval_decision_digest ~ '^[0-9a-f]{64}$'
        AND authorization_receipt_digest ~ '^[0-9a-f]{64}$'
        AND authorization_consumption_digest ~ '^[0-9a-f]{64}$'
        AND completion_attestation_digest ~ '^[0-9a-f]{64}$'
        AND request_digest ~ '^[0-9a-f]{64}$'
        AND record_digest ~ '^[0-9a-f]{64}$'
    ),
    CONSTRAINT chk_portfolio_workflow_settlement_proof_receipt_consumption CHECK (
        authorization_receipt_digest = authorization_consumption_digest
    ),
    CONSTRAINT chk_portfolio_workflow_settlement_proof_actual_usage CHECK (
        actual_effort_minutes >= 0
        AND actual_cost_micros >= 0
        AND (
            (actual_cost_micros = 0 AND currency = '')
            OR (actual_cost_micros > 0 AND currency = 'EUR')
        )
    )
);

CREATE INDEX idx_portfolio_workflow_settlement_proofs_owner_pursuit_time
    ON public.pursuit_portfolio_workflow_settlement_proofs
    (owner_identity, pursuit_id, settled_at DESC, id DESC);

CREATE INDEX idx_portfolio_workflow_settlement_proofs_receipt
    ON public.pursuit_portfolio_workflow_settlement_proofs
    (owner_identity, authorization_receipt_id);

CREATE OR REPLACE FUNCTION public.validate_portfolio_workflow_settlement_proof_insert()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    settlement_record public.pursuit_resource_reservation_settlements%ROWTYPE;
    proposal_record public.pursuit_portfolio_execution_proposal_items%ROWTYPE;
    decision_record public.pursuit_portfolio_execution_proposal_decisions%ROWTYPE;
    receipt_record public.execution_authorization_receipts%ROWTYPE;
    consumption_record public.execution_authorization_consumptions%ROWTYPE;
    attestation_record public.workflow_completion_attestations%ROWTYPE;
BEGIN
    SELECT * INTO settlement_record
    FROM public.pursuit_resource_reservation_settlements
    WHERE id = NEW.settlement_id
    FOR KEY SHARE;

    IF NOT FOUND
       OR settlement_record.reservation_id <> NEW.reservation_id
       OR settlement_record.pursuit_id <> NEW.pursuit_id
       OR settlement_record.owner_identity <> NEW.owner_identity
       OR settlement_record.disposition <> 'consumed'
       OR settlement_record.actual_effort_minutes <> NEW.actual_effort_minutes
       OR settlement_record.actual_cost_micros <> NEW.actual_cost_micros
       OR settlement_record.currency <> NEW.currency
       OR settlement_record.actor <> NEW.actor
       OR settlement_record.record_digest <> NEW.settlement_digest
       OR settlement_record.settled_at <> NEW.settled_at THEN
        RAISE EXCEPTION 'portfolio workflow settlement proof does not match its immutable settlement';
    END IF;

    SELECT * INTO proposal_record
    FROM public.pursuit_portfolio_execution_proposal_items
    WHERE id = NEW.proposal_item_id
    FOR KEY SHARE;

    IF NOT FOUND
       OR proposal_record.reservation_id <> NEW.reservation_id
       OR proposal_record.pursuit_id <> NEW.pursuit_id
       OR proposal_record.owner_identity <> NEW.owner_identity
       OR proposal_record.record_digest <> NEW.proposal_item_digest
       OR proposal_record.status = 'blocked' THEN
        RAISE EXCEPTION 'portfolio workflow settlement proof does not match its immutable proposal item';
    END IF;

    SELECT * INTO decision_record
    FROM public.pursuit_portfolio_execution_proposal_decisions
    WHERE id = NEW.approval_decision_id
    FOR KEY SHARE;

    IF NOT FOUND
       OR decision_record.proposal_item_id <> NEW.proposal_item_id
       OR decision_record.pursuit_id <> NEW.pursuit_id
       OR decision_record.owner_identity <> NEW.owner_identity
       OR decision_record.proposal_item_digest <> NEW.proposal_item_digest
       OR decision_record.record_digest <> NEW.approval_decision_digest
       OR decision_record.decision <> 'approved' THEN
        RAISE EXCEPTION 'portfolio workflow settlement proof does not match an approved immutable decision';
    END IF;

    SELECT * INTO receipt_record
    FROM public.execution_authorization_receipts
    WHERE owner_identity = NEW.owner_identity
      AND id = NEW.authorization_receipt_id
    FOR KEY SHARE;

    IF NOT FOUND
       OR receipt_record.portfolio_proposal_decision_id <> NEW.approval_decision_id
       OR receipt_record.decision_digest <> NEW.authorization_receipt_digest
       OR receipt_record.outcome <> 'authorized'
       OR receipt_record.evaluated_at > decision_record.expires_at THEN
        RAISE EXCEPTION 'portfolio workflow settlement proof does not match its authorization receipt';
    END IF;

    SELECT * INTO consumption_record
    FROM public.execution_authorization_consumptions
    WHERE owner_identity = NEW.owner_identity
      AND receipt_id = NEW.authorization_receipt_id
    FOR KEY SHARE;

    IF NOT FOUND
       OR consumption_record.receipt_digest <> NEW.authorization_consumption_digest
       OR consumption_record.execution_target <> NEW.authorization_consumption_target
       OR consumption_record.consumed_at < receipt_record.evaluated_at THEN
        RAISE EXCEPTION 'portfolio workflow settlement proof does not match the authorization consumption';
    END IF;

    SELECT * INTO attestation_record
    FROM public.workflow_completion_attestations
    WHERE id = NEW.completion_attestation_id
    FOR KEY SHARE;

    IF NOT FOUND
       OR attestation_record.workflow_id <> NEW.workflow_id
       OR attestation_record.owner_identity <> NEW.owner_identity
       OR attestation_record.record_digest <> NEW.completion_attestation_digest
       OR attestation_record.completion_status <> 'completed'
       OR attestation_record.verification_status NOT IN ('verified', 'test_passed')
       OR attestation_record.attested_at > NEW.settled_at
       OR attestation_record.completed_at < consumption_record.consumed_at THEN
        RAISE EXCEPTION 'portfolio workflow settlement proof does not match verified completion evidence';
    END IF;

    RETURN NEW;
END;
$$;

CREATE TRIGGER pursuit_portfolio_workflow_settlement_proofs_validate_insert
    BEFORE INSERT ON public.pursuit_portfolio_workflow_settlement_proofs
    FOR EACH ROW
    EXECUTE FUNCTION public.validate_portfolio_workflow_settlement_proof_insert();

CREATE TRIGGER pursuit_portfolio_workflow_settlement_proofs_reject_update
    BEFORE UPDATE ON public.pursuit_portfolio_workflow_settlement_proofs
    FOR EACH ROW
    EXECUTE FUNCTION public.reject_workflow_completion_settlement_proof_mutation();
CREATE TRIGGER pursuit_portfolio_workflow_settlement_proofs_reject_delete
    BEFORE DELETE ON public.pursuit_portfolio_workflow_settlement_proofs
    FOR EACH ROW
    EXECUTE FUNCTION public.reject_workflow_completion_settlement_proof_mutation();
CREATE TRIGGER pursuit_portfolio_workflow_settlement_proofs_reject_truncate
    BEFORE TRUNCATE ON public.pursuit_portfolio_workflow_settlement_proofs
    FOR EACH STATEMENT
    EXECUTE FUNCTION public.reject_workflow_completion_settlement_proof_mutation();
