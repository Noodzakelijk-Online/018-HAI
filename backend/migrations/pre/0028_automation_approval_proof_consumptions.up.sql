CREATE TABLE public.automation_approval_proof_consumptions (
    contract_version character varying(64) NOT NULL,
    owner_identity character varying(256) NOT NULL,
    proof_id uuid NOT NULL,
    automation_id uuid NOT NULL,
    action_digest character(64) NOT NULL,
    scope character varying(64) NOT NULL,
    approval_source_id character varying(256) NOT NULL,
    nonce_digest character(64) NOT NULL,
    signature_digest character(64) NOT NULL,
    record_digest character(64) NOT NULL,
    issued_at timestamp with time zone NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    consumed_at timestamp with time zone NOT NULL,
    CONSTRAINT automation_approval_proof_consumptions_pkey
        PRIMARY KEY (owner_identity, proof_id),
    CONSTRAINT chk_automation_approval_proof_contract CHECK (
        contract_version = 'automation-approval-proof-consumption.v1'
    ),
    CONSTRAINT chk_automation_approval_proof_owner CHECK (
        length(btrim(owner_identity)) BETWEEN 1 AND 256
        AND owner_identity !~ E'[\\r\\n\\x00]'
    ),
    CONSTRAINT chk_automation_approval_proof_digests CHECK (
        action_digest ~ '^[0-9a-f]{64}$'
        AND nonce_digest ~ '^[0-9a-f]{64}$'
        AND signature_digest ~ '^[0-9a-f]{64}$'
        AND record_digest ~ '^[0-9a-f]{64}$'
    ),
    CONSTRAINT chk_automation_approval_proof_scope CHECK (
        scope IN (
            'automation.api.mutate',
            'automation.script.execute',
            'automation.docker.start',
            'automation.agent-runtime.execute'
        )
    ),
    CONSTRAINT chk_automation_approval_proof_source CHECK (
        length(btrim(approval_source_id)) BETWEEN 1 AND 256
        AND approval_source_id !~ E'[\\r\\n\\x00]'
        AND (
            approval_source_id LIKE 'task-review:%'
            OR approval_source_id LIKE 'workflow-decision:%'
        )
    ),
    CONSTRAINT chk_automation_approval_proof_time CHECK (
        expires_at > issued_at
        AND expires_at <= issued_at + interval '15 minutes'
        AND consumed_at >= issued_at - interval '5 seconds'
        AND consumed_at < expires_at
    )
);

CREATE INDEX idx_automation_approval_proof_consumptions_owner_time
    ON public.automation_approval_proof_consumptions
    (owner_identity, consumed_at DESC, proof_id ASC);

CREATE TRIGGER trg_automation_approval_proof_consumptions_immutable
BEFORE UPDATE OR DELETE ON public.automation_approval_proof_consumptions
FOR EACH ROW EXECUTE FUNCTION public.hai_reject_execution_authorization_mutation();

CREATE TRIGGER trg_automation_approval_proof_consumptions_no_truncate
BEFORE TRUNCATE ON public.automation_approval_proof_consumptions
FOR EACH STATEMENT EXECUTE FUNCTION public.hai_reject_execution_authorization_mutation();
