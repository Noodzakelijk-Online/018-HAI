ALTER TABLE public.robert_constitution_versions
    ADD CONSTRAINT uq_robert_constitution_owner_id_version
    UNIQUE (owner_identity, id, version);

ALTER TABLE public.standing_mandate_authorization_decisions
    ADD CONSTRAINT uq_standing_mandate_decision_owner_id_mandate
    UNIQUE (owner_identity, id, mandate_id);

ALTER TABLE public.task_review_decisions
    ADD CONSTRAINT uq_task_review_decision_owner_id
        UNIQUE (owner_identity, id),
    ADD CONSTRAINT uq_task_review_decision_owner_id_source
        UNIQUE (owner_identity, id, approval_source_id);

ALTER TABLE public.workflow_items
    ADD CONSTRAINT uq_workflow_item_owner_id
    UNIQUE (owner_identity, id);

ALTER TABLE public.workflow_decisions
    ADD COLUMN owner_identity character varying(256);

UPDATE public.workflow_decisions AS decisions
SET owner_identity = items.owner_identity
FROM public.workflow_items AS items
WHERE items.id = decisions.workflow_id;

DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM public.workflow_decisions
        WHERE owner_identity IS NULL OR btrim(owner_identity) = ''
    ) THEN
        RAISE EXCEPTION
            'workflow decisions without an owner-scoped workflow cannot be authorized';
    END IF;
END $$;

ALTER TABLE public.workflow_decisions
    ALTER COLUMN owner_identity SET NOT NULL,
    ADD CONSTRAINT uq_workflow_decision_owner_id
        UNIQUE (owner_identity, id),
    ADD CONSTRAINT fk_workflow_decision_owner_workflow
        FOREIGN KEY (owner_identity, workflow_id)
        REFERENCES public.workflow_items (owner_identity, id)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    ADD CONSTRAINT chk_workflow_decision_owner_identity CHECK (
        char_length(btrim(owner_identity)) BETWEEN 1 AND 256
    );

CREATE OR REPLACE FUNCTION public.hai_bind_workflow_decision_owner()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    workflow_owner character varying(256);
BEGIN
    SELECT owner_identity
    INTO workflow_owner
    FROM public.workflow_items
    WHERE id = NEW.workflow_id;

    IF workflow_owner IS NULL OR btrim(workflow_owner) = '' THEN
        RAISE EXCEPTION 'workflow decision requires an owner-scoped workflow'
            USING ERRCODE = 'foreign_key_violation';
    END IF;
    IF NEW.owner_identity IS NOT NULL
       AND btrim(NEW.owner_identity) <> ''
       AND NEW.owner_identity <> workflow_owner THEN
        RAISE EXCEPTION 'workflow decision owner does not match workflow owner'
            USING ERRCODE = 'foreign_key_violation';
    END IF;
    NEW.owner_identity := workflow_owner;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_workflow_decisions_bind_owner
    BEFORE INSERT OR UPDATE OF workflow_id, owner_identity
    ON public.workflow_decisions
    FOR EACH ROW
    EXECUTE FUNCTION public.hai_bind_workflow_decision_owner();

CREATE TABLE public.execution_authorization_receipts (
    id uuid NOT NULL,
    contract_version integer NOT NULL,
    owner_identity character varying(256) NOT NULL,
    idempotency_key character varying(256) NOT NULL,
    actor_identity character varying(256) NOT NULL,
    actor_kind character varying(16) NOT NULL,
    task_id character varying(256) NOT NULL,
    action character varying(256) NOT NULL,
    stage character varying(40) NOT NULL,
    resource_type character varying(256) NOT NULL,
    resource_id character varying(256) NOT NULL,
    project_key character varying(256) NOT NULL,
    runtime_id character varying(256) NOT NULL,
    approval_source_id character varying(256) NOT NULL,
    effect_digest character(64) NOT NULL,
    outcome character varying(32) NOT NULL,
    reason character varying(4096) NOT NULL,
    request_digest character(64) NOT NULL,
    decision_digest character(64) NOT NULL,
    required_authority smallint NOT NULL,
    requested_autonomy smallint NOT NULL,
    effective_autonomy smallint NOT NULL,
    risk character varying(16) NOT NULL,
    reversible boolean NOT NULL,
    estimated_cost_eur double precision NOT NULL,
    notification_required boolean NOT NULL,
    evaluated_at timestamp with time zone NOT NULL,
    evidence_json jsonb NOT NULL,
    constitution_id uuid,
    constitution_version integer,
    constitution_digest character(64),
    mandate_id uuid,
    mandate_decision_id uuid,
    agent_id character varying(256),
    assignment_id character varying(256),
    task_review_decision_id uuid,
    workflow_decision_id uuid,
    CONSTRAINT execution_authorization_receipts_pkey PRIMARY KEY (id),
    CONSTRAINT uq_execution_authorization_receipt_owner_id
        UNIQUE (owner_identity, id),
    CONSTRAINT uq_execution_authorization_receipt_idempotency
        UNIQUE (owner_identity, idempotency_key),
    CONSTRAINT uq_execution_authorization_receipt_owner_id_digest
        UNIQUE (owner_identity, id, decision_digest),
    CONSTRAINT uq_execution_authorization_receipt_owner_id_effect
        UNIQUE (owner_identity, id, effect_digest),
    CONSTRAINT fk_execution_authorization_receipt_constitution
        FOREIGN KEY (owner_identity, constitution_id, constitution_version)
        REFERENCES public.robert_constitution_versions
            (owner_identity, id, version)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT fk_execution_authorization_receipt_mandate
        FOREIGN KEY (owner_identity, mandate_id)
        REFERENCES public.standing_mandates (owner_identity, id)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT fk_execution_authorization_receipt_mandate_decision
        FOREIGN KEY (owner_identity, mandate_decision_id, mandate_id)
        REFERENCES public.standing_mandate_authorization_decisions
            (owner_identity, id, mandate_id)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT fk_execution_authorization_receipt_agent
        FOREIGN KEY (owner_identity, agent_id)
        REFERENCES public.agent_registry_agents (owner_identity, id)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT fk_execution_authorization_receipt_assignment
        FOREIGN KEY (owner_identity, assignment_id, agent_id)
        REFERENCES public.agent_registry_assignments
            (owner_identity, id, agent_id)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT fk_execution_authorization_receipt_task_review
        FOREIGN KEY (
            owner_identity,
            task_review_decision_id,
            approval_source_id
        )
        REFERENCES public.task_review_decisions (
            owner_identity,
            id,
            approval_source_id
        )
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT fk_execution_authorization_receipt_workflow_decision
        FOREIGN KEY (owner_identity, workflow_decision_id)
        REFERENCES public.workflow_decisions (owner_identity, id)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT chk_execution_authorization_receipt_identity CHECK (
        char_length(btrim(owner_identity)) BETWEEN 1 AND 256
        AND char_length(btrim(idempotency_key)) BETWEEN 1 AND 256
        AND char_length(btrim(actor_identity)) BETWEEN 1 AND 256
        AND char_length(btrim(task_id)) BETWEEN 1 AND 256
        AND char_length(btrim(action)) BETWEEN 1 AND 256
        AND char_length(btrim(resource_type)) BETWEEN 1 AND 256
        AND char_length(resource_id) <= 256
        AND char_length(project_key) <= 256
        AND char_length(runtime_id) <= 256
        AND char_length(approval_source_id) <= 256
    ),
    CONSTRAINT chk_execution_authorization_receipt_contract CHECK (
        contract_version > 0
    ),
    CONSTRAINT chk_execution_authorization_receipt_actor_kind CHECK (
        actor_kind IN ('system', 'agent', 'human')
    ),
    CONSTRAINT chk_execution_authorization_receipt_stage CHECK (
        stage IN (
            'data_access',
            'tool_use',
            'expenditure',
            'communication',
            'commitment',
            'execution',
            'publication',
            'deletion',
            'privilege_escalation',
            'self_modification'
        )
    ),
    CONSTRAINT chk_execution_authorization_receipt_outcome CHECK (
        outcome IN ('authorized', 'requires_approval', 'denied')
    ),
    CONSTRAINT chk_execution_authorization_receipt_reason CHECK (
        char_length(btrim(reason)) BETWEEN 1 AND 4096
    ),
    CONSTRAINT chk_execution_authorization_receipt_request_digest CHECK (
        request_digest ~ '^[0-9a-f]{64}$'
    ),
    CONSTRAINT chk_execution_authorization_receipt_decision_digest CHECK (
        decision_digest ~ '^[0-9a-f]{64}$'
    ),
    CONSTRAINT chk_execution_authorization_receipt_effect_digest CHECK (
        effect_digest ~ '^[0-9a-f]{64}$'
    ),
    CONSTRAINT chk_execution_authorization_receipt_authority CHECK (
        required_authority BETWEEN 0 AND 10
        AND requested_autonomy BETWEEN 0 AND 10
        AND effective_autonomy BETWEEN 0 AND 10
    ),
    CONSTRAINT chk_execution_authorization_receipt_risk CHECK (
        risk IN ('low', 'medium', 'high', 'critical')
    ),
    CONSTRAINT chk_execution_authorization_receipt_cost CHECK (
        estimated_cost_eur >= 0
        AND estimated_cost_eur <= 1000000
    ),
    CONSTRAINT chk_execution_authorization_receipt_constitution CHECK (
        (
            constitution_id IS NULL
            AND constitution_version IS NULL
            AND constitution_digest IS NULL
        )
        OR (
            constitution_id IS NOT NULL
            AND constitution_version > 0
            AND constitution_digest ~ '^[0-9a-f]{64}$'
        )
    ),
    CONSTRAINT chk_execution_authorization_receipt_mandate_binding CHECK (
        mandate_decision_id IS NULL OR mandate_id IS NOT NULL
    ),
    CONSTRAINT chk_execution_authorization_receipt_assignment_binding CHECK (
        assignment_id IS NULL OR agent_id IS NOT NULL
    ),
    CONSTRAINT chk_execution_authorization_receipt_approval_binding CHECK (
        (
            approval_source_id = ''
            AND task_review_decision_id IS NULL
            AND workflow_decision_id IS NULL
        )
        OR (
            task_review_decision_id IS NOT NULL
            AND workflow_decision_id IS NULL
            AND approval_source_id LIKE 'task-review:%'
        )
        OR (
            approval_source_id =
                'workflow-decision:' || workflow_decision_id::text
            AND task_review_decision_id IS NULL
            AND workflow_decision_id IS NOT NULL
        )
    ),
    CONSTRAINT chk_execution_authorization_receipt_evidence CHECK (
        jsonb_typeof(evidence_json) = 'object'
        AND octet_length(evidence_json::text) BETWEEN 2 AND 65536
        AND evidence_json ?& ARRAY[
            'emergencyStop',
            'constitution',
            'mandate',
            'agent',
            'approval',
            'reasonCodes',
            'trace'
        ]
    ),
    CONSTRAINT chk_execution_authorization_receipt_evidence_refs CHECK (
        (
            (
                COALESCE(evidence_json #>> '{constitution,source}', '')
                    LIKE 'builtin-%'
                AND constitution_id IS NULL
                AND constitution_version IS NULL
                AND constitution_digest IS NULL
            )
            OR (
                COALESCE(evidence_json #>> '{constitution,source}', '')
                    NOT LIKE 'builtin-%'
                AND COALESCE(evidence_json #>> '{constitution,id}', '') =
                    COALESCE(constitution_id::text, '')
                AND COALESCE(
                    (evidence_json #>> '{constitution,version}')::integer,
                    0
                ) = COALESCE(constitution_version, 0)
                AND COALESCE(evidence_json #>> '{constitution,digest}', '') =
                    COALESCE(constitution_digest, '')
            )
        )
        AND COALESCE(evidence_json #>> '{mandate,id}', '') =
            COALESCE(mandate_id::text, '')
        AND COALESCE(evidence_json #>> '{mandate,decisionId}', '') =
            COALESCE(mandate_decision_id::text, '')
        AND COALESCE(evidence_json #>> '{agent,agentId}', '') =
            COALESCE(agent_id, '')
        AND COALESCE(evidence_json #>> '{agent,assignmentId}', '') =
            COALESCE(assignment_id, '')
        AND (
            (
                approval_source_id = ''
                AND task_review_decision_id IS NULL
                AND workflow_decision_id IS NULL
                AND COALESCE(
                    evidence_json #>> '{approval,sourceId}',
                    ''
                ) = ''
                AND COALESCE(
                    evidence_json #>> '{approval,decisionId}',
                    ''
                ) = ''
            )
            OR (
                task_review_decision_id IS NOT NULL
                AND workflow_decision_id IS NULL
                AND evidence_json #>> '{approval,sourceId}' =
                    approval_source_id
                AND evidence_json #>> '{approval,decisionId}' =
                    task_review_decision_id::text
            )
            OR (
                task_review_decision_id IS NULL
                AND workflow_decision_id IS NOT NULL
                AND evidence_json #>> '{approval,sourceId}' =
                    approval_source_id
                AND evidence_json #>> '{approval,decisionId}' =
                    workflow_decision_id::text
            )
        )
    )
);

CREATE INDEX idx_execution_authorization_receipts_owner_evaluated
    ON public.execution_authorization_receipts
    (owner_identity, evaluated_at DESC, id DESC);
CREATE INDEX idx_execution_authorization_receipts_task
    ON public.execution_authorization_receipts
    (owner_identity, task_id, evaluated_at DESC);
CREATE INDEX idx_execution_authorization_receipts_action_stage
    ON public.execution_authorization_receipts
    (owner_identity, action, stage, evaluated_at DESC);
CREATE INDEX idx_execution_authorization_receipts_outcome
    ON public.execution_authorization_receipts
    (owner_identity, outcome, evaluated_at DESC);
CREATE INDEX idx_execution_authorization_receipts_request_digest
    ON public.execution_authorization_receipts
    (owner_identity, request_digest);
CREATE INDEX idx_execution_authorization_receipts_constitution
    ON public.execution_authorization_receipts
    (owner_identity, constitution_id, constitution_version)
    WHERE constitution_id IS NOT NULL;
CREATE INDEX idx_execution_authorization_receipts_mandate
    ON public.execution_authorization_receipts
    (owner_identity, mandate_id, evaluated_at DESC)
    WHERE mandate_id IS NOT NULL;
CREATE INDEX idx_execution_authorization_receipts_agent
    ON public.execution_authorization_receipts
    (owner_identity, agent_id, evaluated_at DESC)
    WHERE agent_id IS NOT NULL;
CREATE INDEX idx_execution_authorization_receipts_task_review
    ON public.execution_authorization_receipts
    (owner_identity, task_review_decision_id, evaluated_at DESC)
    WHERE task_review_decision_id IS NOT NULL;
CREATE INDEX idx_execution_authorization_receipts_workflow_decision
    ON public.execution_authorization_receipts
    (owner_identity, workflow_decision_id, evaluated_at DESC)
    WHERE workflow_decision_id IS NOT NULL;

CREATE TABLE public.execution_authorization_consumptions (
    owner_identity character varying(256) NOT NULL,
    receipt_id uuid NOT NULL,
    consumer character varying(256) NOT NULL,
    execution_target character varying(1024) NOT NULL,
    receipt_digest character(64) NOT NULL,
    consumed_at timestamp with time zone NOT NULL,
    CONSTRAINT execution_authorization_consumptions_pkey
        PRIMARY KEY (owner_identity, receipt_id),
    CONSTRAINT uq_execution_authorization_consumption_receipt_digest
        UNIQUE (owner_identity, receipt_id, receipt_digest),
    CONSTRAINT fk_execution_authorization_consumption_receipt
        FOREIGN KEY (owner_identity, receipt_id, receipt_digest)
        REFERENCES public.execution_authorization_receipts
            (owner_identity, id, decision_digest)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT chk_execution_authorization_consumption_identity CHECK (
        char_length(btrim(owner_identity)) BETWEEN 1 AND 256
        AND char_length(btrim(consumer)) BETWEEN 1 AND 256
        AND char_length(btrim(execution_target)) BETWEEN 1 AND 1024
    ),
    CONSTRAINT chk_execution_authorization_consumption_receipt_digest CHECK (
        receipt_digest ~ '^[0-9a-f]{64}$'
    )
);

CREATE INDEX idx_execution_authorization_consumptions_owner_consumed
    ON public.execution_authorization_consumptions
    (owner_identity, consumed_at DESC, receipt_id DESC);
CREATE INDEX idx_execution_authorization_consumptions_consumer
    ON public.execution_authorization_consumptions
    (owner_identity, consumer, consumed_at DESC);
CREATE INDEX idx_execution_authorization_consumptions_target
    ON public.execution_authorization_consumptions
    (owner_identity, execution_target, consumed_at DESC);

CREATE TABLE public.execution_authorization_final_effect_exercises (
    owner_identity character varying(256) NOT NULL,
    receipt_id uuid NOT NULL,
    runtime_id character varying(256) NOT NULL,
    task_id character varying(256) NOT NULL,
    action character varying(256) NOT NULL,
    resource_type character varying(256) NOT NULL,
    resource_id character varying(256) NOT NULL,
    project_key character varying(256) NOT NULL,
    approval_source_id character varying(256) NOT NULL,
    effect_digest character(64) NOT NULL,
    authorization_request_digest character(64) NOT NULL,
    decision_digest character(64) NOT NULL,
    runtime_request_digest character(64) NOT NULL,
    consumption_target character varying(1024) NOT NULL,
    exercised_at timestamp with time zone NOT NULL,
    CONSTRAINT execution_authorization_final_effect_exercises_pkey
        PRIMARY KEY (owner_identity, receipt_id),
    CONSTRAINT fk_execution_authorization_final_effect_receipt
        FOREIGN KEY (owner_identity, receipt_id, effect_digest)
        REFERENCES public.execution_authorization_receipts
            (owner_identity, id, effect_digest)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT fk_execution_authorization_final_effect_consumption
        FOREIGN KEY (owner_identity, receipt_id, decision_digest)
        REFERENCES public.execution_authorization_consumptions
            (owner_identity, receipt_id, receipt_digest)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT chk_execution_authorization_final_effect_identity CHECK (
        char_length(btrim(owner_identity)) BETWEEN 1 AND 256
        AND char_length(btrim(runtime_id)) BETWEEN 1 AND 256
        AND char_length(btrim(task_id)) BETWEEN 1 AND 256
        AND action = 'agent-runtime.execute-task'
        AND resource_type = 'agent-runtime-task'
        AND resource_id = task_id
        AND char_length(project_key) <= 256
        AND char_length(approval_source_id) <= 256
        AND char_length(btrim(consumption_target)) BETWEEN 1 AND 1024
    ),
    CONSTRAINT chk_execution_authorization_final_effect_digests CHECK (
        effect_digest ~ '^[0-9a-f]{64}$'
        AND authorization_request_digest ~ '^[0-9a-f]{64}$'
        AND decision_digest ~ '^[0-9a-f]{64}$'
        AND runtime_request_digest = effect_digest
        AND consumption_target = 'agent-runtime:' || effect_digest
    )
);

CREATE INDEX idx_execution_authorization_final_effect_runtime
    ON public.execution_authorization_final_effect_exercises
    (owner_identity, runtime_id, exercised_at DESC);
CREATE INDEX idx_execution_authorization_final_effect_task
    ON public.execution_authorization_final_effect_exercises
    (owner_identity, task_id, exercised_at DESC);

CREATE OR REPLACE FUNCTION public.hai_reject_execution_authorization_mutation()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE EXCEPTION 'execution authorization ledgers are append-only'
        USING ERRCODE = 'integrity_constraint_violation';
END;
$$;

CREATE TRIGGER trg_execution_authorization_receipts_immutable
    BEFORE UPDATE OR DELETE ON public.execution_authorization_receipts
    FOR EACH ROW
    EXECUTE FUNCTION public.hai_reject_execution_authorization_mutation();

CREATE TRIGGER trg_execution_authorization_receipts_no_truncate
    BEFORE TRUNCATE ON public.execution_authorization_receipts
    FOR EACH STATEMENT
    EXECUTE FUNCTION public.hai_reject_execution_authorization_mutation();

CREATE TRIGGER trg_execution_authorization_consumptions_immutable
    BEFORE UPDATE OR DELETE ON public.execution_authorization_consumptions
    FOR EACH ROW
    EXECUTE FUNCTION public.hai_reject_execution_authorization_mutation();

CREATE TRIGGER trg_execution_authorization_consumptions_no_truncate
    BEFORE TRUNCATE ON public.execution_authorization_consumptions
    FOR EACH STATEMENT
    EXECUTE FUNCTION public.hai_reject_execution_authorization_mutation();

CREATE TRIGGER trg_execution_authorization_final_effects_immutable
    BEFORE UPDATE OR DELETE
    ON public.execution_authorization_final_effect_exercises
    FOR EACH ROW
    EXECUTE FUNCTION public.hai_reject_execution_authorization_mutation();

CREATE TRIGGER trg_execution_authorization_final_effects_no_truncate
    BEFORE TRUNCATE
    ON public.execution_authorization_final_effect_exercises
    FOR EACH STATEMENT
    EXECUTE FUNCTION public.hai_reject_execution_authorization_mutation();
