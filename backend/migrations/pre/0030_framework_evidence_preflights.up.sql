CREATE FUNCTION public.hai_reject_framework_evidence_preflight_mutation()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE EXCEPTION 'framework evidence preflight records are append-only'
        USING ERRCODE = 'object_not_in_prerequisite_state';
END;
$$;

CREATE FUNCTION public.hai_framework_evidence_json_bytes_valid(value bytea)
RETURNS boolean
LANGUAGE plpgsql
IMMUTABLE
STRICT
AS $$
BEGIN
    PERFORM convert_from(value, 'UTF8')::jsonb;
    RETURN true;
EXCEPTION WHEN others THEN
    RETURN false;
END;
$$;

CREATE TABLE public.framework_evidence_preflights (
    contract_version smallint NOT NULL,
    owner_identity character varying(256) NOT NULL,
    task_plan_id character varying(256) NOT NULL,
    framework_selection_id character varying(256) NOT NULL,
    preflight_digest character(64) NOT NULL,
    status character varying(16) NOT NULL,
    assertions_json bytea NOT NULL,
    evaluated_at timestamp with time zone NOT NULL,
    created_at timestamp with time zone NOT NULL,
    CONSTRAINT framework_evidence_preflights_pkey PRIMARY KEY (
        owner_identity,
        task_plan_id,
        framework_selection_id,
        preflight_digest
    ),
    CONSTRAINT chk_framework_evidence_preflight_contract CHECK (
        contract_version = 1
    ),
    CONSTRAINT chk_framework_evidence_preflight_owner CHECK (
        length(btrim(owner_identity)) BETWEEN 1 AND 256
        AND owner_identity = btrim(owner_identity)
        AND owner_identity !~ E'[\\r\\n\\x00]'
    ),
    CONSTRAINT chk_framework_evidence_preflight_task CHECK (
        length(btrim(task_plan_id)) BETWEEN 1 AND 256
        AND task_plan_id = btrim(task_plan_id)
        AND task_plan_id !~ E'[\\r\\n\\x00]'
    ),
    CONSTRAINT chk_framework_evidence_preflight_selection CHECK (
        length(btrim(framework_selection_id)) BETWEEN 1 AND 256
        AND framework_selection_id = btrim(framework_selection_id)
        AND framework_selection_id !~ E'[\\r\\n\\x00]'
    ),
    CONSTRAINT chk_framework_evidence_preflight_digest CHECK (
        preflight_digest ~ '^[0-9a-f]{64}$'
    ),
    CONSTRAINT chk_framework_evidence_preflight_status CHECK (
        status = 'passed'
    ),
    CONSTRAINT chk_framework_evidence_preflight_payload_size CHECK (
        octet_length(assertions_json) BETWEEN 1 AND 1048576
        AND public.hai_framework_evidence_json_bytes_valid(assertions_json)
    )
);

CREATE INDEX idx_framework_evidence_preflights_owner_evaluated
    ON public.framework_evidence_preflights
    (owner_identity, evaluated_at DESC, task_plan_id, framework_selection_id);

CREATE TRIGGER trg_framework_evidence_preflights_immutable
BEFORE UPDATE OR DELETE ON public.framework_evidence_preflights
FOR EACH ROW EXECUTE FUNCTION public.hai_reject_framework_evidence_preflight_mutation();

CREATE TRIGGER trg_framework_evidence_preflights_no_truncate
BEFORE TRUNCATE ON public.framework_evidence_preflights
FOR EACH STATEMENT EXECUTE FUNCTION public.hai_reject_framework_evidence_preflight_mutation();
