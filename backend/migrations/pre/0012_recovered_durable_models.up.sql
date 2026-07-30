CREATE TABLE IF NOT EXISTS public.brain_catalog_upstream_reviews (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    catalog_entry_id character varying(160) NOT NULL,
    name character varying(255) NOT NULL,
    upstream_url character varying(1024) NOT NULL,
    resolved_repository character varying(255) DEFAULT ''::character varying NOT NULL,
    resolved_upstream_url character varying(1024) DEFAULT ''::character varying NOT NULL,
    repository_moved boolean DEFAULT false NOT NULL,
    available boolean DEFAULT false NOT NULL,
    archived boolean DEFAULT false NOT NULL,
    license character varying(120) DEFAULT ''::character varying NOT NULL,
    default_branch character varying(255) DEFAULT ''::character varying NOT NULL,
    pushed_at character varying(80) DEFAULT ''::character varying NOT NULL,
    message text DEFAULT ''::text NOT NULL,
    disposition character varying(80) NOT NULL,
    readiness character varying(80) DEFAULT ''::character varying NOT NULL,
    readiness_reason text DEFAULT ''::text NOT NULL,
    required_gates_json text DEFAULT '[]'::text NOT NULL,
    checked_at timestamp with time zone NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    CONSTRAINT brain_catalog_upstream_reviews_pkey PRIMARY KEY (id),
    CONSTRAINT chk_brain_catalog_upstream_entry CHECK (
        btrim(catalog_entry_id) <> ''
    ),
    CONSTRAINT chk_brain_catalog_upstream_name CHECK (btrim(name) <> ''),
    CONSTRAINT chk_brain_catalog_upstream_url CHECK (btrim(upstream_url) <> ''),
    CONSTRAINT chk_brain_catalog_upstream_disposition CHECK (
        disposition::text = ANY (ARRAY[
            'candidate', 'integrated_profile', 'compatibility_only',
            'reference_only', 'excluded', 'license_review'
        ]::text[])
    ),
    CONSTRAINT chk_brain_catalog_upstream_readiness CHECK (
        readiness::text = ANY (ARRAY[
            'review_now', 'license_review', 'reference_only', 'not_adopted',
            'archived', 'upstream_unavailable', 'profile_review'
        ]::text[])
    ),
    CONSTRAINT chk_brain_catalog_upstream_gates CHECK (
        jsonb_typeof(required_gates_json::jsonb) = 'array'
    )
);

CREATE INDEX IF NOT EXISTS idx_brain_catalog_upstream_reviews_entry_checked
    ON public.brain_catalog_upstream_reviews (catalog_entry_id, checked_at DESC);
CREATE INDEX IF NOT EXISTS idx_brain_catalog_upstream_reviews_checked
    ON public.brain_catalog_upstream_reviews (checked_at DESC);
CREATE INDEX IF NOT EXISTS idx_brain_catalog_upstream_reviews_disposition
    ON public.brain_catalog_upstream_reviews (disposition);
CREATE INDEX IF NOT EXISTS idx_brain_catalog_upstream_reviews_readiness
    ON public.brain_catalog_upstream_reviews (readiness);
CREATE INDEX IF NOT EXISTS idx_brain_catalog_upstream_reviews_available
    ON public.brain_catalog_upstream_reviews (available);
CREATE INDEX IF NOT EXISTS idx_brain_catalog_upstream_reviews_archived
    ON public.brain_catalog_upstream_reviews (archived);
CREATE INDEX IF NOT EXISTS idx_brain_catalog_upstream_reviews_moved
    ON public.brain_catalog_upstream_reviews (repository_moved);

CREATE TABLE IF NOT EXISTS public.brain_catalog_collection_reviews (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    source_url character varying(1024) NOT NULL,
    available boolean DEFAULT false NOT NULL,
    expected_total bigint DEFAULT 0 NOT NULL,
    current_total bigint DEFAULT 0 NOT NULL,
    new_collections_json text DEFAULT '[]'::text NOT NULL,
    missing_expected_json text DEFAULT '[]'::text NOT NULL,
    message text DEFAULT ''::text NOT NULL,
    checked_at timestamp with time zone NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    CONSTRAINT brain_catalog_collection_reviews_pkey PRIMARY KEY (id),
    CONSTRAINT chk_brain_catalog_collection_source CHECK (
        btrim(source_url) <> ''
    ),
    CONSTRAINT chk_brain_catalog_collection_counts CHECK (
        expected_total >= 0 AND current_total >= 0
    ),
    CONSTRAINT chk_brain_catalog_collection_new CHECK (
        jsonb_typeof(new_collections_json::jsonb) IN ('array', 'null')
    ),
    CONSTRAINT chk_brain_catalog_collection_missing CHECK (
        jsonb_typeof(missing_expected_json::jsonb) IN ('array', 'null')
    )
);

CREATE INDEX IF NOT EXISTS idx_brain_catalog_collection_reviews_checked
    ON public.brain_catalog_collection_reviews (checked_at DESC);
CREATE INDEX IF NOT EXISTS idx_brain_catalog_collection_reviews_available
    ON public.brain_catalog_collection_reviews (available);

CREATE TABLE IF NOT EXISTS public.brain_catalog_repository_discovery_reviews (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    source_url character varying(1024) NOT NULL,
    scope character varying(80) NOT NULL,
    available boolean DEFAULT false NOT NULL,
    collections_screened bigint DEFAULT 0 NOT NULL,
    eligible_collections bigint DEFAULT 0 NOT NULL,
    collections_checked bigint DEFAULT 0 NOT NULL,
    repositories_checked bigint DEFAULT 0 NOT NULL,
    known_profile_hits bigint DEFAULT 0 NOT NULL,
    unreviewed_discoveries bigint DEFAULT 0 NOT NULL,
    missing_collections_json text DEFAULT '[]'::text NOT NULL,
    unavailable_collections_json text DEFAULT '[]'::text NOT NULL,
    candidate_repositories_json text DEFAULT '[]'::text NOT NULL,
    candidates_truncated boolean DEFAULT false NOT NULL,
    message text DEFAULT ''::text NOT NULL,
    checked_at timestamp with time zone NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    CONSTRAINT brain_catalog_repository_discovery_reviews_pkey PRIMARY KEY (id),
    CONSTRAINT chk_brain_catalog_repository_source CHECK (
        btrim(source_url) <> ''
    ),
    CONSTRAINT chk_brain_catalog_repository_scope CHECK (
        scope IN ('candidate', 'reviewable')
    ),
    CONSTRAINT chk_brain_catalog_repository_counts CHECK (
        collections_screened >= 0
        AND eligible_collections >= 0
        AND collections_checked >= 0
        AND repositories_checked >= 0
        AND known_profile_hits >= 0
        AND unreviewed_discoveries >= 0
    ),
    CONSTRAINT chk_brain_catalog_repository_missing CHECK (
        jsonb_typeof(missing_collections_json::jsonb) IN ('array', 'null')
    ),
    CONSTRAINT chk_brain_catalog_repository_unavailable CHECK (
        jsonb_typeof(unavailable_collections_json::jsonb) IN ('array', 'null')
    ),
    CONSTRAINT chk_brain_catalog_repository_candidates CHECK (
        jsonb_typeof(candidate_repositories_json::jsonb) IN ('array', 'null')
        AND (
            jsonb_typeof(candidate_repositories_json::jsonb) = 'null'
            OR jsonb_array_length(candidate_repositories_json::jsonb) <= 30
        )
    )
);

CREATE INDEX IF NOT EXISTS idx_brain_catalog_repository_reviews_scope_checked
    ON public.brain_catalog_repository_discovery_reviews (scope, checked_at DESC);
CREATE INDEX IF NOT EXISTS idx_brain_catalog_repository_reviews_checked
    ON public.brain_catalog_repository_discovery_reviews (checked_at DESC);
CREATE INDEX IF NOT EXISTS idx_brain_catalog_repository_reviews_available
    ON public.brain_catalog_repository_discovery_reviews (available);

CREATE TABLE IF NOT EXISTS public.llm_model_maintenances (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    provider_id character varying(120) NOT NULL,
    provider_name character varying(255) NOT NULL,
    model_id character varying(255) NOT NULL,
    model_name character varying(255) NOT NULL,
    status character varying(80) NOT NULL,
    reason text DEFAULT ''::text NOT NULL,
    previous_digest character varying(255) DEFAULT ''::character varying NOT NULL,
    current_digest character varying(255) DEFAULT ''::character varying NOT NULL,
    configuration_fingerprint character(64) NOT NULL,
    configuration_changed boolean DEFAULT false NOT NULL,
    update_attempted boolean DEFAULT false NOT NULL,
    update_applied boolean DEFAULT false NOT NULL,
    blocks_execution boolean DEFAULT false NOT NULL,
    checked_at timestamp with time zone NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    CONSTRAINT llm_model_maintenances_pkey PRIMARY KEY (id),
    CONSTRAINT chk_llm_model_maintenance_provider CHECK (
        btrim(provider_id) <> '' AND btrim(provider_name) <> ''
    ),
    CONSTRAINT chk_llm_model_maintenance_model CHECK (
        btrim(model_id) <> '' AND btrim(model_name) <> ''
    ),
    CONSTRAINT chk_llm_model_maintenance_status CHECK (
        status IN (
            'not_enforced', 'failed', 'current', 'provider_managed',
            'installed', 'updated'
        )
    ),
    CONSTRAINT chk_llm_model_maintenance_fingerprint CHECK (
        configuration_fingerprint ~ '^[0-9a-f]{64}$'
    ),
    CONSTRAINT chk_llm_model_maintenance_update CHECK (
        NOT update_applied OR (
            update_attempted AND status IN ('installed', 'updated')
        )
    ),
    CONSTRAINT chk_llm_model_maintenance_block CHECK (
        NOT blocks_execution OR status = 'failed'
    )
);

CREATE INDEX IF NOT EXISTS idx_llm_model_maintenances_model_checked
    ON public.llm_model_maintenances (
        provider_id, model_id, checked_at DESC
    );
CREATE INDEX IF NOT EXISTS idx_llm_model_maintenances_checked
    ON public.llm_model_maintenances (checked_at DESC);
CREATE INDEX IF NOT EXISTS idx_llm_model_maintenances_status
    ON public.llm_model_maintenances (status);
CREATE INDEX IF NOT EXISTS idx_llm_model_maintenances_fingerprint
    ON public.llm_model_maintenances (configuration_fingerprint);
CREATE INDEX IF NOT EXISTS idx_llm_model_maintenances_configuration_changed
    ON public.llm_model_maintenances (configuration_changed);
CREATE INDEX IF NOT EXISTS idx_llm_model_maintenances_blocks_execution
    ON public.llm_model_maintenances (blocks_execution);

CREATE TABLE IF NOT EXISTS public.llm_generation_records (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    provider_id character varying(120) DEFAULT ''::character varying NOT NULL,
    model_id character varying(255) DEFAULT ''::character varying NOT NULL,
    model_name character varying(255) DEFAULT ''::character varying NOT NULL,
    tier character varying(80) DEFAULT ''::character varying NOT NULL,
    status character varying(80) NOT NULL,
    reason text DEFAULT ''::text NOT NULL,
    estimated_cost_eur double precision DEFAULT 0 NOT NULL,
    input_tokens bigint DEFAULT 0 NOT NULL,
    output_tokens bigint DEFAULT 0 NOT NULL,
    usage_source character varying(80) DEFAULT ''::character varying NOT NULL,
    duration_ms bigint DEFAULT 0 NOT NULL,
    fallback_path_json text DEFAULT '[]'::text NOT NULL,
    logged_at timestamp with time zone NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    CONSTRAINT llm_generation_records_pkey PRIMARY KEY (id),
    CONSTRAINT chk_llm_generation_status CHECK (btrim(status) <> ''),
    CONSTRAINT chk_llm_generation_cost CHECK (
        estimated_cost_eur <> 'NaN'::double precision
        AND estimated_cost_eur >= 0
    ),
    CONSTRAINT chk_llm_generation_usage CHECK (
        input_tokens >= 0 AND output_tokens >= 0 AND duration_ms >= 0
    ),
    CONSTRAINT chk_llm_generation_fallback CHECK (
        jsonb_typeof(fallback_path_json::jsonb) IN ('array', 'null')
    )
);

CREATE INDEX IF NOT EXISTS idx_llm_generation_records_logged
    ON public.llm_generation_records (logged_at DESC);
CREATE INDEX IF NOT EXISTS idx_llm_generation_records_provider_logged
    ON public.llm_generation_records (provider_id, logged_at DESC);
CREATE INDEX IF NOT EXISTS idx_llm_generation_records_model_logged
    ON public.llm_generation_records (model_id, logged_at DESC);
CREATE INDEX IF NOT EXISTS idx_llm_generation_records_tier
    ON public.llm_generation_records (tier);
CREATE INDEX IF NOT EXISTS idx_llm_generation_records_status
    ON public.llm_generation_records (status);

CREATE TABLE IF NOT EXISTS public.mini_swe_patch_proposals (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    owner_identity character varying(255) NOT NULL,
    workflow_id uuid NOT NULL,
    workspace_id character varying(64) NOT NULL,
    status character varying(40) NOT NULL,
    summary character varying(512) NOT NULL,
    diff_digest character varying(64) DEFAULT ''::character varying NOT NULL,
    changed_files bigint DEFAULT 0 NOT NULL,
    diff_truncated boolean DEFAULT false NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    completed_at timestamp with time zone,
    CONSTRAINT mini_swe_patch_proposals_pkey PRIMARY KEY (id),
    CONSTRAINT chk_mini_swe_patch_owner CHECK (btrim(owner_identity) <> ''),
    CONSTRAINT chk_mini_swe_patch_workspace CHECK (
        workspace_id ~ '^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$'
    ),
    CONSTRAINT chk_mini_swe_patch_status CHECK (
        status IN ('running', 'completed', 'failed')
    ),
    CONSTRAINT chk_mini_swe_patch_summary CHECK (btrim(summary) <> ''),
    CONSTRAINT chk_mini_swe_patch_digest CHECK (
        diff_digest = '' OR diff_digest ~ '^[0-9a-f]{64}$'
    ),
    CONSTRAINT chk_mini_swe_patch_changed_files CHECK (
        changed_files BETWEEN 0 AND 2000
    ),
    CONSTRAINT chk_mini_swe_patch_lifecycle CHECK (
        (
            status = 'running'
            AND completed_at IS NULL
            AND diff_digest = ''
            AND changed_files = 0
            AND NOT diff_truncated
        ) OR (
            status = 'failed'
            AND completed_at IS NOT NULL
            AND completed_at >= created_at
            AND diff_digest = ''
            AND changed_files = 0
            AND NOT diff_truncated
        ) OR (
            status = 'completed'
            AND completed_at IS NOT NULL
            AND completed_at >= created_at
            AND diff_digest ~ '^[0-9a-f]{64}$'
            AND NOT diff_truncated
        )
    )
);

CREATE INDEX IF NOT EXISTS idx_mini_swe_patch_proposals_owner_created
    ON public.mini_swe_patch_proposals (owner_identity, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_mini_swe_patch_proposals_workflow
    ON public.mini_swe_patch_proposals (workflow_id);
CREATE INDEX IF NOT EXISTS idx_mini_swe_patch_proposals_workspace
    ON public.mini_swe_patch_proposals (workspace_id);
CREATE INDEX IF NOT EXISTS idx_mini_swe_patch_proposals_status
    ON public.mini_swe_patch_proposals (status);
CREATE INDEX IF NOT EXISTS idx_mini_swe_patch_proposals_completed
    ON public.mini_swe_patch_proposals (completed_at DESC);

CREATE OR REPLACE FUNCTION public.hai_reject_recovered_audit_mutation()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE EXCEPTION 'recovered catalog and LLM audit evidence is immutable'
        USING ERRCODE = '55000';
END;
$$;

DROP TRIGGER IF EXISTS trg_brain_catalog_upstream_reviews_immutable
    ON public.brain_catalog_upstream_reviews;
CREATE TRIGGER trg_brain_catalog_upstream_reviews_immutable
    BEFORE UPDATE OR DELETE ON public.brain_catalog_upstream_reviews
    FOR EACH ROW EXECUTE FUNCTION public.hai_reject_recovered_audit_mutation();
DROP TRIGGER IF EXISTS trg_brain_catalog_upstream_reviews_no_truncate
    ON public.brain_catalog_upstream_reviews;
CREATE TRIGGER trg_brain_catalog_upstream_reviews_no_truncate
    BEFORE TRUNCATE ON public.brain_catalog_upstream_reviews
    FOR EACH STATEMENT EXECUTE FUNCTION public.hai_reject_recovered_audit_mutation();

DROP TRIGGER IF EXISTS trg_brain_catalog_collection_reviews_immutable
    ON public.brain_catalog_collection_reviews;
CREATE TRIGGER trg_brain_catalog_collection_reviews_immutable
    BEFORE UPDATE OR DELETE ON public.brain_catalog_collection_reviews
    FOR EACH ROW EXECUTE FUNCTION public.hai_reject_recovered_audit_mutation();
DROP TRIGGER IF EXISTS trg_brain_catalog_collection_reviews_no_truncate
    ON public.brain_catalog_collection_reviews;
CREATE TRIGGER trg_brain_catalog_collection_reviews_no_truncate
    BEFORE TRUNCATE ON public.brain_catalog_collection_reviews
    FOR EACH STATEMENT EXECUTE FUNCTION public.hai_reject_recovered_audit_mutation();

DROP TRIGGER IF EXISTS trg_brain_catalog_repository_reviews_immutable
    ON public.brain_catalog_repository_discovery_reviews;
CREATE TRIGGER trg_brain_catalog_repository_reviews_immutable
    BEFORE UPDATE OR DELETE ON public.brain_catalog_repository_discovery_reviews
    FOR EACH ROW EXECUTE FUNCTION public.hai_reject_recovered_audit_mutation();
DROP TRIGGER IF EXISTS trg_brain_catalog_repository_reviews_no_truncate
    ON public.brain_catalog_repository_discovery_reviews;
CREATE TRIGGER trg_brain_catalog_repository_reviews_no_truncate
    BEFORE TRUNCATE ON public.brain_catalog_repository_discovery_reviews
    FOR EACH STATEMENT EXECUTE FUNCTION public.hai_reject_recovered_audit_mutation();

DROP TRIGGER IF EXISTS trg_llm_model_maintenances_immutable
    ON public.llm_model_maintenances;
CREATE TRIGGER trg_llm_model_maintenances_immutable
    BEFORE UPDATE OR DELETE ON public.llm_model_maintenances
    FOR EACH ROW EXECUTE FUNCTION public.hai_reject_recovered_audit_mutation();
DROP TRIGGER IF EXISTS trg_llm_model_maintenances_no_truncate
    ON public.llm_model_maintenances;
CREATE TRIGGER trg_llm_model_maintenances_no_truncate
    BEFORE TRUNCATE ON public.llm_model_maintenances
    FOR EACH STATEMENT EXECUTE FUNCTION public.hai_reject_recovered_audit_mutation();

DROP TRIGGER IF EXISTS trg_llm_generation_records_immutable
    ON public.llm_generation_records;
CREATE TRIGGER trg_llm_generation_records_immutable
    BEFORE UPDATE OR DELETE ON public.llm_generation_records
    FOR EACH ROW EXECUTE FUNCTION public.hai_reject_recovered_audit_mutation();
DROP TRIGGER IF EXISTS trg_llm_generation_records_no_truncate
    ON public.llm_generation_records;
CREATE TRIGGER trg_llm_generation_records_no_truncate
    BEFORE TRUNCATE ON public.llm_generation_records
    FOR EACH STATEMENT EXECUTE FUNCTION public.hai_reject_recovered_audit_mutation();
