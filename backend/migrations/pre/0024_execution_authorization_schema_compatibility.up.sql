-- Older local installations applied an earlier 0014 receipt schema. The
-- repository now binds receipts to projects, runtimes, exact approval sources,
-- workflow decisions, and one final effect. Upgrade those installations in
-- place while remaining a no-op for fresh databases created by current 0014.

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'uq_task_review_decision_owner_id_source'
          AND conrelid = 'public.task_review_decisions'::regclass
    ) THEN
        ALTER TABLE public.task_review_decisions
            ADD CONSTRAINT uq_task_review_decision_owner_id_source
            UNIQUE (owner_identity, id, approval_source_id);
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'uq_workflow_item_owner_id'
          AND conrelid = 'public.workflow_items'::regclass
    ) THEN
        ALTER TABLE public.workflow_items
            ADD CONSTRAINT uq_workflow_item_owner_id
            UNIQUE (owner_identity, id);
    END IF;
END;
$$;

ALTER TABLE public.workflow_decisions
    ADD COLUMN IF NOT EXISTS owner_identity character varying(256);

UPDATE public.workflow_decisions AS decisions
SET owner_identity = items.owner_identity
FROM public.workflow_items AS items
WHERE items.id = decisions.workflow_id
  AND decisions.owner_identity IS NULL;

DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM public.workflow_decisions
        WHERE owner_identity IS NULL OR btrim(owner_identity) = ''
    ) THEN
        RAISE EXCEPTION
            'workflow decisions without an owner-scoped workflow cannot be authorized';
    END IF;
END;
$$;

ALTER TABLE public.workflow_decisions
    ALTER COLUMN owner_identity SET NOT NULL;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'uq_workflow_decision_owner_id'
          AND conrelid = 'public.workflow_decisions'::regclass
    ) THEN
        ALTER TABLE public.workflow_decisions
            ADD CONSTRAINT uq_workflow_decision_owner_id
            UNIQUE (owner_identity, id);
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'fk_workflow_decision_owner_workflow'
          AND conrelid = 'public.workflow_decisions'::regclass
    ) THEN
        ALTER TABLE public.workflow_decisions
            ADD CONSTRAINT fk_workflow_decision_owner_workflow
            FOREIGN KEY (owner_identity, workflow_id)
            REFERENCES public.workflow_items (owner_identity, id)
            ON UPDATE RESTRICT ON DELETE RESTRICT;
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'chk_workflow_decision_owner_identity'
          AND conrelid = 'public.workflow_decisions'::regclass
    ) THEN
        ALTER TABLE public.workflow_decisions
            ADD CONSTRAINT chk_workflow_decision_owner_identity CHECK (
                char_length(btrim(owner_identity)) BETWEEN 1 AND 256
            );
    END IF;
END;
$$;

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

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_trigger
        WHERE tgname = 'trg_workflow_decisions_bind_owner'
          AND tgrelid = 'public.workflow_decisions'::regclass
    ) THEN
        CREATE TRIGGER trg_workflow_decisions_bind_owner
            BEFORE INSERT OR UPDATE OF workflow_id, owner_identity
            ON public.workflow_decisions
            FOR EACH ROW
            EXECUTE FUNCTION public.hai_bind_workflow_decision_owner();
    END IF;
END;
$$;

ALTER TABLE public.execution_authorization_receipts
    ADD COLUMN IF NOT EXISTS project_key character varying(256),
    ADD COLUMN IF NOT EXISTS runtime_id character varying(256),
    ADD COLUMN IF NOT EXISTS approval_source_id character varying(256),
    ADD COLUMN IF NOT EXISTS effect_digest character(64),
    ADD COLUMN IF NOT EXISTS task_review_decision_id uuid,
    ADD COLUMN IF NOT EXISTS workflow_decision_id uuid;

ALTER TABLE public.execution_authorization_receipts
    DISABLE TRIGGER trg_execution_authorization_receipts_immutable;

UPDATE public.execution_authorization_receipts
SET project_key = COALESCE(project_key, ''),
    runtime_id = COALESCE(runtime_id, ''),
    effect_digest = COALESCE(effect_digest, request_digest),
    approval_source_id = COALESCE(approval_source_id, '');

DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_schema = 'public'
          AND table_name = 'execution_authorization_receipts'
          AND column_name = 'approval_decision_id'
    ) THEN
        UPDATE public.execution_authorization_receipts AS receipts
        SET task_review_decision_id = COALESCE(
                receipts.task_review_decision_id,
                receipts.approval_decision_id
            ),
            approval_source_id = CASE
                WHEN receipts.approval_decision_id IS NULL
                    THEN receipts.approval_source_id
                ELSE COALESCE(
                    (
                        SELECT decisions.approval_source_id
                        FROM public.task_review_decisions AS decisions
                        WHERE decisions.owner_identity = receipts.owner_identity
                          AND decisions.id = receipts.approval_decision_id
                    ),
                    receipts.approval_source_id,
                    ''
                )
            END;
    END IF;
END;
$$;

UPDATE public.execution_authorization_receipts
SET evidence_json = jsonb_set(
        evidence_json,
        '{approval,sourceId}',
        to_jsonb(approval_source_id::text),
        true
    )
WHERE COALESCE(evidence_json #>> '{approval,sourceId}', '')
      IS DISTINCT FROM approval_source_id;

ALTER TABLE public.execution_authorization_receipts
    ENABLE TRIGGER trg_execution_authorization_receipts_immutable;

ALTER TABLE public.execution_authorization_receipts
    ALTER COLUMN project_key SET NOT NULL,
    ALTER COLUMN runtime_id SET NOT NULL,
    ALTER COLUMN approval_source_id SET NOT NULL,
    ALTER COLUMN effect_digest SET NOT NULL;

ALTER TABLE public.execution_authorization_receipts
    DROP CONSTRAINT IF EXISTS fk_execution_authorization_receipt_approval,
    DROP CONSTRAINT IF EXISTS chk_execution_authorization_receipt_identity,
    DROP CONSTRAINT IF EXISTS chk_execution_authorization_receipt_effect_digest,
    DROP CONSTRAINT IF EXISTS chk_execution_authorization_receipt_approval_binding,
    DROP CONSTRAINT IF EXISTS chk_execution_authorization_receipt_evidence_refs;

DROP INDEX IF EXISTS public.idx_execution_authorization_receipts_approval;

ALTER TABLE public.execution_authorization_receipts
    DROP COLUMN IF EXISTS approval_decision_id;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'uq_execution_authorization_receipt_owner_id_effect'
          AND conrelid = 'public.execution_authorization_receipts'::regclass
    ) THEN
        ALTER TABLE public.execution_authorization_receipts
            ADD CONSTRAINT uq_execution_authorization_receipt_owner_id_effect
            UNIQUE (owner_identity, id, effect_digest);
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'fk_execution_authorization_receipt_task_review'
          AND conrelid = 'public.execution_authorization_receipts'::regclass
    ) THEN
        ALTER TABLE public.execution_authorization_receipts
            ADD CONSTRAINT fk_execution_authorization_receipt_task_review
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
            ON UPDATE RESTRICT ON DELETE RESTRICT;
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'fk_execution_authorization_receipt_workflow_decision'
          AND conrelid = 'public.execution_authorization_receipts'::regclass
    ) THEN
        ALTER TABLE public.execution_authorization_receipts
            ADD CONSTRAINT fk_execution_authorization_receipt_workflow_decision
            FOREIGN KEY (owner_identity, workflow_decision_id)
            REFERENCES public.workflow_decisions (owner_identity, id)
            ON UPDATE RESTRICT ON DELETE RESTRICT;
    END IF;
END;
$$;

ALTER TABLE public.execution_authorization_receipts
    ADD CONSTRAINT chk_execution_authorization_receipt_identity CHECK (
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
    ADD CONSTRAINT chk_execution_authorization_receipt_effect_digest CHECK (
        effect_digest ~ '^[0-9a-f]{64}$'
    ),
    ADD CONSTRAINT chk_execution_authorization_receipt_approval_binding CHECK (
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
    ADD CONSTRAINT chk_execution_authorization_receipt_evidence_refs CHECK (
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
        AND COALESCE(evidence_json #>> '{approval,sourceId}', '') =
            approval_source_id
        AND COALESCE(evidence_json #>> '{approval,decisionId}', '') =
            COALESCE(
                task_review_decision_id::text,
                workflow_decision_id::text,
                ''
            )
    );

CREATE INDEX IF NOT EXISTS idx_execution_authorization_receipts_task_review
    ON public.execution_authorization_receipts
    (owner_identity, task_review_decision_id, evaluated_at DESC)
    WHERE task_review_decision_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_execution_authorization_receipts_workflow_decision
    ON public.execution_authorization_receipts
    (owner_identity, workflow_decision_id, evaluated_at DESC)
    WHERE workflow_decision_id IS NOT NULL;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname =
              'uq_execution_authorization_consumption_receipt_digest'
          AND conrelid =
              'public.execution_authorization_consumptions'::regclass
    ) THEN
        ALTER TABLE public.execution_authorization_consumptions
            ADD CONSTRAINT
                uq_execution_authorization_consumption_receipt_digest
            UNIQUE (owner_identity, receipt_id, receipt_digest);
    END IF;
END;
$$;

CREATE TABLE IF NOT EXISTS public.execution_authorization_final_effect_exercises (
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

CREATE INDEX IF NOT EXISTS idx_execution_authorization_final_effect_runtime
    ON public.execution_authorization_final_effect_exercises
    (owner_identity, runtime_id, exercised_at DESC);
CREATE INDEX IF NOT EXISTS idx_execution_authorization_final_effect_task
    ON public.execution_authorization_final_effect_exercises
    (owner_identity, task_id, exercised_at DESC);

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_trigger
        WHERE tgname = 'trg_execution_authorization_final_effects_immutable'
          AND tgrelid =
              'public.execution_authorization_final_effect_exercises'::regclass
    ) THEN
        CREATE TRIGGER trg_execution_authorization_final_effects_immutable
            BEFORE UPDATE OR DELETE
            ON public.execution_authorization_final_effect_exercises
            FOR EACH ROW
            EXECUTE FUNCTION public.hai_reject_execution_authorization_mutation();
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM pg_trigger
        WHERE tgname = 'trg_execution_authorization_final_effects_no_truncate'
          AND tgrelid =
              'public.execution_authorization_final_effect_exercises'::regclass
    ) THEN
        CREATE TRIGGER trg_execution_authorization_final_effects_no_truncate
            BEFORE TRUNCATE
            ON public.execution_authorization_final_effect_exercises
            FOR EACH STATEMENT
            EXECUTE FUNCTION public.hai_reject_execution_authorization_mutation();
    END IF;
END;
$$;
