CREATE TABLE IF NOT EXISTS public.life_entity_domain_links (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL PRIMARY KEY,
    owner_identity varchar(255) NOT NULL,
    entity_type varchar(80) NOT NULL,
    entity_id varchar(255) NOT NULL,
    domain_id varchar(80) NOT NULL,
    "primary" boolean DEFAULT false NOT NULL,
    confidence numeric(5,4) NOT NULL,
    source_label varchar(255) NOT NULL,
    source_uri text DEFAULT '' NOT NULL,
    evidence_json jsonb DEFAULT '[]'::jsonb NOT NULL,
    verification_status varchar(40) NOT NULL,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    CONSTRAINT uq_life_entity_domain_links_scope UNIQUE (owner_identity, entity_type, entity_id, domain_id),
    CONSTRAINT chk_life_entity_domain_links_owner CHECK (btrim(owner_identity) <> ''),
    CONSTRAINT chk_life_entity_domain_links_entity CHECK (btrim(entity_type) <> '' AND btrim(entity_id) <> ''),
    CONSTRAINT chk_life_entity_domain_links_confidence CHECK (confidence >= 0 AND confidence <= 1),
    CONSTRAINT chk_life_entity_domain_links_evidence CHECK (jsonb_typeof(evidence_json) = 'array')
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_life_entity_domain_links_primary
    ON public.life_entity_domain_links (owner_identity, entity_type, entity_id)
    WHERE "primary";
CREATE INDEX IF NOT EXISTS idx_life_entity_domain_links_owner_entity
    ON public.life_entity_domain_links (owner_identity, entity_type, entity_id);

CREATE TABLE IF NOT EXISTS public.life_need_observations (
    id uuid NOT NULL PRIMARY KEY,
    owner_identity varchar(255) NOT NULL,
    domain_id varchar(80) NOT NULL,
    need_level varchar(120) NOT NULL,
    state varchar(40) NOT NULL,
    current_level smallint NOT NULL,
    target_level smallint NOT NULL,
    gap smallint NOT NULL,
    priority smallint NOT NULL,
    confidence numeric(5,4) NOT NULL,
    evidence_json jsonb DEFAULT '[]'::jsonb NOT NULL,
    source_label varchar(255) NOT NULL,
    source_uri text DEFAULT '' NOT NULL,
    observed_at timestamptz NOT NULL,
    expires_at timestamptz,
    needs_review boolean DEFAULT false NOT NULL,
    created_at timestamptz NOT NULL,
    CONSTRAINT chk_life_need_observations_owner CHECK (btrim(owner_identity) <> ''),
    CONSTRAINT chk_life_need_observations_levels CHECK (
        current_level BETWEEN 0 AND 100 AND target_level BETWEEN 0 AND 100
        AND gap BETWEEN 0 AND 100 AND priority BETWEEN 0 AND 100
    ),
    CONSTRAINT chk_life_need_observations_confidence CHECK (confidence >= 0 AND confidence <= 1),
    CONSTRAINT chk_life_need_observations_evidence CHECK (jsonb_typeof(evidence_json) = 'array'),
    CONSTRAINT chk_life_need_observations_expiry CHECK (expires_at IS NULL OR expires_at > observed_at)
);
CREATE INDEX IF NOT EXISTS idx_life_need_observations_owner_observed
    ON public.life_need_observations (owner_identity, observed_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_life_need_observations_owner_domain
    ON public.life_need_observations (owner_identity, domain_id, observed_at DESC);

CREATE TABLE IF NOT EXISTS public.life_capacity_snapshots (
    id uuid NOT NULL PRIMARY KEY,
    owner_identity varchar(255) NOT NULL,
    status varchar(40) NOT NULL,
    signals_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    time_available_minutes integer DEFAULT 0 NOT NULL,
    concurrent_work_limit integer DEFAULT 0 NOT NULL,
    current_load smallint DEFAULT 0 NOT NULL,
    planning_step_limit smallint DEFAULT 1 NOT NULL,
    constraints_json jsonb DEFAULT '[]'::jsonb NOT NULL,
    source_label varchar(255) NOT NULL,
    source_uri text DEFAULT '' NOT NULL,
    captured_at timestamptz NOT NULL,
    confidence numeric(5,4) NOT NULL,
    fresh boolean DEFAULT false NOT NULL,
    needs_review boolean DEFAULT false NOT NULL,
    created_at timestamptz NOT NULL,
    CONSTRAINT chk_life_capacity_snapshots_owner CHECK (btrim(owner_identity) <> ''),
    CONSTRAINT chk_life_capacity_snapshots_limits CHECK (
        time_available_minutes >= 0 AND concurrent_work_limit >= 0
        AND current_load BETWEEN 0 AND 100 AND planning_step_limit BETWEEN 1 AND 20
    ),
    CONSTRAINT chk_life_capacity_snapshots_confidence CHECK (confidence >= 0 AND confidence <= 1),
    CONSTRAINT chk_life_capacity_snapshots_signals CHECK (jsonb_typeof(signals_json) = 'object'),
    CONSTRAINT chk_life_capacity_snapshots_constraints CHECK (jsonb_typeof(constraints_json) = 'array')
);
CREATE INDEX IF NOT EXISTS idx_life_capacity_snapshots_owner_captured
    ON public.life_capacity_snapshots (owner_identity, captured_at DESC, id DESC);

CREATE TABLE IF NOT EXISTS public.life_goal_nodes (
    id uuid NOT NULL PRIMARY KEY,
    owner_identity varchar(255) NOT NULL,
    parent_id uuid,
    level varchar(80) NOT NULL,
    domain_ids_json jsonb DEFAULT '[]'::jsonb NOT NULL,
    title varchar(500) NOT NULL,
    description text DEFAULT '' NOT NULL,
    success_criteria_json jsonb DEFAULT '[]'::jsonb NOT NULL,
    stop_conditions_json jsonb DEFAULT '[]'::jsonb NOT NULL,
    status varchar(40) NOT NULL,
    confidence numeric(5,4) NOT NULL,
    source_label varchar(255) NOT NULL,
    source_uri text DEFAULT '' NOT NULL,
    target_at timestamptz,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    CONSTRAINT uq_life_goal_nodes_owner_id UNIQUE (owner_identity, id),
    CONSTRAINT fk_life_goal_nodes_parent FOREIGN KEY (owner_identity, parent_id)
        REFERENCES public.life_goal_nodes(owner_identity, id) DEFERRABLE INITIALLY DEFERRED,
    CONSTRAINT chk_life_goal_nodes_owner CHECK (btrim(owner_identity) <> ''),
    CONSTRAINT chk_life_goal_nodes_title CHECK (btrim(title) <> ''),
    CONSTRAINT chk_life_goal_nodes_confidence CHECK (confidence >= 0 AND confidence <= 1),
    CONSTRAINT chk_life_goal_nodes_domains CHECK (
        jsonb_typeof(domain_ids_json) = 'array' AND jsonb_array_length(domain_ids_json) > 0
    ),
    CONSTRAINT chk_life_goal_nodes_success CHECK (jsonb_typeof(success_criteria_json) = 'array'),
    CONSTRAINT chk_life_goal_nodes_stop CHECK (jsonb_typeof(stop_conditions_json) = 'array'),
    CONSTRAINT chk_life_goal_nodes_not_self_parent CHECK (parent_id IS NULL OR parent_id <> id)
);
CREATE INDEX IF NOT EXISTS idx_life_goal_nodes_owner_level
    ON public.life_goal_nodes (owner_identity, level, created_at, id);
CREATE INDEX IF NOT EXISTS idx_life_goal_nodes_owner_status
    ON public.life_goal_nodes (owner_identity, status, updated_at DESC);

CREATE TABLE IF NOT EXISTS public.life_priority_assessments (
    id uuid NOT NULL PRIMARY KEY,
    owner_identity varchar(255) NOT NULL,
    entity_type varchar(80) NOT NULL,
    entity_id varchar(255) NOT NULL,
    title varchar(500) NOT NULL,
    score smallint NOT NULL,
    band varchar(40) NOT NULL,
    factors_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    contributions_json jsonb DEFAULT '[]'::jsonb NOT NULL,
    reasons_json jsonb DEFAULT '[]'::jsonb NOT NULL,
    capacity_applied boolean DEFAULT false NOT NULL,
    algorithm_version varchar(80) NOT NULL,
    source_label varchar(255) NOT NULL,
    source_uri text DEFAULT '' NOT NULL,
    assessed_at timestamptz NOT NULL,
    CONSTRAINT chk_life_priority_owner CHECK (btrim(owner_identity) <> ''),
    CONSTRAINT chk_life_priority_entity CHECK (btrim(entity_type) <> '' AND btrim(entity_id) <> ''),
    CONSTRAINT chk_life_priority_score CHECK (score BETWEEN 0 AND 100),
    CONSTRAINT chk_life_priority_json CHECK (
        jsonb_typeof(factors_json) = 'object'
        AND jsonb_typeof(contributions_json) = 'array'
        AND jsonb_typeof(reasons_json) = 'array'
    )
);
CREATE INDEX IF NOT EXISTS idx_life_priority_owner_assessed
    ON public.life_priority_assessments (owner_identity, assessed_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_life_priority_entity
    ON public.life_priority_assessments (owner_identity, entity_type, entity_id, assessed_at DESC);

CREATE OR REPLACE FUNCTION public.hai_reject_life_observation_mutation()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE EXCEPTION '% is append-only', TG_TABLE_NAME USING ERRCODE = '55000';
END;
$$;

DROP TRIGGER IF EXISTS trg_life_need_observations_immutable ON public.life_need_observations;
CREATE TRIGGER trg_life_need_observations_immutable
    BEFORE UPDATE OR DELETE ON public.life_need_observations
    FOR EACH ROW EXECUTE FUNCTION public.hai_reject_life_observation_mutation();

DROP TRIGGER IF EXISTS trg_life_capacity_snapshots_immutable ON public.life_capacity_snapshots;
CREATE TRIGGER trg_life_capacity_snapshots_immutable
    BEFORE UPDATE OR DELETE ON public.life_capacity_snapshots
    FOR EACH ROW EXECUTE FUNCTION public.hai_reject_life_observation_mutation();

DROP TRIGGER IF EXISTS trg_life_priority_assessments_immutable ON public.life_priority_assessments;
CREATE TRIGGER trg_life_priority_assessments_immutable
    BEFORE UPDATE OR DELETE ON public.life_priority_assessments
    FOR EACH ROW EXECUTE FUNCTION public.hai_reject_life_observation_mutation();
