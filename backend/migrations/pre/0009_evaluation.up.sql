CREATE TABLE IF NOT EXISTS public.evaluation_datasets (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    owner_identity character varying(255) NOT NULL,
    dataset_id character varying(128) NOT NULL,
    dataset_version integer NOT NULL,
    schema_version integer NOT NULL,
    name character varying(255) NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    created_at_value character varying(64) NOT NULL,
    digest character(64) NOT NULL,
    recorded_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    CONSTRAINT evaluation_datasets_pkey PRIMARY KEY (id),
    CONSTRAINT uq_evaluation_datasets_owner_key
        UNIQUE (owner_identity, dataset_id, dataset_version),
    CONSTRAINT uq_evaluation_datasets_owner_record UNIQUE (owner_identity, id),
    CONSTRAINT chk_evaluation_datasets_owner CHECK (btrim(owner_identity) <> ''),
    CONSTRAINT chk_evaluation_datasets_id CHECK (btrim(dataset_id) <> ''),
    CONSTRAINT chk_evaluation_datasets_version CHECK (dataset_version > 0),
    CONSTRAINT chk_evaluation_datasets_schema CHECK (schema_version > 0),
    CONSTRAINT chk_evaluation_datasets_name CHECK (btrim(name) <> ''),
    CONSTRAINT chk_evaluation_datasets_created CHECK (btrim(created_at_value) <> ''),
    CONSTRAINT chk_evaluation_datasets_digest CHECK (digest ~ '^[0-9a-f]{64}$')
);

CREATE INDEX IF NOT EXISTS idx_evaluation_datasets_owner_recorded
    ON public.evaluation_datasets (owner_identity, recorded_at DESC);
CREATE INDEX IF NOT EXISTS idx_evaluation_datasets_digest
    ON public.evaluation_datasets (digest);

CREATE TABLE IF NOT EXISTS public.evaluation_dataset_cases (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    owner_identity character varying(255) NOT NULL,
    dataset_record_id uuid NOT NULL,
    ordinal integer NOT NULL,
    case_id character varying(128) NOT NULL,
    case_version integer NOT NULL,
    input_json text NOT NULL,
    expected_json text NOT NULL,
    criteria_json text NOT NULL,
    CONSTRAINT evaluation_dataset_cases_pkey PRIMARY KEY (id),
    CONSTRAINT uq_evaluation_dataset_cases_owner_case
        UNIQUE (owner_identity, dataset_record_id, case_id, case_version),
    CONSTRAINT uq_evaluation_dataset_cases_owner_ordinal
        UNIQUE (owner_identity, dataset_record_id, ordinal),
    CONSTRAINT fk_evaluation_dataset_cases_owner_dataset
        FOREIGN KEY (owner_identity, dataset_record_id)
        REFERENCES public.evaluation_datasets (owner_identity, id)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT chk_evaluation_dataset_cases_owner CHECK (btrim(owner_identity) <> ''),
    CONSTRAINT chk_evaluation_dataset_cases_ordinal CHECK (ordinal >= 0),
    CONSTRAINT chk_evaluation_dataset_cases_id CHECK (btrim(case_id) <> ''),
    CONSTRAINT chk_evaluation_dataset_cases_version CHECK (case_version > 0),
    CONSTRAINT chk_evaluation_dataset_cases_input CHECK (
        jsonb_typeof(input_json::jsonb) IS NOT NULL
    ),
    CONSTRAINT chk_evaluation_dataset_cases_expected CHECK (
        jsonb_typeof(expected_json::jsonb) IS NOT NULL
    ),
    CONSTRAINT chk_evaluation_dataset_cases_criteria CHECK (
        jsonb_typeof(criteria_json::jsonb) = 'array'
    )
);

CREATE INDEX IF NOT EXISTS idx_evaluation_dataset_cases_dataset
    ON public.evaluation_dataset_cases (owner_identity, dataset_record_id, ordinal);

CREATE TABLE IF NOT EXISTS public.evaluation_runs (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    owner_identity character varying(255) NOT NULL,
    run_id character varying(128) NOT NULL,
    schema_version integer NOT NULL,
    dataset_record_id uuid NOT NULL,
    dataset_id character varying(128) NOT NULL,
    dataset_version integer NOT NULL,
    dataset_digest character(64) NOT NULL,
    evaluator_id character varying(128) NOT NULL,
    evaluator_version character varying(128) NOT NULL,
    subject_id character varying(128) NOT NULL,
    subject_version character varying(128) NOT NULL,
    subject_artifact_digest character(64) NOT NULL,
    mode character varying(32) NOT NULL,
    canary_percent double precision NOT NULL,
    baseline_run_record_id uuid,
    baseline_run_id character varying(128) DEFAULT ''::character varying NOT NULL,
    started_at_value character varying(64) NOT NULL,
    completed_at_value character varying(64) NOT NULL,
    completed_at_index timestamp with time zone NOT NULL,
    status character varying(32) NOT NULL,
    failure_code character varying(160) DEFAULT ''::character varying NOT NULL,
    overall_score double precision NOT NULL,
    case_pass_rate double precision NOT NULL,
    required_failure_count integer NOT NULL,
    criterion_error_count integer NOT NULL,
    reproducibility_json text NOT NULL,
    reproducibility_digest character(64) NOT NULL,
    record_digest character(64) NOT NULL,
    recorded_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    CONSTRAINT evaluation_runs_pkey PRIMARY KEY (id),
    CONSTRAINT uq_evaluation_runs_owner_key UNIQUE (owner_identity, run_id),
    CONSTRAINT uq_evaluation_runs_owner_record UNIQUE (owner_identity, id),
    CONSTRAINT fk_evaluation_runs_owner_dataset
        FOREIGN KEY (owner_identity, dataset_record_id)
        REFERENCES public.evaluation_datasets (owner_identity, id)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT fk_evaluation_runs_owner_baseline
        FOREIGN KEY (owner_identity, baseline_run_record_id)
        REFERENCES public.evaluation_runs (owner_identity, id)
        ON UPDATE RESTRICT ON DELETE RESTRICT
        DEFERRABLE INITIALLY IMMEDIATE,
    CONSTRAINT chk_evaluation_runs_owner CHECK (btrim(owner_identity) <> ''),
    CONSTRAINT chk_evaluation_runs_id CHECK (btrim(run_id) <> ''),
    CONSTRAINT chk_evaluation_runs_schema CHECK (schema_version > 0),
    CONSTRAINT chk_evaluation_runs_dataset_id CHECK (btrim(dataset_id) <> ''),
    CONSTRAINT chk_evaluation_runs_dataset_version CHECK (dataset_version > 0),
    CONSTRAINT chk_evaluation_runs_dataset_digest CHECK (
        dataset_digest ~ '^[0-9a-f]{64}$'
    ),
    CONSTRAINT chk_evaluation_runs_evaluator CHECK (
        btrim(evaluator_id) <> '' AND btrim(evaluator_version) <> ''
    ),
    CONSTRAINT chk_evaluation_runs_subject CHECK (
        btrim(subject_id) <> '' AND btrim(subject_version) <> ''
    ),
    CONSTRAINT chk_evaluation_runs_subject_digest CHECK (
        subject_artifact_digest ~ '^[0-9a-f]{64}$'
    ),
    CONSTRAINT chk_evaluation_runs_mode CHECK (mode IN ('shadow', 'canary')),
    CONSTRAINT chk_evaluation_runs_canary CHECK (
        canary_percent <> 'NaN'::double precision AND
        (
            (mode = 'shadow' AND canary_percent = 0) OR
            (mode = 'canary' AND canary_percent > 0 AND canary_percent <= 100)
        )
    ),
    CONSTRAINT chk_evaluation_runs_baseline CHECK (
        (baseline_run_record_id IS NULL AND btrim(baseline_run_id) = '') OR
        (baseline_run_record_id IS NOT NULL AND btrim(baseline_run_id) <> '')
    ),
    CONSTRAINT chk_evaluation_runs_times CHECK (
        btrim(started_at_value) <> '' AND btrim(completed_at_value) <> ''
    ),
    CONSTRAINT chk_evaluation_runs_status CHECK (status IN ('completed', 'failed')),
    CONSTRAINT chk_evaluation_runs_status_payload CHECK (
        (
            status = 'completed' AND
            btrim(failure_code) = ''
        ) OR (
            status = 'failed' AND
            btrim(failure_code) <> '' AND
            overall_score = 0 AND
            case_pass_rate = 0 AND
            required_failure_count = 0 AND
            criterion_error_count = 0
        )
    ),
    CONSTRAINT chk_evaluation_runs_scores CHECK (
        overall_score <> 'NaN'::double precision AND
        case_pass_rate <> 'NaN'::double precision AND
        overall_score BETWEEN 0 AND 1 AND
        case_pass_rate BETWEEN 0 AND 1
    ),
    CONSTRAINT chk_evaluation_runs_counts CHECK (
        required_failure_count >= 0 AND criterion_error_count >= 0
    ),
    CONSTRAINT chk_evaluation_runs_reproducibility CHECK (
        jsonb_typeof(reproducibility_json::jsonb) = 'object'
    ),
    CONSTRAINT chk_evaluation_runs_reproducibility_digest CHECK (
        reproducibility_digest ~ '^[0-9a-f]{64}$'
    ),
    CONSTRAINT chk_evaluation_runs_record_digest CHECK (
        record_digest ~ '^[0-9a-f]{64}$'
    )
);

CREATE INDEX IF NOT EXISTS idx_evaluation_runs_owner_completed
    ON public.evaluation_runs (owner_identity, completed_at_index DESC);
CREATE INDEX IF NOT EXISTS idx_evaluation_runs_dataset
    ON public.evaluation_runs (owner_identity, dataset_id, dataset_version);
CREATE INDEX IF NOT EXISTS idx_evaluation_runs_subject
    ON public.evaluation_runs (owner_identity, subject_id);
CREATE INDEX IF NOT EXISTS idx_evaluation_runs_mode
    ON public.evaluation_runs (owner_identity, mode);
CREATE INDEX IF NOT EXISTS idx_evaluation_runs_baseline
    ON public.evaluation_runs (owner_identity, baseline_run_id);
CREATE INDEX IF NOT EXISTS idx_evaluation_runs_record_digest
    ON public.evaluation_runs (record_digest);

CREATE TABLE IF NOT EXISTS public.evaluation_run_case_results (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    owner_identity character varying(255) NOT NULL,
    run_record_id uuid NOT NULL,
    ordinal integer NOT NULL,
    case_id character varying(128) NOT NULL,
    case_version integer NOT NULL,
    passed boolean NOT NULL,
    score double precision NOT NULL,
    criteria_json text NOT NULL,
    CONSTRAINT evaluation_run_case_results_pkey PRIMARY KEY (id),
    CONSTRAINT uq_evaluation_run_results_owner_case
        UNIQUE (owner_identity, run_record_id, case_id, case_version),
    CONSTRAINT uq_evaluation_run_results_owner_ordinal
        UNIQUE (owner_identity, run_record_id, ordinal),
    CONSTRAINT fk_evaluation_run_results_owner_run
        FOREIGN KEY (owner_identity, run_record_id)
        REFERENCES public.evaluation_runs (owner_identity, id)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT chk_evaluation_run_results_owner CHECK (btrim(owner_identity) <> ''),
    CONSTRAINT chk_evaluation_run_results_ordinal CHECK (ordinal >= 0),
    CONSTRAINT chk_evaluation_run_results_case CHECK (
        btrim(case_id) <> '' AND case_version > 0
    ),
    CONSTRAINT chk_evaluation_run_results_score CHECK (
        score <> 'NaN'::double precision AND score BETWEEN 0 AND 1
    ),
    CONSTRAINT chk_evaluation_run_results_criteria CHECK (
        jsonb_typeof(criteria_json::jsonb) = 'array'
    )
);

CREATE INDEX IF NOT EXISTS idx_evaluation_run_results_run
    ON public.evaluation_run_case_results (owner_identity, run_record_id, ordinal);

CREATE TABLE IF NOT EXISTS public.evaluation_comparison_receipts (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    owner_identity character varying(255) NOT NULL,
    receipt_id character varying(128) NOT NULL,
    schema_version integer NOT NULL,
    candidate_run_id character varying(128) NOT NULL,
    candidate_record_digest character(64) NOT NULL,
    baseline_run_id character varying(128) NOT NULL,
    baseline_record_digest character(64) NOT NULL,
    thresholds_json text NOT NULL,
    comparison_json text NOT NULL,
    created_at_value character varying(64) NOT NULL,
    receipt_digest character(64) NOT NULL,
    recorded_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    CONSTRAINT evaluation_comparison_receipts_pkey PRIMARY KEY (id),
    CONSTRAINT uq_evaluation_comparison_receipts_owner_key
        UNIQUE (owner_identity, receipt_id),
    CONSTRAINT uq_evaluation_comparison_receipts_owner_record
        UNIQUE (owner_identity, id),
    CONSTRAINT fk_evaluation_comparison_candidate
        FOREIGN KEY (owner_identity, candidate_run_id)
        REFERENCES public.evaluation_runs (owner_identity, run_id)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT fk_evaluation_comparison_baseline
        FOREIGN KEY (owner_identity, baseline_run_id)
        REFERENCES public.evaluation_runs (owner_identity, run_id)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT chk_evaluation_comparison_owner CHECK (btrim(owner_identity) <> ''),
    CONSTRAINT chk_evaluation_comparison_id CHECK (btrim(receipt_id) <> ''),
    CONSTRAINT chk_evaluation_comparison_schema CHECK (schema_version > 0),
    CONSTRAINT chk_evaluation_comparison_runs CHECK (
        btrim(candidate_run_id) <> '' AND
        btrim(baseline_run_id) <> '' AND
        candidate_run_id <> baseline_run_id
    ),
    CONSTRAINT chk_evaluation_comparison_digests CHECK (
        candidate_record_digest ~ '^[0-9a-f]{64}$' AND
        baseline_record_digest ~ '^[0-9a-f]{64}$' AND
        receipt_digest ~ '^[0-9a-f]{64}$'
    ),
    CONSTRAINT chk_evaluation_comparison_thresholds CHECK (
        jsonb_typeof(thresholds_json::jsonb) = 'object'
    ),
    CONSTRAINT chk_evaluation_comparison_payload CHECK (
        jsonb_typeof(comparison_json::jsonb) = 'object'
    ),
    CONSTRAINT chk_evaluation_comparison_created CHECK (btrim(created_at_value) <> '')
);

CREATE INDEX IF NOT EXISTS idx_evaluation_comparison_owner_recorded
    ON public.evaluation_comparison_receipts (owner_identity, recorded_at DESC);
CREATE INDEX IF NOT EXISTS idx_evaluation_comparison_candidate
    ON public.evaluation_comparison_receipts (owner_identity, candidate_run_id);

CREATE TABLE IF NOT EXISTS public.evaluation_promotion_decision_receipts (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    owner_identity character varying(255) NOT NULL,
    receipt_id character varying(128) NOT NULL,
    schema_version integer NOT NULL,
    candidate_run_id character varying(128) NOT NULL,
    candidate_record_digest character(64) NOT NULL,
    baseline_run_id character varying(128),
    baseline_record_digest character(64),
    comparison_receipt_id uuid,
    comparison_receipt_key character varying(128) DEFAULT ''::character varying NOT NULL,
    thresholds_json text NOT NULL,
    decision_json text NOT NULL,
    created_at_value character varying(64) NOT NULL,
    receipt_digest character(64) NOT NULL,
    recorded_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    CONSTRAINT evaluation_promotion_decision_receipts_pkey PRIMARY KEY (id),
    CONSTRAINT uq_evaluation_promotion_receipts_owner_key
        UNIQUE (owner_identity, receipt_id),
    CONSTRAINT uq_evaluation_promotion_receipts_owner_record
        UNIQUE (owner_identity, id),
    CONSTRAINT fk_evaluation_promotion_candidate
        FOREIGN KEY (owner_identity, candidate_run_id)
        REFERENCES public.evaluation_runs (owner_identity, run_id)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT fk_evaluation_promotion_baseline
        FOREIGN KEY (owner_identity, baseline_run_id)
        REFERENCES public.evaluation_runs (owner_identity, run_id)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT fk_evaluation_promotion_comparison
        FOREIGN KEY (owner_identity, comparison_receipt_id)
        REFERENCES public.evaluation_comparison_receipts (owner_identity, id)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT chk_evaluation_promotion_owner CHECK (btrim(owner_identity) <> ''),
    CONSTRAINT chk_evaluation_promotion_id CHECK (btrim(receipt_id) <> ''),
    CONSTRAINT chk_evaluation_promotion_schema CHECK (schema_version > 0),
    CONSTRAINT chk_evaluation_promotion_candidate CHECK (
        btrim(candidate_run_id) <> '' AND
        candidate_record_digest ~ '^[0-9a-f]{64}$'
    ),
    CONSTRAINT chk_evaluation_promotion_baseline CHECK (
        (
            baseline_run_id IS NULL AND
            baseline_record_digest IS NULL
        ) OR (
            baseline_run_id IS NOT NULL AND
            baseline_record_digest IS NOT NULL AND
            btrim(baseline_run_id) <> '' AND
            baseline_run_id <> candidate_run_id AND
            baseline_record_digest ~ '^[0-9a-f]{64}$'
        )
    ),
    CONSTRAINT chk_evaluation_promotion_comparison CHECK (
        (
            comparison_receipt_id IS NULL AND
            btrim(comparison_receipt_key) = ''
        ) OR (
            comparison_receipt_id IS NOT NULL AND
            btrim(comparison_receipt_key) <> ''
        )
    ),
    CONSTRAINT chk_evaluation_promotion_thresholds CHECK (
        jsonb_typeof(thresholds_json::jsonb) = 'object'
    ),
    CONSTRAINT chk_evaluation_promotion_decision CHECK (
        jsonb_typeof(decision_json::jsonb) = 'object'
    ),
    CONSTRAINT chk_evaluation_promotion_created CHECK (btrim(created_at_value) <> ''),
    CONSTRAINT chk_evaluation_promotion_digest CHECK (
        receipt_digest ~ '^[0-9a-f]{64}$'
    )
);

CREATE INDEX IF NOT EXISTS idx_evaluation_promotion_owner_recorded
    ON public.evaluation_promotion_decision_receipts (owner_identity, recorded_at DESC);
CREATE INDEX IF NOT EXISTS idx_evaluation_promotion_candidate
    ON public.evaluation_promotion_decision_receipts (owner_identity, candidate_run_id);

CREATE OR REPLACE FUNCTION public.hai_reject_evaluation_mutation()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE EXCEPTION 'evaluation evidence is immutable'
        USING ERRCODE = '55000';
END;
$$;

CREATE OR REPLACE FUNCTION public.hai_reject_evaluation_truncate()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE EXCEPTION 'evaluation evidence cannot be truncated'
        USING ERRCODE = '55000';
END;
$$;

DROP TRIGGER IF EXISTS trg_evaluation_datasets_immutable ON public.evaluation_datasets;
CREATE TRIGGER trg_evaluation_datasets_immutable
    BEFORE UPDATE OR DELETE ON public.evaluation_datasets
    FOR EACH ROW EXECUTE FUNCTION public.hai_reject_evaluation_mutation();
DROP TRIGGER IF EXISTS trg_evaluation_datasets_no_truncate ON public.evaluation_datasets;
CREATE TRIGGER trg_evaluation_datasets_no_truncate
    BEFORE TRUNCATE ON public.evaluation_datasets
    FOR EACH STATEMENT EXECUTE FUNCTION public.hai_reject_evaluation_truncate();

DROP TRIGGER IF EXISTS trg_evaluation_dataset_cases_immutable ON public.evaluation_dataset_cases;
CREATE TRIGGER trg_evaluation_dataset_cases_immutable
    BEFORE UPDATE OR DELETE ON public.evaluation_dataset_cases
    FOR EACH ROW EXECUTE FUNCTION public.hai_reject_evaluation_mutation();
DROP TRIGGER IF EXISTS trg_evaluation_dataset_cases_no_truncate ON public.evaluation_dataset_cases;
CREATE TRIGGER trg_evaluation_dataset_cases_no_truncate
    BEFORE TRUNCATE ON public.evaluation_dataset_cases
    FOR EACH STATEMENT EXECUTE FUNCTION public.hai_reject_evaluation_truncate();

DROP TRIGGER IF EXISTS trg_evaluation_runs_immutable ON public.evaluation_runs;
CREATE TRIGGER trg_evaluation_runs_immutable
    BEFORE UPDATE OR DELETE ON public.evaluation_runs
    FOR EACH ROW EXECUTE FUNCTION public.hai_reject_evaluation_mutation();
DROP TRIGGER IF EXISTS trg_evaluation_runs_no_truncate ON public.evaluation_runs;
CREATE TRIGGER trg_evaluation_runs_no_truncate
    BEFORE TRUNCATE ON public.evaluation_runs
    FOR EACH STATEMENT EXECUTE FUNCTION public.hai_reject_evaluation_truncate();

DROP TRIGGER IF EXISTS trg_evaluation_run_results_immutable ON public.evaluation_run_case_results;
CREATE TRIGGER trg_evaluation_run_results_immutable
    BEFORE UPDATE OR DELETE ON public.evaluation_run_case_results
    FOR EACH ROW EXECUTE FUNCTION public.hai_reject_evaluation_mutation();
DROP TRIGGER IF EXISTS trg_evaluation_run_results_no_truncate ON public.evaluation_run_case_results;
CREATE TRIGGER trg_evaluation_run_results_no_truncate
    BEFORE TRUNCATE ON public.evaluation_run_case_results
    FOR EACH STATEMENT EXECUTE FUNCTION public.hai_reject_evaluation_truncate();

DROP TRIGGER IF EXISTS trg_evaluation_comparison_receipts_immutable
    ON public.evaluation_comparison_receipts;
CREATE TRIGGER trg_evaluation_comparison_receipts_immutable
    BEFORE UPDATE OR DELETE ON public.evaluation_comparison_receipts
    FOR EACH ROW EXECUTE FUNCTION public.hai_reject_evaluation_mutation();
DROP TRIGGER IF EXISTS trg_evaluation_comparison_receipts_no_truncate
    ON public.evaluation_comparison_receipts;
CREATE TRIGGER trg_evaluation_comparison_receipts_no_truncate
    BEFORE TRUNCATE ON public.evaluation_comparison_receipts
    FOR EACH STATEMENT EXECUTE FUNCTION public.hai_reject_evaluation_truncate();

DROP TRIGGER IF EXISTS trg_evaluation_promotion_receipts_immutable
    ON public.evaluation_promotion_decision_receipts;
CREATE TRIGGER trg_evaluation_promotion_receipts_immutable
    BEFORE UPDATE OR DELETE ON public.evaluation_promotion_decision_receipts
    FOR EACH ROW EXECUTE FUNCTION public.hai_reject_evaluation_mutation();
DROP TRIGGER IF EXISTS trg_evaluation_promotion_receipts_no_truncate
    ON public.evaluation_promotion_decision_receipts;
CREATE TRIGGER trg_evaluation_promotion_receipts_no_truncate
    BEFORE TRUNCATE ON public.evaluation_promotion_decision_receipts
    FOR EACH STATEMENT EXECUTE FUNCTION public.hai_reject_evaluation_truncate();
