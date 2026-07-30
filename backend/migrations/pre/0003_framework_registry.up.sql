CREATE TABLE IF NOT EXISTS public.framework_preferences (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    owner_identity character varying(255) NOT NULL,
    framework_id character varying(160) NOT NULL,
    state character varying(32) DEFAULT 'default'::character varying NOT NULL,
    pinned boolean DEFAULT false NOT NULL,
    maximum_autonomy_level smallint,
    adaptations_json jsonb DEFAULT '[]'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    CONSTRAINT framework_preferences_pkey PRIMARY KEY (id),
    CONSTRAINT uq_framework_preferences_owner_framework UNIQUE (owner_identity, framework_id),
    CONSTRAINT chk_framework_preferences_owner CHECK (btrim(owner_identity) <> ''),
    CONSTRAINT chk_framework_preferences_framework CHECK (btrim(framework_id) <> ''),
    CONSTRAINT chk_framework_preferences_state CHECK (state IN ('default', 'enabled', 'disabled')),
    CONSTRAINT chk_framework_preferences_autonomy CHECK (
        maximum_autonomy_level IS NULL OR maximum_autonomy_level BETWEEN 0 AND 10
    ),
    CONSTRAINT chk_framework_preferences_adaptations_array CHECK (
        jsonb_typeof(adaptations_json) = 'array'
    )
);

CREATE INDEX IF NOT EXISTS idx_framework_preferences_state
    ON public.framework_preferences USING btree (state);
CREATE INDEX IF NOT EXISTS idx_framework_preferences_pinned
    ON public.framework_preferences USING btree (pinned);

CREATE TABLE IF NOT EXISTS public.framework_selection_records (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    owner_identity character varying(255) NOT NULL,
    task_plan_id character varying(160),
    request_hash character(64) NOT NULL,
    request_summary character varying(512) NOT NULL,
    catalog_version character varying(32) NOT NULL,
    catalog_digest character(64) NOT NULL,
    selector_algorithm_version character varying(64) NOT NULL,
    effective_preference_digest character(64) NOT NULL,
    constitution_digest character(64) NOT NULL,
    life_domain character varying(120) NOT NULL,
    need_or_commitment text NOT NULL,
    selected_json jsonb DEFAULT '[]'::jsonb NOT NULL,
    conflicts_json jsonb DEFAULT '[]'::jsonb NOT NULL,
    required_agents_json jsonb DEFAULT '[]'::jsonb NOT NULL,
    maximum_autonomy_level smallint DEFAULT 0 NOT NULL,
    authority_summary text NOT NULL,
    requires_approval boolean DEFAULT false NOT NULL,
    approval_reasons_json jsonb DEFAULT '[]'::jsonb NOT NULL,
    evidence_requirements_json jsonb DEFAULT '[]'::jsonb NOT NULL,
    completion_criteria_json jsonb DEFAULT '[]'::jsonb NOT NULL,
    learning_plan_json jsonb DEFAULT '[]'::jsonb NOT NULL,
    context_requirements_json jsonb DEFAULT '[]'::jsonb NOT NULL,
    selection_reason text NOT NULL,
    constitution_version integer DEFAULT 0 NOT NULL,
    constitution_source text NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    CONSTRAINT framework_selection_records_pkey PRIMARY KEY (id),
    CONSTRAINT chk_framework_selection_records_owner CHECK (btrim(owner_identity) <> ''),
    CONSTRAINT chk_framework_selection_records_request_hash CHECK (
        request_hash ~ '^[0-9a-f]{64}$'
    ),
    CONSTRAINT chk_framework_selection_records_catalog_version CHECK (
        btrim(catalog_version) <> ''
    ),
    CONSTRAINT chk_framework_selection_records_catalog_digest CHECK (
        catalog_digest ~ '^[0-9a-f]{64}$'
    ),
    CONSTRAINT chk_framework_selection_records_selector_version CHECK (
        btrim(selector_algorithm_version) <> ''
    ),
    CONSTRAINT chk_framework_selection_records_preference_digest CHECK (
        effective_preference_digest ~ '^[0-9a-f]{64}$'
    ),
    CONSTRAINT chk_framework_selection_records_constitution_digest CHECK (
        constitution_digest ~ '^[0-9a-f]{64}$'
    ),
    CONSTRAINT chk_framework_selection_records_constitution_source CHECK (
        constitution_source ~ '^[A-Za-z0-9][A-Za-z0-9._-]*:v[1-9][0-9]*$' AND
        constitution_source ~ (':v' || constitution_version::text || '$')
    ),
    CONSTRAINT chk_framework_selection_records_summary CHECK (
        char_length(request_summary) BETWEEN 1 AND 512
    ),
    CONSTRAINT chk_framework_selection_records_autonomy CHECK (
        maximum_autonomy_level BETWEEN 0 AND 10
    ),
    CONSTRAINT chk_framework_selection_records_constitution_version CHECK (
        constitution_version > 0
    ),
    CONSTRAINT chk_framework_selection_records_life_domain CHECK (
        btrim(life_domain) <> ''
    ),
    CONSTRAINT chk_framework_selection_records_need CHECK (
        btrim(need_or_commitment) <> ''
    ),
    CONSTRAINT chk_framework_selection_records_authority CHECK (
        btrim(authority_summary) <> ''
    ),
    CONSTRAINT chk_framework_selection_records_reason CHECK (
        btrim(selection_reason) <> ''
    ),
    CONSTRAINT chk_framework_selection_records_selected_array CHECK (
        jsonb_typeof(selected_json) = 'array'
    ),
    CONSTRAINT chk_framework_selection_records_conflicts_array CHECK (
        jsonb_typeof(conflicts_json) = 'array'
    ),
    CONSTRAINT chk_framework_selection_records_agents_array CHECK (
        jsonb_typeof(required_agents_json) = 'array'
    ),
    CONSTRAINT chk_framework_selection_records_approval_reasons_array CHECK (
        jsonb_typeof(approval_reasons_json) = 'array'
    ),
    CONSTRAINT chk_framework_selection_records_evidence_array CHECK (
        jsonb_typeof(evidence_requirements_json) = 'array'
    ),
    CONSTRAINT chk_framework_selection_records_completion_array CHECK (
        jsonb_typeof(completion_criteria_json) = 'array'
    ),
    CONSTRAINT chk_framework_selection_records_learning_array CHECK (
        jsonb_typeof(learning_plan_json) = 'array'
    ),
    CONSTRAINT chk_framework_selection_records_context_array CHECK (
        jsonb_typeof(context_requirements_json) = 'array'
    )
);

CREATE INDEX IF NOT EXISTS idx_framework_selection_records_owner_created
    ON public.framework_selection_records USING btree (owner_identity, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_framework_selection_records_task_plan
    ON public.framework_selection_records USING btree (task_plan_id);
CREATE INDEX IF NOT EXISTS idx_framework_selection_records_request_hash
    ON public.framework_selection_records USING btree (request_hash);
CREATE INDEX IF NOT EXISTS idx_framework_selection_records_requires_approval
    ON public.framework_selection_records USING btree (requires_approval);
CREATE INDEX IF NOT EXISTS idx_framework_selection_records_reproducibility
    ON public.framework_selection_records USING btree (
        catalog_digest,
        selector_algorithm_version,
        effective_preference_digest,
        constitution_digest
    );

CREATE OR REPLACE FUNCTION public.hai_reject_framework_selection_mutation()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE EXCEPTION 'framework selection audit rows are immutable'
        USING ERRCODE = '55000';
END;
$$;

DROP TRIGGER IF EXISTS trg_framework_selection_records_immutable
    ON public.framework_selection_records;
CREATE TRIGGER trg_framework_selection_records_immutable
    BEFORE UPDATE OR DELETE ON public.framework_selection_records
    FOR EACH ROW
    EXECUTE FUNCTION public.hai_reject_framework_selection_mutation();

CREATE OR REPLACE FUNCTION public.hai_reject_framework_registry_truncate()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE EXCEPTION 'framework registry audit history cannot be truncated'
        USING ERRCODE = '55000';
END;
$$;

DROP TRIGGER IF EXISTS trg_framework_selection_records_no_truncate
    ON public.framework_selection_records;
CREATE TRIGGER trg_framework_selection_records_no_truncate
    BEFORE TRUNCATE ON public.framework_selection_records
    FOR EACH STATEMENT
    EXECUTE FUNCTION public.hai_reject_framework_registry_truncate();

CREATE TABLE IF NOT EXISTS public.robert_constitution_versions (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    owner_identity character varying(255) NOT NULL,
    version integer NOT NULL,
    base_version integer DEFAULT 0 NOT NULL,
    status character varying(32) DEFAULT 'draft'::character varying NOT NULL,
    values_json jsonb DEFAULT '[]'::jsonb NOT NULL,
    prohibitions_json jsonb DEFAULT '[]'::jsonb NOT NULL,
    standing_permissions_json jsonb DEFAULT '[]'::jsonb NOT NULL,
    preferences_json jsonb DEFAULT '[]'::jsonb NOT NULL,
    relationship_rules_json jsonb DEFAULT '[]'::jsonb NOT NULL,
    financial_boundaries_json jsonb DEFAULT '[]'::jsonb NOT NULL,
    communication_rules_json jsonb DEFAULT '[]'::jsonb NOT NULL,
    escalation_rules_json jsonb DEFAULT '[]'::jsonb NOT NULL,
    protected_rules_json jsonb DEFAULT '[]'::jsonb NOT NULL,
    change_summary text NOT NULL,
    approved_by character varying(255) DEFAULT ''::character varying NOT NULL,
    approval_note character varying(1024) DEFAULT ''::character varying NOT NULL,
    approved_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    CONSTRAINT robert_constitution_versions_pkey PRIMARY KEY (id),
    CONSTRAINT uq_robert_constitution_owner_version UNIQUE (owner_identity, version),
    CONSTRAINT chk_robert_constitution_owner CHECK (btrim(owner_identity) <> ''),
    CONSTRAINT chk_robert_constitution_version CHECK (version > 0),
    CONSTRAINT chk_robert_constitution_base_version CHECK (
        (version = 1 AND base_version = 0) OR
        (version > 1 AND base_version BETWEEN 1 AND version - 1)
    ),
    CONSTRAINT chk_robert_constitution_status CHECK (status IN ('draft', 'active', 'superseded')),
    CONSTRAINT chk_robert_constitution_approval CHECK (
        (
            status = 'draft' AND
            btrim(approved_by) = '' AND
            btrim(approval_note) = '' AND
            approved_at IS NULL
        ) OR (
            status IN ('active', 'superseded') AND
            btrim(approved_by) <> '' AND
            btrim(approval_note) <> '' AND
            approved_at IS NOT NULL
        )
    ),
    CONSTRAINT chk_robert_constitution_change_summary CHECK (
        btrim(change_summary) <> ''
    ),
    CONSTRAINT chk_robert_constitution_values_array CHECK (jsonb_typeof(values_json) = 'array'),
    CONSTRAINT chk_robert_constitution_prohibitions_array CHECK (jsonb_typeof(prohibitions_json) = 'array'),
    CONSTRAINT chk_robert_constitution_permissions_array CHECK (jsonb_typeof(standing_permissions_json) = 'array'),
    CONSTRAINT chk_robert_constitution_preferences_array CHECK (jsonb_typeof(preferences_json) = 'array'),
    CONSTRAINT chk_robert_constitution_relationship_array CHECK (jsonb_typeof(relationship_rules_json) = 'array'),
    CONSTRAINT chk_robert_constitution_financial_array CHECK (jsonb_typeof(financial_boundaries_json) = 'array'),
    CONSTRAINT chk_robert_constitution_communication_array CHECK (jsonb_typeof(communication_rules_json) = 'array'),
    CONSTRAINT chk_robert_constitution_escalation_array CHECK (jsonb_typeof(escalation_rules_json) = 'array'),
    CONSTRAINT chk_robert_constitution_protected_array CHECK (jsonb_typeof(protected_rules_json) = 'array')
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_robert_constitution_active_owner
    ON public.robert_constitution_versions USING btree (owner_identity)
    WHERE status = 'active';
CREATE INDEX IF NOT EXISTS idx_robert_constitution_owner_created
    ON public.robert_constitution_versions USING btree (owner_identity, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_robert_constitution_status
    ON public.robert_constitution_versions USING btree (status);

CREATE OR REPLACE FUNCTION public.hai_enforce_constitution_lifecycle()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    latest_activated_version integer;
BEGIN
    IF TG_OP = 'INSERT' THEN
        IF NEW.status <> 'draft' THEN
            RAISE EXCEPTION 'new constitution versions must start as drafts'
                USING ERRCODE = '23514';
        END IF;
        IF
            btrim(NEW.approved_by) <> '' OR
            btrim(NEW.approval_note) <> '' OR
            NEW.approved_at IS NOT NULL
        THEN
            RAISE EXCEPTION 'new constitution drafts cannot contain approval metadata'
                USING ERRCODE = '23514';
        END IF;
        RETURN NEW;
    END IF;

    IF TG_OP = 'DELETE' THEN
        RAISE EXCEPTION 'constitution versions are immutable history and cannot be deleted'
            USING ERRCODE = '55000';
    END IF;

    IF
        NEW.id IS DISTINCT FROM OLD.id OR
        NEW.owner_identity IS DISTINCT FROM OLD.owner_identity OR
        NEW.version IS DISTINCT FROM OLD.version OR
        NEW.base_version IS DISTINCT FROM OLD.base_version OR
        NEW.values_json IS DISTINCT FROM OLD.values_json OR
        NEW.prohibitions_json IS DISTINCT FROM OLD.prohibitions_json OR
        NEW.standing_permissions_json IS DISTINCT FROM OLD.standing_permissions_json OR
        NEW.preferences_json IS DISTINCT FROM OLD.preferences_json OR
        NEW.relationship_rules_json IS DISTINCT FROM OLD.relationship_rules_json OR
        NEW.financial_boundaries_json IS DISTINCT FROM OLD.financial_boundaries_json OR
        NEW.communication_rules_json IS DISTINCT FROM OLD.communication_rules_json OR
        NEW.escalation_rules_json IS DISTINCT FROM OLD.escalation_rules_json OR
        NEW.protected_rules_json IS DISTINCT FROM OLD.protected_rules_json OR
        NEW.change_summary IS DISTINCT FROM OLD.change_summary OR
        NEW.created_at IS DISTINCT FROM OLD.created_at
    THEN
        RAISE EXCEPTION 'constitution identity and content are immutable after insertion'
            USING ERRCODE = '55000';
    END IF;

    IF OLD.status = 'draft' AND NEW.status = 'active' THEN
        SELECT MAX(version)
        INTO latest_activated_version
        FROM public.robert_constitution_versions
        WHERE owner_identity = NEW.owner_identity
          AND id <> NEW.id
          AND status IN ('active', 'superseded');

        IF latest_activated_version IS NULL THEN
            IF NOT (
                (NEW.version = 1 AND NEW.base_version = 0) OR
                (NEW.version > 1 AND NEW.base_version = 1)
            ) THEN
                RAISE EXCEPTION
                    'initial constitution activation has invalid version/base pair %/%',
                    NEW.version,
                    NEW.base_version
                    USING ERRCODE = '55000';
            END IF;
        ELSIF NEW.base_version <> latest_activated_version THEN
            RAISE EXCEPTION
                'stale constitution activation: base version % does not match latest active history version %',
                NEW.base_version,
                latest_activated_version
                USING ERRCODE = '55000';
        END IF;

        IF
            btrim(NEW.approved_by) = '' OR
            btrim(NEW.approval_note) = '' OR
            NEW.approved_at IS NULL
        THEN
            RAISE EXCEPTION 'constitution activation requires approver, approval note, and approval timestamp'
                USING ERRCODE = '23514';
        END IF;
        RETURN NEW;
    END IF;

    IF OLD.status = 'active' AND NEW.status = 'superseded' THEN
        IF
            NEW.approved_by IS DISTINCT FROM OLD.approved_by OR
            NEW.approval_note IS DISTINCT FROM OLD.approval_note OR
            NEW.approved_at IS DISTINCT FROM OLD.approved_at
        THEN
            RAISE EXCEPTION 'superseding a constitution cannot alter activation metadata'
                USING ERRCODE = '55000';
        END IF;
        RETURN NEW;
    END IF;

    RAISE EXCEPTION 'invalid constitution lifecycle transition from % to %', OLD.status, NEW.status
        USING ERRCODE = '55000';
END;
$$;

DROP TRIGGER IF EXISTS trg_robert_constitution_versions_lifecycle
    ON public.robert_constitution_versions;
CREATE TRIGGER trg_robert_constitution_versions_lifecycle
    BEFORE INSERT OR UPDATE OR DELETE ON public.robert_constitution_versions
    FOR EACH ROW
    EXECUTE FUNCTION public.hai_enforce_constitution_lifecycle();

CREATE OR REPLACE FUNCTION public.hai_require_active_constitution_after_history()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    subject_owner character varying(255);
    activated_count bigint;
    active_count bigint;
BEGIN
    IF TG_OP = 'DELETE' THEN
        subject_owner := OLD.owner_identity;
    ELSE
        subject_owner := NEW.owner_identity;
    END IF;

    SELECT
        COUNT(*) FILTER (WHERE status IN ('active', 'superseded')),
        COUNT(*) FILTER (WHERE status = 'active')
    INTO activated_count, active_count
    FROM public.robert_constitution_versions
    WHERE owner_identity = subject_owner;

    IF activated_count > 0 AND active_count <> 1 THEN
        RAISE EXCEPTION
            'owner % has activated Constitution history but % active versions',
            subject_owner,
            active_count
            USING ERRCODE = '55000';
    END IF;
    RETURN NULL;
END;
$$;

DROP TRIGGER IF EXISTS trg_robert_constitution_active_history
    ON public.robert_constitution_versions;
CREATE CONSTRAINT TRIGGER trg_robert_constitution_active_history
    AFTER INSERT OR UPDATE OR DELETE ON public.robert_constitution_versions
    DEFERRABLE INITIALLY DEFERRED
    FOR EACH ROW
    EXECUTE FUNCTION public.hai_require_active_constitution_after_history();

DROP TRIGGER IF EXISTS trg_robert_constitution_versions_no_truncate
    ON public.robert_constitution_versions;
CREATE TRIGGER trg_robert_constitution_versions_no_truncate
    BEFORE TRUNCATE ON public.robert_constitution_versions
    FOR EACH STATEMENT
    EXECUTE FUNCTION public.hai_reject_framework_registry_truncate();
