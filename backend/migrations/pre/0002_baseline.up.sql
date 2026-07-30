-- Baseline schema: every table the application expects, generated from a
-- migrated database (pg_dump --schema-only) so production no longer depends on
-- Gorm AutoMigrate. Set DB_AUTOMIGRATE=false and this file is the source of truth.
--
-- Idempotent on purpose: tables/indexes use IF NOT EXISTS and constraints are
-- wrapped in exception-guarded DO blocks, so applying it to a database that was
-- already built by AutoMigrate is a safe no-op.
--
-- Regenerate with: scripts/generate-migration-baseline.sh
COMMENT ON SCHEMA public IS '';
CREATE EXTENSION IF NOT EXISTS "uuid-ossp" WITH SCHEMA public;
COMMENT ON EXTENSION "uuid-ossp" IS 'generate universally unique identifiers (UUIDs)';
CREATE TABLE IF NOT EXISTS public.ai_conversation_archives (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    owner_identity character varying(255),
    platform character varying(50) NOT NULL,
    external_id character varying(255) NOT NULL,
    title character varying(512),
    source_uri character varying(1024) NOT NULL,
    content_hash character varying(64) NOT NULL,
    revision bigint DEFAULT 1,
    message_count bigint,
    encrypted_payload bytea,
    encryption_nonce bytea,
    preview text,
    captured_at timestamp with time zone,
    last_message_at timestamp with time zone,
    archived boolean DEFAULT false,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);
CREATE TABLE IF NOT EXISTS public.ai_memory_insights (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    conversation_id uuid NOT NULL,
    owner_identity character varying(255),
    revision bigint NOT NULL,
    kind character varying(50) NOT NULL,
    text text NOT NULL,
    project_key character varying(255),
    owner character varying(120),
    robert_needed boolean,
    risk_level character varying(50),
    confidence numeric,
    source_uri character varying(1024),
    source_label character varying(512),
    needs_review boolean,
    status character varying(50),
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);
CREATE TABLE IF NOT EXISTS public.ambient_need_overrides (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    owner_identity character varying(255) NOT NULL,
    need_key character varying(80) NOT NULL,
    current_level bigint DEFAULT 0,
    target_level bigint DEFAULT 100,
    priority_weight bigint DEFAULT 50,
    enabled boolean DEFAULT true,
    notes text,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);
CREATE TABLE IF NOT EXISTS public.ambient_needs (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    key character varying(80) NOT NULL,
    name character varying(120) NOT NULL,
    description text,
    current_level bigint DEFAULT 0,
    target_level bigint DEFAULT 100,
    priority_weight bigint DEFAULT 50,
    enabled boolean DEFAULT true,
    notes text,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);
CREATE TABLE IF NOT EXISTS public.ambient_opportunities (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    owner_identity character varying(255),
    fingerprint character varying(64) NOT NULL,
    workflow_id uuid,
    need_key character varying(80) NOT NULL,
    title character varying(512) NOT NULL,
    rationale text NOT NULL,
    next_action text NOT NULL,
    source_type character varying(80),
    source_id character varying(160),
    source_uri character varying(1024),
    evidence_manifest text,
    resolution_note text,
    priority_score bigint,
    urgency bigint,
    impact bigint,
    effort bigint,
    confidence bigint,
    risk bigint,
    requires_approval boolean,
    status character varying(50) NOT NULL,
    last_seen_at timestamp with time zone,
    cooldown_until timestamp with time zone,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);
CREATE TABLE IF NOT EXISTS public.ambient_scans (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    owner_identity character varying(255),
    trigger character varying(80) NOT NULL,
    status character varying(50) NOT NULL,
    started_at timestamp with time zone,
    completed_at timestamp with time zone,
    items_examined bigint,
    opportunities_found bigint,
    created bigint,
    updated bigint,
    deduplicated bigint,
    advanced bigint,
    filtered bigint,
    skipped bigint,
    blocked bigint,
    manifest_bytes bigint,
    deduplicated_bytes bigint,
    error_message text,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);
CREATE TABLE IF NOT EXISTS public.automation_alerts (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    automation_id uuid,
    severity character varying(30),
    title character varying(255) NOT NULL,
    message text,
    status character varying(30) DEFAULT 'open'::character varying,
    first_seen_at timestamp with time zone,
    last_seen_at timestamp with time zone,
    acknowledged_at timestamp with time zone,
    resolved_at timestamp with time zone
);
CREATE TABLE IF NOT EXISTS public.automation_dependencies (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    automation_id uuid,
    name character varying(255) NOT NULL,
    kind character varying(50) NOT NULL,
    target character varying(1024),
    required boolean DEFAULT true,
    status character varying(30) DEFAULT 'unknown'::character varying,
    last_checked_at timestamp with time zone,
    notes text
);
CREATE TABLE IF NOT EXISTS public.automation_health_events (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    automation_id uuid,
    status character varying(30),
    check_type character varying(50),
    target character varying(1024),
    latency_ms bigint DEFAULT 0,
    failure_reason text,
    consecutive_failures bigint DEFAULT 0,
    checked_at timestamp with time zone
);
CREATE TABLE IF NOT EXISTS public.automation_incidents (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    automation_id uuid,
    title character varying(255) NOT NULL,
    severity character varying(30),
    status character varying(30) DEFAULT 'open'::character varying,
    started_at timestamp with time zone,
    resolved_at timestamp with time zone,
    root_cause text,
    resolution_note text
);
CREATE TABLE IF NOT EXISTS public.automation_launch_events (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    automation_id uuid,
    owner_identity character varying(255),
    runtime_type character varying(50),
    launch_type character varying(50),
    runtime_task_id character varying(120),
    target character varying(1024),
    status character varying(30),
    message text,
    output text,
    audit_events text,
    runtime_route_trace text,
    exit_code bigint DEFAULT 0,
    duration_ms bigint DEFAULT 0,
    started_at timestamp with time zone,
    completed_at timestamp with time zone
);
CREATE TABLE IF NOT EXISTS public.automation_route_checks (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    automation_id uuid,
    expected_route character varying(255),
    expected_host character varying(255),
    expected_port bigint,
    expected_status bigint DEFAULT 200,
    status character varying(30) DEFAULT 'unknown'::character varying,
    failure_reason text,
    last_checked_at timestamp with time zone
);
CREATE TABLE IF NOT EXISTS public.automation_slos (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    automation_id uuid,
    availability_target_pct numeric DEFAULT 99,
    max_latency_ms bigint DEFAULT 5000,
    max_consecutive_failures bigint DEFAULT 3,
    monitoring_window_hours bigint DEFAULT 24,
    notes text
);
CREATE TABLE IF NOT EXISTS public.automations (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    name character varying(50),
    url_path character varying(255),
    image character varying(255),
    host character varying(50),
    port bigint,
    "position" bigint,
    launch_type character varying(50) DEFAULT 'browser_url'::character varying,
    launch_target character varying(1024),
    runtime_type character varying(50),
    service_name character varying(255),
    route_path character varying(255),
    public_url character varying(1024),
    local_url character varying(1024),
    dependency_notes text,
    health_check_type character varying(50) DEFAULT 'http'::character varying,
    health_check_url character varying(1024),
    health_check_interval_seconds bigint DEFAULT 60,
    expected_http_status bigint DEFAULT 200,
    status character varying(30) DEFAULT 'unknown'::character varying,
    last_checked_at timestamp with time zone,
    last_success_at timestamp with time zone,
    last_failure_at timestamp with time zone,
    last_failure_reason text,
    consecutive_failures bigint DEFAULT 0,
    average_latency_ms bigint DEFAULT 0,
    last_launch_at timestamp with time zone,
    CONSTRAINT chk_automations_port CHECK (((port >= 0) AND (port <= 65535))),
    CONSTRAINT chk_automations_position CHECK (("position" >= 0))
);
CREATE TABLE IF NOT EXISTS public.autonomy_action_traces (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    workflow_id uuid NOT NULL,
    world_state_id uuid,
    attempt bigint NOT NULL,
    interface_type character varying(80) NOT NULL,
    action_type character varying(120) NOT NULL,
    action_payload text,
    status character varying(50) NOT NULL,
    policy_decision character varying(50) NOT NULL,
    policy_reason text,
    requires_approval boolean,
    approval_recorded boolean,
    execution_verified boolean,
    verification_status character varying(80),
    external_side_effects boolean,
    started_at timestamp with time zone NOT NULL,
    completed_at timestamp with time zone,
    latency_milliseconds bigint,
    result_summary text,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);
CREATE TABLE IF NOT EXISTS public.autonomy_evaluations (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    workflow_id uuid NOT NULL,
    action_trace_id uuid NOT NULL,
    attempt bigint NOT NULL,
    raw_completion boolean,
    execution_based_correctness boolean,
    completion_under_policy boolean,
    partial_completion boolean,
    policy_compliant boolean,
    risk_violation boolean,
    invalid_action boolean,
    human_intervention boolean,
    recovery_attempt boolean,
    recovered boolean,
    retry_count bigint,
    latency_milliseconds bigint,
    failure_mode character varying(120),
    created_at timestamp with time zone
);
CREATE TABLE IF NOT EXISTS public.autonomy_stress_runs (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    passed bigint,
    failed bigint,
    results text NOT NULL,
    created_at timestamp with time zone
);
CREATE TABLE IF NOT EXISTS public.autonomy_world_states (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    workflow_id uuid NOT NULL,
    attempt bigint NOT NULL,
    observation_type character varying(80) NOT NULL,
    state character varying(80) NOT NULL,
    snapshot text NOT NULL,
    confidence numeric,
    uncertainty numeric,
    source_revision character varying(64),
    observed_at timestamp with time zone NOT NULL,
    stale_after timestamp with time zone NOT NULL,
    partial boolean,
    requires_reobserve boolean,
    created_at timestamp with time zone
);
CREATE TABLE IF NOT EXISTS public.connected_sources (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    owner_identity character varying(255),
    connector_key character varying(80) NOT NULL,
    name character varying(255) NOT NULL,
    category character varying(80) NOT NULL,
    enabled boolean DEFAULT true,
    local_only boolean DEFAULT true,
    sync_frequency character varying(50) DEFAULT 'manual'::character varying,
    sync_target text,
    default_project_key character varying(255),
    ingestion_modes character varying(512),
    permissions character varying(1024),
    exclude_patterns text,
    cursor character varying(512),
    status character varying(50) DEFAULT 'active'::character varying,
    last_synced_at timestamp with time zone,
    revoked_at timestamp with time zone,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);
CREATE TABLE IF NOT EXISTS public.context_memories (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    owner_identity character varying(255),
    project_key character varying(255),
    kind character varying(50),
    content text NOT NULL,
    summary text,
    tags character varying(512),
    confidence numeric DEFAULT 0.7,
    source_uri character varying(1024),
    source_label character varying(255),
    content_hash character varying(64),
    archived boolean DEFAULT false,
    last_used_at timestamp with time zone,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);
CREATE TABLE IF NOT EXISTS public.durable_jobs (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    queue text DEFAULT 'default'::text NOT NULL,
    kind text NOT NULL,
    payload jsonb DEFAULT '{}'::jsonb NOT NULL,
    status text DEFAULT 'pending'::text NOT NULL,
    attempts bigint DEFAULT 0 NOT NULL,
    max_attempts bigint DEFAULT 5 NOT NULL,
    run_at timestamp with time zone NOT NULL,
    locked_by text,
    locked_at timestamp with time zone,
    last_error text,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    completed_at timestamp with time zone
);
CREATE TABLE IF NOT EXISTS public.llm_provider_probes (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    provider_id character varying(120) NOT NULL,
    provider_name character varying(255) NOT NULL,
    status character varying(80) NOT NULL,
    reason text,
    endpoint_url character varying(1024),
    http_status bigint,
    models_seen bigint,
    duration_ms bigint,
    live boolean,
    requires_review boolean,
    checked_at timestamp with time zone NOT NULL,
    last_successful_at timestamp with time zone,
    created_at timestamp with time zone
);
CREATE TABLE IF NOT EXISTS public.model_run_telemetries (
    id text NOT NULL,
    provider_id text NOT NULL,
    model_id text NOT NULL,
    lane text NOT NULL,
    operation_id text,
    input_tokens bigint DEFAULT 0 NOT NULL,
    output_tokens bigint DEFAULT 0 NOT NULL,
    duration_ms bigint DEFAULT 0 NOT NULL,
    tokens_per_second numeric DEFAULT 0 NOT NULL,
    ok boolean DEFAULT false NOT NULL,
    cache_hit boolean DEFAULT false NOT NULL,
    created_at timestamp with time zone
);
CREATE TABLE IF NOT EXISTS public.operation_events (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    operation_id uuid NOT NULL,
    event_type text NOT NULL,
    actor_type text NOT NULL,
    actor_id text,
    before_status text,
    after_status text,
    message text,
    payload_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone
);
CREATE TABLE IF NOT EXISTS public.operations (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    owner_user_id text NOT NULL,
    workspace_id text DEFAULT 'local'::text NOT NULL,
    title text NOT NULL,
    description text,
    source_type text NOT NULL,
    source_id uuid,
    source_uri text,
    source_received_at timestamp with time zone,
    source_revision_hash text,
    project_key text,
    pursuit_id uuid,
    workflow_id uuid,
    account_feed_id uuid,
    operation_type text NOT NULL,
    status text NOT NULL,
    risk_level text NOT NULL,
    autonomy_level text NOT NULL,
    owner_type text NOT NULL,
    current_decision text DEFAULT 'observe_only'::text NOT NULL,
    requires_approval boolean DEFAULT false NOT NULL,
    approval_id uuid,
    recommended_action text,
    evidence_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    world_model_state_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    runtime_id text,
    model_provider_id text,
    model_id text,
    verification_status text DEFAULT 'not_required'::text NOT NULL,
    result_summary text,
    last_error text,
    dedupe_key text NOT NULL,
    next_review_at timestamp with time zone,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    completed_at timestamp with time zone,
    version bigint DEFAULT 1 NOT NULL
);
CREATE TABLE IF NOT EXISTS public.pursuit_activities (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    pursuit_id uuid NOT NULL,
    event_type character varying(80) NOT NULL,
    message text,
    actor character varying(120),
    source_type character varying(80),
    source_id character varying(120),
    source_uri character varying(1024),
    created_at timestamp with time zone
);
CREATE TABLE IF NOT EXISTS public.pursuit_links (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    pursuit_id uuid NOT NULL,
    link_type character varying(80) NOT NULL,
    link_id character varying(120) NOT NULL,
    relationship character varying(80) NOT NULL,
    source_uri character varying(1024),
    source_label character varying(512),
    confidence numeric,
    created_at timestamp with time zone
);
CREATE TABLE IF NOT EXISTS public.pursuit_task_attempts (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    pursuit_id uuid NOT NULL,
    task_plan_id character varying(120) NOT NULL,
    owner_identity character varying(255),
    request_summary text,
    project_key character varying(255),
    mode character varying(40) NOT NULL,
    status character varying(80) NOT NULL,
    risk_level character varying(80),
    verification_status character varying(80),
    automation_id character varying(120),
    launch_event_id character varying(120),
    blocked_reason text,
    started_at timestamp with time zone,
    completed_at timestamp with time zone,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);
CREATE TABLE IF NOT EXISTS public.pursuits (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    owner_identity character varying(255),
    title character varying(512) NOT NULL,
    description text,
    why_it_matters text,
    project_key character varying(255),
    domain character varying(120),
    desired_outcome text,
    current_state_summary text,
    status character varying(80) NOT NULL,
    priority_score bigint,
    risk_level character varying(80),
    confidence numeric,
    autonomy_level character varying(80),
    need_category character varying(120),
    source_of_creation character varying(120),
    next_recommended_action text,
    completion_definition text,
    completion_state character varying(80),
    last_activity_at timestamp with time zone,
    next_review_at timestamp with time zone,
    archived boolean DEFAULT false,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);
CREATE TABLE IF NOT EXISTS public.source_audit_logs (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    source_id uuid,
    action character varying(80) NOT NULL,
    message text,
    created_at timestamp with time zone
);
CREATE TABLE IF NOT EXISTS public.source_connectors (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    connector_key character varying(80) NOT NULL,
    name character varying(255) NOT NULL,
    category character varying(80) NOT NULL,
    supported_modes character varying(512),
    required_scopes character varying(512),
    local_only_capable boolean DEFAULT true,
    enabled boolean DEFAULT false,
    adapter_status character varying(80) DEFAULT 'not_implemented'::character varying,
    status_reason text,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);
CREATE TABLE IF NOT EXISTS public.source_extractions (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    source_id uuid NOT NULL,
    raw_item_id uuid NOT NULL,
    project_key character varying(255),
    content_type character varying(80),
    text text,
    summary text,
    entities text,
    dates text,
    tasks text,
    decisions text,
    follow_ups text,
    source_uri character varying(1024),
    source_label character varying(512),
    content_hash character varying(64),
    sensitive boolean DEFAULT false,
    uncertain boolean DEFAULT false,
    archived boolean DEFAULT false,
    last_indexed_at timestamp with time zone,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);
CREATE TABLE IF NOT EXISTS public.source_index_entries (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    source_id uuid NOT NULL,
    extraction_id uuid NOT NULL,
    project_key character varying(255),
    index_type character varying(50) NOT NULL,
    keywords text,
    vector_ref character varying(512),
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);
CREATE TABLE IF NOT EXISTS public.source_o_auth_tokens (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    source_id uuid NOT NULL,
    provider character varying(50) NOT NULL,
    access_token bytea,
    refresh_token bytea,
    scope character varying(1024),
    expiry timestamp with time zone,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);
CREATE TABLE IF NOT EXISTS public.source_raw_items (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    source_id uuid NOT NULL,
    external_id character varying(255) NOT NULL,
    project_key character varying(255),
    item_type character varying(80),
    title character varying(512),
    source_uri character varying(1024),
    content text,
    metadata text,
    content_hash character varying(64),
    fetched_at timestamp with time zone,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);
CREATE TABLE IF NOT EXISTS public.source_sync_jobs (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    source_id uuid NOT NULL,
    mode character varying(50) NOT NULL,
    status character varying(50) NOT NULL,
    cursor_before character varying(512),
    cursor_after character varying(512),
    items_seen bigint,
    items_added bigint,
    items_updated bigint,
    items_failed bigint,
    message text,
    started_at timestamp with time zone,
    completed_at timestamp with time zone,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);
CREATE TABLE IF NOT EXISTS public.verification_audit_logs (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    run_id uuid,
    action character varying(80) NOT NULL,
    message text,
    created_at timestamp with time zone
);
CREATE TABLE IF NOT EXISTS public.verification_claims (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    run_id uuid NOT NULL,
    claim_text text NOT NULL,
    status character varying(50) NOT NULL,
    source_refs text,
    support_explanation text,
    confidence numeric DEFAULT 0,
    needs_review boolean DEFAULT false,
    high_risk boolean DEFAULT false,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);
CREATE TABLE IF NOT EXISTS public.verification_evidences (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    run_id uuid NOT NULL,
    source_type character varying(80) NOT NULL,
    source_id character varying(255),
    source_uri character varying(1024),
    source_label character varying(512),
    snippet text NOT NULL,
    authority character varying(80),
    freshness character varying(80),
    quality_score numeric DEFAULT 0,
    used boolean DEFAULT false,
    rejected boolean DEFAULT false,
    reject_reason text,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);
CREATE TABLE IF NOT EXISTS public.verification_runs (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    owner_identity character varying(255),
    mode character varying(40) NOT NULL,
    question text NOT NULL,
    project_key character varying(255),
    answer text,
    status character varying(50) NOT NULL,
    research_questions text,
    sources_searched text,
    sources_used text,
    sources_rejected text,
    missing_sources text,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);
CREATE TABLE IF NOT EXISTS public.workflow_checklist_items (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    workflow_id uuid NOT NULL,
    label text NOT NULL,
    status character varying(50) NOT NULL,
    "position" bigint,
    requires_approval boolean,
    due_at timestamp with time zone,
    reminder_at timestamp with time zone,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);
CREATE TABLE IF NOT EXISTS public.workflow_decisions (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    workflow_id uuid NOT NULL,
    decision_type character varying(80) NOT NULL,
    decision character varying(120) NOT NULL,
    reason text,
    rule_applied text,
    approved boolean,
    actor character varying(120),
    created_at timestamp with time zone
);
CREATE TABLE IF NOT EXISTS public.workflow_events (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    workflow_id uuid NOT NULL,
    event_type character varying(80) NOT NULL,
    from_state character varying(80),
    to_state character varying(80),
    message text,
    trigger character varying(255),
    rule_applied text,
    source_uri character varying(1024),
    actor character varying(120),
    created_at timestamp with time zone
);
CREATE TABLE IF NOT EXISTS public.workflow_evidence_claims (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    workflow_id uuid NOT NULL,
    claim_text text NOT NULL,
    source_uri character varying(1024),
    source_label character varying(512),
    reliability character varying(80),
    status character varying(80),
    needs_review boolean,
    created_at timestamp with time zone
);
CREATE TABLE IF NOT EXISTS public.workflow_intake_records (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    workflow_id uuid,
    source_type character varying(80),
    source_id character varying(120),
    source_uri character varying(1024),
    source_label character varying(512),
    content_type character varying(80),
    sender character varying(255),
    received_at timestamp with time zone,
    raw_content text,
    normalized_summary text,
    detected_entities text,
    possible_project character varying(255),
    urgency character varying(50),
    created_at timestamp with time zone
);
CREATE TABLE IF NOT EXISTS public.workflow_items (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    owner_identity character varying(255),
    title character varying(512) NOT NULL,
    description text,
    project_key character varying(255),
    automation_id character varying(64),
    current_state character varying(80) NOT NULL,
    task_type character varying(80),
    risk_level character varying(80),
    priority_score bigint,
    confidence numeric,
    autonomy_level character varying(80),
    requires_approval boolean,
    approval_status character varying(50),
    approval_reason text,
    blocked_reason text,
    next_action text,
    source_type character varying(80),
    source_id character varying(120),
    source_uri character varying(1024),
    source_label character varying(512),
    source_revision character varying(64),
    due_at timestamp with time zone,
    retry_count bigint DEFAULT 0,
    max_retries bigint DEFAULT 2,
    next_run_at timestamp with time zone,
    last_run_at timestamp with time zone,
    worker_claim_id character varying(64),
    worker_lease_until timestamp with time zone,
    completed_at timestamp with time zone,
    verification_status character varying(80),
    recovery_status character varying(80),
    recovery_note text,
    last_task_plan_id character varying(120),
    last_worker_error text,
    archived boolean DEFAULT false,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);
CREATE TABLE IF NOT EXISTS public.workflow_open_loops (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    workflow_id uuid NOT NULL,
    responsible_party character varying(120),
    waiting_for text,
    next_action text,
    follow_up_at timestamp with time zone,
    status character varying(50),
    claim_id character varying(64),
    lease_until timestamp with time zone,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);
CREATE TABLE IF NOT EXISTS public.workflow_project_matches (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    workflow_id uuid NOT NULL,
    project_key character varying(255) NOT NULL,
    matched_by text,
    confidence numeric,
    trello_card_ref character varying(512),
    drive_folder_ref character varying(512),
    created_at timestamp with time zone
);
CREATE TABLE IF NOT EXISTS public.workflow_proposals (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    workflow_id uuid NOT NULL,
    recommended_action text NOT NULL,
    options text,
    selected_option text,
    resolution_note text,
    resolved_by character varying(120),
    resolved_at timestamp with time zone,
    status character varying(50),
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);
CREATE TABLE IF NOT EXISTS public.workflow_quality_gates (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    workflow_id uuid NOT NULL,
    gate character varying(120) NOT NULL,
    status character varying(50) NOT NULL,
    reason text,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);
CREATE TABLE IF NOT EXISTS public.workflow_rules (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    rule_key character varying(120) NOT NULL,
    name character varying(255) NOT NULL,
    description text,
    category character varying(80),
    enabled boolean DEFAULT true,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);
CREATE TABLE IF NOT EXISTS public.workflow_source_links (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    workflow_id uuid NOT NULL,
    source_type character varying(80),
    source_id character varying(120),
    source_uri character varying(1024),
    source_label character varying(512),
    relationship character varying(80),
    created_at timestamp with time zone
);
CREATE TABLE IF NOT EXISTS public.workflow_transitions (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    workflow_id uuid NOT NULL,
    from_state character varying(80),
    to_state character varying(80) NOT NULL,
    trigger character varying(255),
    actor character varying(120),
    approved boolean,
    reason text,
    created_at timestamp with time zone
);
DO $$ BEGIN
ALTER TABLE ONLY public.ai_conversation_archives
    ADD CONSTRAINT ai_conversation_archives_pkey PRIMARY KEY (id);
EXCEPTION WHEN duplicate_object THEN NULL; WHEN duplicate_table THEN NULL; WHEN invalid_table_definition THEN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conrelid = 'public.ai_conversation_archives'::regclass
          AND contype = 'p'
          AND regexp_replace(pg_get_constraintdef(oid), '\s+', '', 'g')
              = regexp_replace('PRIMARY KEY (id)', '\s+', '', 'g')
    ) THEN
        RAISE;
    END IF;
END $$;
DO $$ BEGIN
ALTER TABLE ONLY public.ai_memory_insights
    ADD CONSTRAINT ai_memory_insights_pkey PRIMARY KEY (id);
EXCEPTION WHEN duplicate_object THEN NULL; WHEN duplicate_table THEN NULL; WHEN invalid_table_definition THEN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conrelid = 'public.ai_memory_insights'::regclass
          AND contype = 'p'
          AND regexp_replace(pg_get_constraintdef(oid), '\s+', '', 'g')
              = regexp_replace('PRIMARY KEY (id)', '\s+', '', 'g')
    ) THEN
        RAISE;
    END IF;
END $$;
DO $$ BEGIN
ALTER TABLE ONLY public.ambient_need_overrides
    ADD CONSTRAINT ambient_need_overrides_pkey PRIMARY KEY (id);
EXCEPTION WHEN duplicate_object THEN NULL; WHEN duplicate_table THEN NULL; WHEN invalid_table_definition THEN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conrelid = 'public.ambient_need_overrides'::regclass
          AND contype = 'p'
          AND regexp_replace(pg_get_constraintdef(oid), '\s+', '', 'g')
              = regexp_replace('PRIMARY KEY (id)', '\s+', '', 'g')
    ) THEN
        RAISE;
    END IF;
END $$;
DO $$ BEGIN
ALTER TABLE ONLY public.ambient_needs
    ADD CONSTRAINT ambient_needs_pkey PRIMARY KEY (id);
EXCEPTION WHEN duplicate_object THEN NULL; WHEN duplicate_table THEN NULL; WHEN invalid_table_definition THEN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conrelid = 'public.ambient_needs'::regclass
          AND contype = 'p'
          AND regexp_replace(pg_get_constraintdef(oid), '\s+', '', 'g')
              = regexp_replace('PRIMARY KEY (id)', '\s+', '', 'g')
    ) THEN
        RAISE;
    END IF;
END $$;
DO $$ BEGIN
ALTER TABLE ONLY public.ambient_opportunities
    ADD CONSTRAINT ambient_opportunities_pkey PRIMARY KEY (id);
EXCEPTION WHEN duplicate_object THEN NULL; WHEN duplicate_table THEN NULL; WHEN invalid_table_definition THEN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conrelid = 'public.ambient_opportunities'::regclass
          AND contype = 'p'
          AND regexp_replace(pg_get_constraintdef(oid), '\s+', '', 'g')
              = regexp_replace('PRIMARY KEY (id)', '\s+', '', 'g')
    ) THEN
        RAISE;
    END IF;
END $$;
DO $$ BEGIN
ALTER TABLE ONLY public.ambient_scans
    ADD CONSTRAINT ambient_scans_pkey PRIMARY KEY (id);
EXCEPTION WHEN duplicate_object THEN NULL; WHEN duplicate_table THEN NULL; WHEN invalid_table_definition THEN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conrelid = 'public.ambient_scans'::regclass
          AND contype = 'p'
          AND regexp_replace(pg_get_constraintdef(oid), '\s+', '', 'g')
              = regexp_replace('PRIMARY KEY (id)', '\s+', '', 'g')
    ) THEN
        RAISE;
    END IF;
END $$;
DO $$ BEGIN
ALTER TABLE ONLY public.automation_alerts
    ADD CONSTRAINT automation_alerts_pkey PRIMARY KEY (id);
EXCEPTION WHEN duplicate_object THEN NULL; WHEN duplicate_table THEN NULL; WHEN invalid_table_definition THEN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conrelid = 'public.automation_alerts'::regclass
          AND contype = 'p'
          AND regexp_replace(pg_get_constraintdef(oid), '\s+', '', 'g')
              = regexp_replace('PRIMARY KEY (id)', '\s+', '', 'g')
    ) THEN
        RAISE;
    END IF;
END $$;
DO $$ BEGIN
ALTER TABLE ONLY public.automation_dependencies
    ADD CONSTRAINT automation_dependencies_pkey PRIMARY KEY (id);
EXCEPTION WHEN duplicate_object THEN NULL; WHEN duplicate_table THEN NULL; WHEN invalid_table_definition THEN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conrelid = 'public.automation_dependencies'::regclass
          AND contype = 'p'
          AND regexp_replace(pg_get_constraintdef(oid), '\s+', '', 'g')
              = regexp_replace('PRIMARY KEY (id)', '\s+', '', 'g')
    ) THEN
        RAISE;
    END IF;
END $$;
DO $$ BEGIN
ALTER TABLE ONLY public.automation_health_events
    ADD CONSTRAINT automation_health_events_pkey PRIMARY KEY (id);
EXCEPTION WHEN duplicate_object THEN NULL; WHEN duplicate_table THEN NULL; WHEN invalid_table_definition THEN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conrelid = 'public.automation_health_events'::regclass
          AND contype = 'p'
          AND regexp_replace(pg_get_constraintdef(oid), '\s+', '', 'g')
              = regexp_replace('PRIMARY KEY (id)', '\s+', '', 'g')
    ) THEN
        RAISE;
    END IF;
END $$;
DO $$ BEGIN
ALTER TABLE ONLY public.automation_incidents
    ADD CONSTRAINT automation_incidents_pkey PRIMARY KEY (id);
EXCEPTION WHEN duplicate_object THEN NULL; WHEN duplicate_table THEN NULL; WHEN invalid_table_definition THEN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conrelid = 'public.automation_incidents'::regclass
          AND contype = 'p'
          AND regexp_replace(pg_get_constraintdef(oid), '\s+', '', 'g')
              = regexp_replace('PRIMARY KEY (id)', '\s+', '', 'g')
    ) THEN
        RAISE;
    END IF;
END $$;
DO $$ BEGIN
ALTER TABLE ONLY public.automation_launch_events
    ADD CONSTRAINT automation_launch_events_pkey PRIMARY KEY (id);
EXCEPTION WHEN duplicate_object THEN NULL; WHEN duplicate_table THEN NULL; WHEN invalid_table_definition THEN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conrelid = 'public.automation_launch_events'::regclass
          AND contype = 'p'
          AND regexp_replace(pg_get_constraintdef(oid), '\s+', '', 'g')
              = regexp_replace('PRIMARY KEY (id)', '\s+', '', 'g')
    ) THEN
        RAISE;
    END IF;
END $$;
DO $$ BEGIN
ALTER TABLE ONLY public.automation_route_checks
    ADD CONSTRAINT automation_route_checks_pkey PRIMARY KEY (id);
EXCEPTION WHEN duplicate_object THEN NULL; WHEN duplicate_table THEN NULL; WHEN invalid_table_definition THEN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conrelid = 'public.automation_route_checks'::regclass
          AND contype = 'p'
          AND regexp_replace(pg_get_constraintdef(oid), '\s+', '', 'g')
              = regexp_replace('PRIMARY KEY (id)', '\s+', '', 'g')
    ) THEN
        RAISE;
    END IF;
END $$;
DO $$ BEGIN
ALTER TABLE ONLY public.automation_slos
    ADD CONSTRAINT automation_slos_pkey PRIMARY KEY (id);
EXCEPTION WHEN duplicate_object THEN NULL; WHEN duplicate_table THEN NULL; WHEN invalid_table_definition THEN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conrelid = 'public.automation_slos'::regclass
          AND contype = 'p'
          AND regexp_replace(pg_get_constraintdef(oid), '\s+', '', 'g')
              = regexp_replace('PRIMARY KEY (id)', '\s+', '', 'g')
    ) THEN
        RAISE;
    END IF;
END $$;
DO $$ BEGIN
ALTER TABLE ONLY public.automations
    ADD CONSTRAINT automations_pkey PRIMARY KEY (id);
EXCEPTION WHEN duplicate_object THEN NULL; WHEN duplicate_table THEN NULL; WHEN invalid_table_definition THEN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conrelid = 'public.automations'::regclass
          AND contype = 'p'
          AND regexp_replace(pg_get_constraintdef(oid), '\s+', '', 'g')
              = regexp_replace('PRIMARY KEY (id)', '\s+', '', 'g')
    ) THEN
        RAISE;
    END IF;
END $$;
DO $$ BEGIN
ALTER TABLE ONLY public.autonomy_action_traces
    ADD CONSTRAINT autonomy_action_traces_pkey PRIMARY KEY (id);
EXCEPTION WHEN duplicate_object THEN NULL; WHEN duplicate_table THEN NULL; WHEN invalid_table_definition THEN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conrelid = 'public.autonomy_action_traces'::regclass
          AND contype = 'p'
          AND regexp_replace(pg_get_constraintdef(oid), '\s+', '', 'g')
              = regexp_replace('PRIMARY KEY (id)', '\s+', '', 'g')
    ) THEN
        RAISE;
    END IF;
END $$;
DO $$ BEGIN
ALTER TABLE ONLY public.autonomy_evaluations
    ADD CONSTRAINT autonomy_evaluations_pkey PRIMARY KEY (id);
EXCEPTION WHEN duplicate_object THEN NULL; WHEN duplicate_table THEN NULL; WHEN invalid_table_definition THEN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conrelid = 'public.autonomy_evaluations'::regclass
          AND contype = 'p'
          AND regexp_replace(pg_get_constraintdef(oid), '\s+', '', 'g')
              = regexp_replace('PRIMARY KEY (id)', '\s+', '', 'g')
    ) THEN
        RAISE;
    END IF;
END $$;
DO $$ BEGIN
ALTER TABLE ONLY public.autonomy_stress_runs
    ADD CONSTRAINT autonomy_stress_runs_pkey PRIMARY KEY (id);
EXCEPTION WHEN duplicate_object THEN NULL; WHEN duplicate_table THEN NULL; WHEN invalid_table_definition THEN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conrelid = 'public.autonomy_stress_runs'::regclass
          AND contype = 'p'
          AND regexp_replace(pg_get_constraintdef(oid), '\s+', '', 'g')
              = regexp_replace('PRIMARY KEY (id)', '\s+', '', 'g')
    ) THEN
        RAISE;
    END IF;
END $$;
DO $$ BEGIN
ALTER TABLE ONLY public.autonomy_world_states
    ADD CONSTRAINT autonomy_world_states_pkey PRIMARY KEY (id);
EXCEPTION WHEN duplicate_object THEN NULL; WHEN duplicate_table THEN NULL; WHEN invalid_table_definition THEN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conrelid = 'public.autonomy_world_states'::regclass
          AND contype = 'p'
          AND regexp_replace(pg_get_constraintdef(oid), '\s+', '', 'g')
              = regexp_replace('PRIMARY KEY (id)', '\s+', '', 'g')
    ) THEN
        RAISE;
    END IF;
END $$;
DO $$ BEGIN
ALTER TABLE ONLY public.connected_sources
    ADD CONSTRAINT connected_sources_pkey PRIMARY KEY (id);
EXCEPTION WHEN duplicate_object THEN NULL; WHEN duplicate_table THEN NULL; WHEN invalid_table_definition THEN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conrelid = 'public.connected_sources'::regclass
          AND contype = 'p'
          AND regexp_replace(pg_get_constraintdef(oid), '\s+', '', 'g')
              = regexp_replace('PRIMARY KEY (id)', '\s+', '', 'g')
    ) THEN
        RAISE;
    END IF;
END $$;
DO $$ BEGIN
ALTER TABLE ONLY public.context_memories
    ADD CONSTRAINT context_memories_pkey PRIMARY KEY (id);
EXCEPTION WHEN duplicate_object THEN NULL; WHEN duplicate_table THEN NULL; WHEN invalid_table_definition THEN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conrelid = 'public.context_memories'::regclass
          AND contype = 'p'
          AND regexp_replace(pg_get_constraintdef(oid), '\s+', '', 'g')
              = regexp_replace('PRIMARY KEY (id)', '\s+', '', 'g')
    ) THEN
        RAISE;
    END IF;
END $$;
DO $$ BEGIN
ALTER TABLE ONLY public.durable_jobs
    ADD CONSTRAINT durable_jobs_pkey PRIMARY KEY (id);
EXCEPTION WHEN duplicate_object THEN NULL; WHEN duplicate_table THEN NULL; WHEN invalid_table_definition THEN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conrelid = 'public.durable_jobs'::regclass
          AND contype = 'p'
          AND regexp_replace(pg_get_constraintdef(oid), '\s+', '', 'g')
              = regexp_replace('PRIMARY KEY (id)', '\s+', '', 'g')
    ) THEN
        RAISE;
    END IF;
END $$;
DO $$ BEGIN
ALTER TABLE ONLY public.llm_provider_probes
    ADD CONSTRAINT llm_provider_probes_pkey PRIMARY KEY (id);
EXCEPTION WHEN duplicate_object THEN NULL; WHEN duplicate_table THEN NULL; WHEN invalid_table_definition THEN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conrelid = 'public.llm_provider_probes'::regclass
          AND contype = 'p'
          AND regexp_replace(pg_get_constraintdef(oid), '\s+', '', 'g')
              = regexp_replace('PRIMARY KEY (id)', '\s+', '', 'g')
    ) THEN
        RAISE;
    END IF;
END $$;
DO $$ BEGIN
ALTER TABLE ONLY public.model_run_telemetries
    ADD CONSTRAINT model_run_telemetries_pkey PRIMARY KEY (id);
EXCEPTION WHEN duplicate_object THEN NULL; WHEN duplicate_table THEN NULL; WHEN invalid_table_definition THEN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conrelid = 'public.model_run_telemetries'::regclass
          AND contype = 'p'
          AND regexp_replace(pg_get_constraintdef(oid), '\s+', '', 'g')
              = regexp_replace('PRIMARY KEY (id)', '\s+', '', 'g')
    ) THEN
        RAISE;
    END IF;
END $$;
DO $$ BEGIN
ALTER TABLE ONLY public.operation_events
    ADD CONSTRAINT operation_events_pkey PRIMARY KEY (id);
EXCEPTION WHEN duplicate_object THEN NULL; WHEN duplicate_table THEN NULL; WHEN invalid_table_definition THEN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conrelid = 'public.operation_events'::regclass
          AND contype = 'p'
          AND regexp_replace(pg_get_constraintdef(oid), '\s+', '', 'g')
              = regexp_replace('PRIMARY KEY (id)', '\s+', '', 'g')
    ) THEN
        RAISE;
    END IF;
END $$;
DO $$ BEGIN
ALTER TABLE ONLY public.operations
    ADD CONSTRAINT operations_pkey PRIMARY KEY (id);
EXCEPTION WHEN duplicate_object THEN NULL; WHEN duplicate_table THEN NULL; WHEN invalid_table_definition THEN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conrelid = 'public.operations'::regclass
          AND contype = 'p'
          AND regexp_replace(pg_get_constraintdef(oid), '\s+', '', 'g')
              = regexp_replace('PRIMARY KEY (id)', '\s+', '', 'g')
    ) THEN
        RAISE;
    END IF;
END $$;
DO $$ BEGIN
ALTER TABLE ONLY public.pursuit_activities
    ADD CONSTRAINT pursuit_activities_pkey PRIMARY KEY (id);
EXCEPTION WHEN duplicate_object THEN NULL; WHEN duplicate_table THEN NULL; WHEN invalid_table_definition THEN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conrelid = 'public.pursuit_activities'::regclass
          AND contype = 'p'
          AND regexp_replace(pg_get_constraintdef(oid), '\s+', '', 'g')
              = regexp_replace('PRIMARY KEY (id)', '\s+', '', 'g')
    ) THEN
        RAISE;
    END IF;
END $$;
DO $$ BEGIN
ALTER TABLE ONLY public.pursuit_links
    ADD CONSTRAINT pursuit_links_pkey PRIMARY KEY (id);
EXCEPTION WHEN duplicate_object THEN NULL; WHEN duplicate_table THEN NULL; WHEN invalid_table_definition THEN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conrelid = 'public.pursuit_links'::regclass
          AND contype = 'p'
          AND regexp_replace(pg_get_constraintdef(oid), '\s+', '', 'g')
              = regexp_replace('PRIMARY KEY (id)', '\s+', '', 'g')
    ) THEN
        RAISE;
    END IF;
END $$;
DO $$ BEGIN
ALTER TABLE ONLY public.pursuit_task_attempts
    ADD CONSTRAINT pursuit_task_attempts_pkey PRIMARY KEY (id);
EXCEPTION WHEN duplicate_object THEN NULL; WHEN duplicate_table THEN NULL; WHEN invalid_table_definition THEN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conrelid = 'public.pursuit_task_attempts'::regclass
          AND contype = 'p'
          AND regexp_replace(pg_get_constraintdef(oid), '\s+', '', 'g')
              = regexp_replace('PRIMARY KEY (id)', '\s+', '', 'g')
    ) THEN
        RAISE;
    END IF;
END $$;
DO $$ BEGIN
ALTER TABLE ONLY public.pursuits
    ADD CONSTRAINT pursuits_pkey PRIMARY KEY (id);
EXCEPTION WHEN duplicate_object THEN NULL; WHEN duplicate_table THEN NULL; WHEN invalid_table_definition THEN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conrelid = 'public.pursuits'::regclass
          AND contype = 'p'
          AND regexp_replace(pg_get_constraintdef(oid), '\s+', '', 'g')
              = regexp_replace('PRIMARY KEY (id)', '\s+', '', 'g')
    ) THEN
        RAISE;
    END IF;
END $$;
DO $$ BEGIN
ALTER TABLE ONLY public.source_audit_logs
    ADD CONSTRAINT source_audit_logs_pkey PRIMARY KEY (id);
EXCEPTION WHEN duplicate_object THEN NULL; WHEN duplicate_table THEN NULL; WHEN invalid_table_definition THEN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conrelid = 'public.source_audit_logs'::regclass
          AND contype = 'p'
          AND regexp_replace(pg_get_constraintdef(oid), '\s+', '', 'g')
              = regexp_replace('PRIMARY KEY (id)', '\s+', '', 'g')
    ) THEN
        RAISE;
    END IF;
END $$;
DO $$ BEGIN
ALTER TABLE ONLY public.source_connectors
    ADD CONSTRAINT source_connectors_pkey PRIMARY KEY (id);
EXCEPTION WHEN duplicate_object THEN NULL; WHEN duplicate_table THEN NULL; WHEN invalid_table_definition THEN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conrelid = 'public.source_connectors'::regclass
          AND contype = 'p'
          AND regexp_replace(pg_get_constraintdef(oid), '\s+', '', 'g')
              = regexp_replace('PRIMARY KEY (id)', '\s+', '', 'g')
    ) THEN
        RAISE;
    END IF;
END $$;
DO $$ BEGIN
ALTER TABLE ONLY public.source_extractions
    ADD CONSTRAINT source_extractions_pkey PRIMARY KEY (id);
EXCEPTION WHEN duplicate_object THEN NULL; WHEN duplicate_table THEN NULL; WHEN invalid_table_definition THEN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conrelid = 'public.source_extractions'::regclass
          AND contype = 'p'
          AND regexp_replace(pg_get_constraintdef(oid), '\s+', '', 'g')
              = regexp_replace('PRIMARY KEY (id)', '\s+', '', 'g')
    ) THEN
        RAISE;
    END IF;
END $$;
DO $$ BEGIN
ALTER TABLE ONLY public.source_index_entries
    ADD CONSTRAINT source_index_entries_pkey PRIMARY KEY (id);
EXCEPTION WHEN duplicate_object THEN NULL; WHEN duplicate_table THEN NULL; WHEN invalid_table_definition THEN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conrelid = 'public.source_index_entries'::regclass
          AND contype = 'p'
          AND regexp_replace(pg_get_constraintdef(oid), '\s+', '', 'g')
              = regexp_replace('PRIMARY KEY (id)', '\s+', '', 'g')
    ) THEN
        RAISE;
    END IF;
END $$;
DO $$ BEGIN
ALTER TABLE ONLY public.source_o_auth_tokens
    ADD CONSTRAINT source_o_auth_tokens_pkey PRIMARY KEY (id);
EXCEPTION WHEN duplicate_object THEN NULL; WHEN duplicate_table THEN NULL; WHEN invalid_table_definition THEN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conrelid = 'public.source_o_auth_tokens'::regclass
          AND contype = 'p'
          AND regexp_replace(pg_get_constraintdef(oid), '\s+', '', 'g')
              = regexp_replace('PRIMARY KEY (id)', '\s+', '', 'g')
    ) THEN
        RAISE;
    END IF;
END $$;
DO $$ BEGIN
ALTER TABLE ONLY public.source_raw_items
    ADD CONSTRAINT source_raw_items_pkey PRIMARY KEY (id);
EXCEPTION WHEN duplicate_object THEN NULL; WHEN duplicate_table THEN NULL; WHEN invalid_table_definition THEN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conrelid = 'public.source_raw_items'::regclass
          AND contype = 'p'
          AND regexp_replace(pg_get_constraintdef(oid), '\s+', '', 'g')
              = regexp_replace('PRIMARY KEY (id)', '\s+', '', 'g')
    ) THEN
        RAISE;
    END IF;
END $$;
DO $$ BEGIN
ALTER TABLE ONLY public.source_sync_jobs
    ADD CONSTRAINT source_sync_jobs_pkey PRIMARY KEY (id);
EXCEPTION WHEN duplicate_object THEN NULL; WHEN duplicate_table THEN NULL; WHEN invalid_table_definition THEN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conrelid = 'public.source_sync_jobs'::regclass
          AND contype = 'p'
          AND regexp_replace(pg_get_constraintdef(oid), '\s+', '', 'g')
              = regexp_replace('PRIMARY KEY (id)', '\s+', '', 'g')
    ) THEN
        RAISE;
    END IF;
END $$;
DO $$ BEGIN
ALTER TABLE ONLY public.automations
    ADD CONSTRAINT uni_automations_name UNIQUE (name);
EXCEPTION WHEN duplicate_object THEN NULL; WHEN duplicate_table THEN NULL; END $$;
DO $$ BEGIN
ALTER TABLE ONLY public.automations
    ADD CONSTRAINT uni_automations_position UNIQUE ("position");
EXCEPTION WHEN duplicate_object THEN NULL; WHEN duplicate_table THEN NULL; END $$;
DO $$ BEGIN
ALTER TABLE ONLY public.automations
    ADD CONSTRAINT uni_automations_url_path UNIQUE (url_path);
EXCEPTION WHEN duplicate_object THEN NULL; WHEN duplicate_table THEN NULL; END $$;
DO $$ BEGIN
ALTER TABLE ONLY public.verification_audit_logs
    ADD CONSTRAINT verification_audit_logs_pkey PRIMARY KEY (id);
EXCEPTION WHEN duplicate_object THEN NULL; WHEN duplicate_table THEN NULL; WHEN invalid_table_definition THEN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conrelid = 'public.verification_audit_logs'::regclass
          AND contype = 'p'
          AND regexp_replace(pg_get_constraintdef(oid), '\s+', '', 'g')
              = regexp_replace('PRIMARY KEY (id)', '\s+', '', 'g')
    ) THEN
        RAISE;
    END IF;
END $$;
DO $$ BEGIN
ALTER TABLE ONLY public.verification_claims
    ADD CONSTRAINT verification_claims_pkey PRIMARY KEY (id);
EXCEPTION WHEN duplicate_object THEN NULL; WHEN duplicate_table THEN NULL; WHEN invalid_table_definition THEN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conrelid = 'public.verification_claims'::regclass
          AND contype = 'p'
          AND regexp_replace(pg_get_constraintdef(oid), '\s+', '', 'g')
              = regexp_replace('PRIMARY KEY (id)', '\s+', '', 'g')
    ) THEN
        RAISE;
    END IF;
END $$;
DO $$ BEGIN
ALTER TABLE ONLY public.verification_evidences
    ADD CONSTRAINT verification_evidences_pkey PRIMARY KEY (id);
EXCEPTION WHEN duplicate_object THEN NULL; WHEN duplicate_table THEN NULL; WHEN invalid_table_definition THEN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conrelid = 'public.verification_evidences'::regclass
          AND contype = 'p'
          AND regexp_replace(pg_get_constraintdef(oid), '\s+', '', 'g')
              = regexp_replace('PRIMARY KEY (id)', '\s+', '', 'g')
    ) THEN
        RAISE;
    END IF;
END $$;
DO $$ BEGIN
ALTER TABLE ONLY public.verification_runs
    ADD CONSTRAINT verification_runs_pkey PRIMARY KEY (id);
EXCEPTION WHEN duplicate_object THEN NULL; WHEN duplicate_table THEN NULL; WHEN invalid_table_definition THEN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conrelid = 'public.verification_runs'::regclass
          AND contype = 'p'
          AND regexp_replace(pg_get_constraintdef(oid), '\s+', '', 'g')
              = regexp_replace('PRIMARY KEY (id)', '\s+', '', 'g')
    ) THEN
        RAISE;
    END IF;
END $$;
DO $$ BEGIN
ALTER TABLE ONLY public.workflow_checklist_items
    ADD CONSTRAINT workflow_checklist_items_pkey PRIMARY KEY (id);
EXCEPTION WHEN duplicate_object THEN NULL; WHEN duplicate_table THEN NULL; WHEN invalid_table_definition THEN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conrelid = 'public.workflow_checklist_items'::regclass
          AND contype = 'p'
          AND regexp_replace(pg_get_constraintdef(oid), '\s+', '', 'g')
              = regexp_replace('PRIMARY KEY (id)', '\s+', '', 'g')
    ) THEN
        RAISE;
    END IF;
END $$;
DO $$ BEGIN
ALTER TABLE ONLY public.workflow_decisions
    ADD CONSTRAINT workflow_decisions_pkey PRIMARY KEY (id);
EXCEPTION WHEN duplicate_object THEN NULL; WHEN duplicate_table THEN NULL; WHEN invalid_table_definition THEN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conrelid = 'public.workflow_decisions'::regclass
          AND contype = 'p'
          AND regexp_replace(pg_get_constraintdef(oid), '\s+', '', 'g')
              = regexp_replace('PRIMARY KEY (id)', '\s+', '', 'g')
    ) THEN
        RAISE;
    END IF;
END $$;
DO $$ BEGIN
ALTER TABLE ONLY public.workflow_events
    ADD CONSTRAINT workflow_events_pkey PRIMARY KEY (id);
EXCEPTION WHEN duplicate_object THEN NULL; WHEN duplicate_table THEN NULL; WHEN invalid_table_definition THEN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conrelid = 'public.workflow_events'::regclass
          AND contype = 'p'
          AND regexp_replace(pg_get_constraintdef(oid), '\s+', '', 'g')
              = regexp_replace('PRIMARY KEY (id)', '\s+', '', 'g')
    ) THEN
        RAISE;
    END IF;
END $$;
DO $$ BEGIN
ALTER TABLE ONLY public.workflow_evidence_claims
    ADD CONSTRAINT workflow_evidence_claims_pkey PRIMARY KEY (id);
EXCEPTION WHEN duplicate_object THEN NULL; WHEN duplicate_table THEN NULL; WHEN invalid_table_definition THEN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conrelid = 'public.workflow_evidence_claims'::regclass
          AND contype = 'p'
          AND regexp_replace(pg_get_constraintdef(oid), '\s+', '', 'g')
              = regexp_replace('PRIMARY KEY (id)', '\s+', '', 'g')
    ) THEN
        RAISE;
    END IF;
END $$;
DO $$ BEGIN
ALTER TABLE ONLY public.workflow_intake_records
    ADD CONSTRAINT workflow_intake_records_pkey PRIMARY KEY (id);
EXCEPTION WHEN duplicate_object THEN NULL; WHEN duplicate_table THEN NULL; WHEN invalid_table_definition THEN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conrelid = 'public.workflow_intake_records'::regclass
          AND contype = 'p'
          AND regexp_replace(pg_get_constraintdef(oid), '\s+', '', 'g')
              = regexp_replace('PRIMARY KEY (id)', '\s+', '', 'g')
    ) THEN
        RAISE;
    END IF;
END $$;
DO $$ BEGIN
ALTER TABLE ONLY public.workflow_items
    ADD CONSTRAINT workflow_items_pkey PRIMARY KEY (id);
EXCEPTION WHEN duplicate_object THEN NULL; WHEN duplicate_table THEN NULL; WHEN invalid_table_definition THEN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conrelid = 'public.workflow_items'::regclass
          AND contype = 'p'
          AND regexp_replace(pg_get_constraintdef(oid), '\s+', '', 'g')
              = regexp_replace('PRIMARY KEY (id)', '\s+', '', 'g')
    ) THEN
        RAISE;
    END IF;
END $$;
DO $$ BEGIN
ALTER TABLE ONLY public.workflow_open_loops
    ADD CONSTRAINT workflow_open_loops_pkey PRIMARY KEY (id);
EXCEPTION WHEN duplicate_object THEN NULL; WHEN duplicate_table THEN NULL; WHEN invalid_table_definition THEN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conrelid = 'public.workflow_open_loops'::regclass
          AND contype = 'p'
          AND regexp_replace(pg_get_constraintdef(oid), '\s+', '', 'g')
              = regexp_replace('PRIMARY KEY (id)', '\s+', '', 'g')
    ) THEN
        RAISE;
    END IF;
END $$;
DO $$ BEGIN
ALTER TABLE ONLY public.workflow_project_matches
    ADD CONSTRAINT workflow_project_matches_pkey PRIMARY KEY (id);
EXCEPTION WHEN duplicate_object THEN NULL; WHEN duplicate_table THEN NULL; WHEN invalid_table_definition THEN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conrelid = 'public.workflow_project_matches'::regclass
          AND contype = 'p'
          AND regexp_replace(pg_get_constraintdef(oid), '\s+', '', 'g')
              = regexp_replace('PRIMARY KEY (id)', '\s+', '', 'g')
    ) THEN
        RAISE;
    END IF;
END $$;
DO $$ BEGIN
ALTER TABLE ONLY public.workflow_proposals
    ADD CONSTRAINT workflow_proposals_pkey PRIMARY KEY (id);
EXCEPTION WHEN duplicate_object THEN NULL; WHEN duplicate_table THEN NULL; WHEN invalid_table_definition THEN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conrelid = 'public.workflow_proposals'::regclass
          AND contype = 'p'
          AND regexp_replace(pg_get_constraintdef(oid), '\s+', '', 'g')
              = regexp_replace('PRIMARY KEY (id)', '\s+', '', 'g')
    ) THEN
        RAISE;
    END IF;
END $$;
DO $$ BEGIN
ALTER TABLE ONLY public.workflow_quality_gates
    ADD CONSTRAINT workflow_quality_gates_pkey PRIMARY KEY (id);
EXCEPTION WHEN duplicate_object THEN NULL; WHEN duplicate_table THEN NULL; WHEN invalid_table_definition THEN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conrelid = 'public.workflow_quality_gates'::regclass
          AND contype = 'p'
          AND regexp_replace(pg_get_constraintdef(oid), '\s+', '', 'g')
              = regexp_replace('PRIMARY KEY (id)', '\s+', '', 'g')
    ) THEN
        RAISE;
    END IF;
END $$;
DO $$ BEGIN
ALTER TABLE ONLY public.workflow_rules
    ADD CONSTRAINT workflow_rules_pkey PRIMARY KEY (id);
EXCEPTION WHEN duplicate_object THEN NULL; WHEN duplicate_table THEN NULL; WHEN invalid_table_definition THEN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conrelid = 'public.workflow_rules'::regclass
          AND contype = 'p'
          AND regexp_replace(pg_get_constraintdef(oid), '\s+', '', 'g')
              = regexp_replace('PRIMARY KEY (id)', '\s+', '', 'g')
    ) THEN
        RAISE;
    END IF;
END $$;
DO $$ BEGIN
ALTER TABLE ONLY public.workflow_source_links
    ADD CONSTRAINT workflow_source_links_pkey PRIMARY KEY (id);
EXCEPTION WHEN duplicate_object THEN NULL; WHEN duplicate_table THEN NULL; WHEN invalid_table_definition THEN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conrelid = 'public.workflow_source_links'::regclass
          AND contype = 'p'
          AND regexp_replace(pg_get_constraintdef(oid), '\s+', '', 'g')
              = regexp_replace('PRIMARY KEY (id)', '\s+', '', 'g')
    ) THEN
        RAISE;
    END IF;
END $$;
DO $$ BEGIN
ALTER TABLE ONLY public.workflow_transitions
    ADD CONSTRAINT workflow_transitions_pkey PRIMARY KEY (id);
EXCEPTION WHEN duplicate_object THEN NULL; WHEN duplicate_table THEN NULL; WHEN invalid_table_definition THEN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conrelid = 'public.workflow_transitions'::regclass
          AND contype = 'p'
          AND regexp_replace(pg_get_constraintdef(oid), '\s+', '', 'g')
              = regexp_replace('PRIMARY KEY (id)', '\s+', '', 'g')
    ) THEN
        RAISE;
    END IF;
END $$;
CREATE INDEX IF NOT EXISTS idx_ai_conversation_archives_archived ON public.ai_conversation_archives USING btree (archived);
CREATE INDEX IF NOT EXISTS idx_ai_conversation_archives_captured_at ON public.ai_conversation_archives USING btree (captured_at);
CREATE INDEX IF NOT EXISTS idx_ai_conversation_archives_content_hash ON public.ai_conversation_archives USING btree (content_hash);
CREATE INDEX IF NOT EXISTS idx_ai_conversation_archives_external_id ON public.ai_conversation_archives USING btree (external_id);
CREATE INDEX IF NOT EXISTS idx_ai_conversation_archives_last_message_at ON public.ai_conversation_archives USING btree (last_message_at);
CREATE INDEX IF NOT EXISTS idx_ai_conversation_archives_owner_identity ON public.ai_conversation_archives USING btree (owner_identity);
CREATE INDEX IF NOT EXISTS idx_ai_conversation_archives_platform ON public.ai_conversation_archives USING btree (platform);
CREATE INDEX IF NOT EXISTS idx_ai_conversation_archives_source_uri ON public.ai_conversation_archives USING btree (source_uri);
CREATE INDEX IF NOT EXISTS idx_ai_conversation_archives_title ON public.ai_conversation_archives USING btree (title);
CREATE UNIQUE INDEX IF NOT EXISTS idx_ai_conversation_owner_identity ON public.ai_conversation_archives USING btree (owner_identity, platform, external_id);
CREATE INDEX IF NOT EXISTS idx_ai_memory_insights_conversation_id ON public.ai_memory_insights USING btree (conversation_id);
CREATE INDEX IF NOT EXISTS idx_ai_memory_insights_kind ON public.ai_memory_insights USING btree (kind);
CREATE INDEX IF NOT EXISTS idx_ai_memory_insights_needs_review ON public.ai_memory_insights USING btree (needs_review);
CREATE INDEX IF NOT EXISTS idx_ai_memory_insights_owner ON public.ai_memory_insights USING btree (owner);
CREATE INDEX IF NOT EXISTS idx_ai_memory_insights_owner_identity ON public.ai_memory_insights USING btree (owner_identity);
CREATE INDEX IF NOT EXISTS idx_ai_memory_insights_project_key ON public.ai_memory_insights USING btree (project_key);
CREATE INDEX IF NOT EXISTS idx_ai_memory_insights_revision ON public.ai_memory_insights USING btree (revision);
CREATE INDEX IF NOT EXISTS idx_ai_memory_insights_risk_level ON public.ai_memory_insights USING btree (risk_level);
CREATE INDEX IF NOT EXISTS idx_ai_memory_insights_robert_needed ON public.ai_memory_insights USING btree (robert_needed);
CREATE INDEX IF NOT EXISTS idx_ai_memory_insights_source_uri ON public.ai_memory_insights USING btree (source_uri);
CREATE INDEX IF NOT EXISTS idx_ai_memory_insights_status ON public.ai_memory_insights USING btree (status);
CREATE UNIQUE INDEX IF NOT EXISTS idx_ambient_need_override_owner_key ON public.ambient_need_overrides USING btree (owner_identity, need_key);
CREATE INDEX IF NOT EXISTS idx_ambient_need_overrides_enabled ON public.ambient_need_overrides USING btree (enabled);
CREATE INDEX IF NOT EXISTS idx_ambient_needs_enabled ON public.ambient_needs USING btree (enabled);
CREATE UNIQUE INDEX IF NOT EXISTS idx_ambient_needs_key ON public.ambient_needs USING btree (key);
CREATE INDEX IF NOT EXISTS idx_ambient_opportunities_cooldown_until ON public.ambient_opportunities USING btree (cooldown_until);
CREATE UNIQUE INDEX IF NOT EXISTS idx_ambient_opportunities_fingerprint ON public.ambient_opportunities USING btree (fingerprint);
CREATE INDEX IF NOT EXISTS idx_ambient_opportunities_last_seen_at ON public.ambient_opportunities USING btree (last_seen_at);
CREATE INDEX IF NOT EXISTS idx_ambient_opportunities_need_key ON public.ambient_opportunities USING btree (need_key);
CREATE INDEX IF NOT EXISTS idx_ambient_opportunities_owner_identity ON public.ambient_opportunities USING btree (owner_identity);
CREATE INDEX IF NOT EXISTS idx_ambient_opportunities_priority_score ON public.ambient_opportunities USING btree (priority_score);
CREATE INDEX IF NOT EXISTS idx_ambient_opportunities_requires_approval ON public.ambient_opportunities USING btree (requires_approval);
CREATE INDEX IF NOT EXISTS idx_ambient_opportunities_source_id ON public.ambient_opportunities USING btree (source_id);
CREATE INDEX IF NOT EXISTS idx_ambient_opportunities_source_type ON public.ambient_opportunities USING btree (source_type);
CREATE INDEX IF NOT EXISTS idx_ambient_opportunities_status ON public.ambient_opportunities USING btree (status);
CREATE INDEX IF NOT EXISTS idx_ambient_opportunities_workflow_id ON public.ambient_opportunities USING btree (workflow_id);
CREATE INDEX IF NOT EXISTS idx_ambient_scans_completed_at ON public.ambient_scans USING btree (completed_at);
CREATE INDEX IF NOT EXISTS idx_ambient_scans_owner_identity ON public.ambient_scans USING btree (owner_identity);
CREATE INDEX IF NOT EXISTS idx_ambient_scans_started_at ON public.ambient_scans USING btree (started_at);
CREATE INDEX IF NOT EXISTS idx_ambient_scans_status ON public.ambient_scans USING btree (status);
CREATE INDEX IF NOT EXISTS idx_ambient_scans_trigger ON public.ambient_scans USING btree (trigger);
CREATE INDEX IF NOT EXISTS idx_automation_alerts_automation_id ON public.automation_alerts USING btree (automation_id);
CREATE INDEX IF NOT EXISTS idx_automation_alerts_first_seen_at ON public.automation_alerts USING btree (first_seen_at);
CREATE INDEX IF NOT EXISTS idx_automation_alerts_last_seen_at ON public.automation_alerts USING btree (last_seen_at);
CREATE INDEX IF NOT EXISTS idx_automation_alerts_severity ON public.automation_alerts USING btree (severity);
CREATE INDEX IF NOT EXISTS idx_automation_alerts_status ON public.automation_alerts USING btree (status);
CREATE INDEX IF NOT EXISTS idx_automation_dependencies_automation_id ON public.automation_dependencies USING btree (automation_id);
CREATE INDEX IF NOT EXISTS idx_automation_health_events_automation_id ON public.automation_health_events USING btree (automation_id);
CREATE INDEX IF NOT EXISTS idx_automation_health_events_checked_at ON public.automation_health_events USING btree (checked_at);
CREATE INDEX IF NOT EXISTS idx_automation_health_events_status ON public.automation_health_events USING btree (status);
CREATE INDEX IF NOT EXISTS idx_automation_incidents_automation_id ON public.automation_incidents USING btree (automation_id);
CREATE INDEX IF NOT EXISTS idx_automation_incidents_severity ON public.automation_incidents USING btree (severity);
CREATE INDEX IF NOT EXISTS idx_automation_incidents_started_at ON public.automation_incidents USING btree (started_at);
CREATE INDEX IF NOT EXISTS idx_automation_incidents_status ON public.automation_incidents USING btree (status);
CREATE INDEX IF NOT EXISTS idx_automation_launch_events_automation_id ON public.automation_launch_events USING btree (automation_id);
CREATE INDEX IF NOT EXISTS idx_automation_launch_events_completed_at ON public.automation_launch_events USING btree (completed_at);
CREATE INDEX IF NOT EXISTS idx_automation_launch_events_launch_type ON public.automation_launch_events USING btree (launch_type);
CREATE INDEX IF NOT EXISTS idx_automation_launch_events_owner_identity ON public.automation_launch_events USING btree (owner_identity);
CREATE INDEX IF NOT EXISTS idx_automation_launch_events_runtime_task_id ON public.automation_launch_events USING btree (runtime_task_id);
CREATE INDEX IF NOT EXISTS idx_automation_launch_events_runtime_type ON public.automation_launch_events USING btree (runtime_type);
CREATE INDEX IF NOT EXISTS idx_automation_launch_events_started_at ON public.automation_launch_events USING btree (started_at);
CREATE INDEX IF NOT EXISTS idx_automation_launch_events_status ON public.automation_launch_events USING btree (status);
CREATE INDEX IF NOT EXISTS idx_automation_route_checks_automation_id ON public.automation_route_checks USING btree (automation_id);
CREATE INDEX IF NOT EXISTS idx_automation_slos_automation_id ON public.automation_slos USING btree (automation_id);
CREATE INDEX IF NOT EXISTS idx_autonomy_action_traces_action_type ON public.autonomy_action_traces USING btree (action_type);
CREATE INDEX IF NOT EXISTS idx_autonomy_action_traces_approval_recorded ON public.autonomy_action_traces USING btree (approval_recorded);
CREATE INDEX IF NOT EXISTS idx_autonomy_action_traces_attempt ON public.autonomy_action_traces USING btree (attempt);
CREATE INDEX IF NOT EXISTS idx_autonomy_action_traces_completed_at ON public.autonomy_action_traces USING btree (completed_at);
CREATE INDEX IF NOT EXISTS idx_autonomy_action_traces_execution_verified ON public.autonomy_action_traces USING btree (execution_verified);
CREATE INDEX IF NOT EXISTS idx_autonomy_action_traces_external_side_effects ON public.autonomy_action_traces USING btree (external_side_effects);
CREATE INDEX IF NOT EXISTS idx_autonomy_action_traces_interface_type ON public.autonomy_action_traces USING btree (interface_type);
CREATE INDEX IF NOT EXISTS idx_autonomy_action_traces_policy_decision ON public.autonomy_action_traces USING btree (policy_decision);
CREATE INDEX IF NOT EXISTS idx_autonomy_action_traces_requires_approval ON public.autonomy_action_traces USING btree (requires_approval);
CREATE INDEX IF NOT EXISTS idx_autonomy_action_traces_started_at ON public.autonomy_action_traces USING btree (started_at);
CREATE INDEX IF NOT EXISTS idx_autonomy_action_traces_status ON public.autonomy_action_traces USING btree (status);
CREATE INDEX IF NOT EXISTS idx_autonomy_action_traces_verification_status ON public.autonomy_action_traces USING btree (verification_status);
CREATE INDEX IF NOT EXISTS idx_autonomy_action_traces_workflow_id ON public.autonomy_action_traces USING btree (workflow_id);
CREATE INDEX IF NOT EXISTS idx_autonomy_action_traces_world_state_id ON public.autonomy_action_traces USING btree (world_state_id);
CREATE INDEX IF NOT EXISTS idx_autonomy_evaluations_action_trace_id ON public.autonomy_evaluations USING btree (action_trace_id);
CREATE INDEX IF NOT EXISTS idx_autonomy_evaluations_attempt ON public.autonomy_evaluations USING btree (attempt);
CREATE INDEX IF NOT EXISTS idx_autonomy_evaluations_completion_under_policy ON public.autonomy_evaluations USING btree (completion_under_policy);
CREATE INDEX IF NOT EXISTS idx_autonomy_evaluations_created_at ON public.autonomy_evaluations USING btree (created_at);
CREATE INDEX IF NOT EXISTS idx_autonomy_evaluations_execution_based_correctness ON public.autonomy_evaluations USING btree (execution_based_correctness);
CREATE INDEX IF NOT EXISTS idx_autonomy_evaluations_failure_mode ON public.autonomy_evaluations USING btree (failure_mode);
CREATE INDEX IF NOT EXISTS idx_autonomy_evaluations_human_intervention ON public.autonomy_evaluations USING btree (human_intervention);
CREATE INDEX IF NOT EXISTS idx_autonomy_evaluations_invalid_action ON public.autonomy_evaluations USING btree (invalid_action);
CREATE INDEX IF NOT EXISTS idx_autonomy_evaluations_partial_completion ON public.autonomy_evaluations USING btree (partial_completion);
CREATE INDEX IF NOT EXISTS idx_autonomy_evaluations_policy_compliant ON public.autonomy_evaluations USING btree (policy_compliant);
CREATE INDEX IF NOT EXISTS idx_autonomy_evaluations_raw_completion ON public.autonomy_evaluations USING btree (raw_completion);
CREATE INDEX IF NOT EXISTS idx_autonomy_evaluations_recovered ON public.autonomy_evaluations USING btree (recovered);
CREATE INDEX IF NOT EXISTS idx_autonomy_evaluations_recovery_attempt ON public.autonomy_evaluations USING btree (recovery_attempt);
CREATE INDEX IF NOT EXISTS idx_autonomy_evaluations_risk_violation ON public.autonomy_evaluations USING btree (risk_violation);
CREATE INDEX IF NOT EXISTS idx_autonomy_evaluations_workflow_id ON public.autonomy_evaluations USING btree (workflow_id);
CREATE INDEX IF NOT EXISTS idx_autonomy_stress_runs_created_at ON public.autonomy_stress_runs USING btree (created_at);
CREATE INDEX IF NOT EXISTS idx_autonomy_world_states_attempt ON public.autonomy_world_states USING btree (attempt);
CREATE INDEX IF NOT EXISTS idx_autonomy_world_states_observation_type ON public.autonomy_world_states USING btree (observation_type);
CREATE INDEX IF NOT EXISTS idx_autonomy_world_states_observed_at ON public.autonomy_world_states USING btree (observed_at);
CREATE INDEX IF NOT EXISTS idx_autonomy_world_states_partial ON public.autonomy_world_states USING btree (partial);
CREATE INDEX IF NOT EXISTS idx_autonomy_world_states_requires_reobserve ON public.autonomy_world_states USING btree (requires_reobserve);
CREATE INDEX IF NOT EXISTS idx_autonomy_world_states_source_revision ON public.autonomy_world_states USING btree (source_revision);
CREATE INDEX IF NOT EXISTS idx_autonomy_world_states_stale_after ON public.autonomy_world_states USING btree (stale_after);
CREATE INDEX IF NOT EXISTS idx_autonomy_world_states_state ON public.autonomy_world_states USING btree (state);
CREATE INDEX IF NOT EXISTS idx_autonomy_world_states_workflow_id ON public.autonomy_world_states USING btree (workflow_id);
CREATE INDEX IF NOT EXISTS idx_connected_sources_category ON public.connected_sources USING btree (category);
CREATE INDEX IF NOT EXISTS idx_connected_sources_connector_key ON public.connected_sources USING btree (connector_key);
CREATE INDEX IF NOT EXISTS idx_connected_sources_default_project_key ON public.connected_sources USING btree (default_project_key);
CREATE INDEX IF NOT EXISTS idx_connected_sources_enabled ON public.connected_sources USING btree (enabled);
CREATE INDEX IF NOT EXISTS idx_connected_sources_local_only ON public.connected_sources USING btree (local_only);
CREATE INDEX IF NOT EXISTS idx_connected_sources_owner_identity ON public.connected_sources USING btree (owner_identity);
CREATE INDEX IF NOT EXISTS idx_connected_sources_status ON public.connected_sources USING btree (status);
CREATE INDEX IF NOT EXISTS idx_context_memories_archived ON public.context_memories USING btree (archived);
CREATE INDEX IF NOT EXISTS idx_context_memories_content_hash ON public.context_memories USING btree (content_hash);
CREATE INDEX IF NOT EXISTS idx_context_memories_kind ON public.context_memories USING btree (kind);
CREATE INDEX IF NOT EXISTS idx_context_memories_owner_identity ON public.context_memories USING btree (owner_identity);
CREATE INDEX IF NOT EXISTS idx_context_memories_project_key ON public.context_memories USING btree (project_key);
CREATE INDEX IF NOT EXISTS idx_durable_jobs_claim ON public.durable_jobs USING btree (status, run_at);
CREATE INDEX IF NOT EXISTS idx_durable_jobs_kind ON public.durable_jobs USING btree (kind);
CREATE INDEX IF NOT EXISTS idx_durable_jobs_lease ON public.durable_jobs USING btree (status, locked_at);
CREATE INDEX IF NOT EXISTS idx_durable_jobs_queue ON public.durable_jobs USING btree (queue);
CREATE INDEX IF NOT EXISTS idx_durable_jobs_run_at ON public.durable_jobs USING btree (run_at);
CREATE INDEX IF NOT EXISTS idx_durable_jobs_status ON public.durable_jobs USING btree (status);
CREATE INDEX IF NOT EXISTS idx_llm_provider_probes_checked_at ON public.llm_provider_probes USING btree (checked_at);
CREATE INDEX IF NOT EXISTS idx_llm_provider_probes_last_successful_at ON public.llm_provider_probes USING btree (last_successful_at);
CREATE INDEX IF NOT EXISTS idx_llm_provider_probes_live ON public.llm_provider_probes USING btree (live);
CREATE INDEX IF NOT EXISTS idx_llm_provider_probes_provider_id ON public.llm_provider_probes USING btree (provider_id);
CREATE INDEX IF NOT EXISTS idx_llm_provider_probes_requires_review ON public.llm_provider_probes USING btree (requires_review);
CREATE INDEX IF NOT EXISTS idx_llm_provider_probes_status ON public.llm_provider_probes USING btree (status);
CREATE INDEX IF NOT EXISTS idx_model_run_telemetries_created_at ON public.model_run_telemetries USING btree (created_at);
CREATE INDEX IF NOT EXISTS idx_model_run_telemetries_lane ON public.model_run_telemetries USING btree (lane);
CREATE INDEX IF NOT EXISTS idx_model_run_telemetries_model_id ON public.model_run_telemetries USING btree (model_id);
CREATE INDEX IF NOT EXISTS idx_model_run_telemetries_operation_id ON public.model_run_telemetries USING btree (operation_id);
CREATE INDEX IF NOT EXISTS idx_model_run_telemetries_provider_id ON public.model_run_telemetries USING btree (provider_id);
CREATE INDEX IF NOT EXISTS idx_operation_events_operation_id ON public.operation_events USING btree (operation_id);
CREATE INDEX IF NOT EXISTS idx_operations_account_feed_id ON public.operations USING btree (account_feed_id);
CREATE INDEX IF NOT EXISTS idx_operations_dedupe_key ON public.operations USING btree (dedupe_key);
CREATE INDEX IF NOT EXISTS idx_operations_next_review_at ON public.operations USING btree (next_review_at);
CREATE INDEX IF NOT EXISTS idx_operations_owner_workspace_status ON public.operations USING btree (owner_user_id, workspace_id, status);
CREATE INDEX IF NOT EXISTS idx_operations_project_key ON public.operations USING btree (project_key);
CREATE INDEX IF NOT EXISTS idx_operations_pursuit_id ON public.operations USING btree (pursuit_id);
CREATE INDEX IF NOT EXISTS idx_operations_requires_approval ON public.operations USING btree (requires_approval);
CREATE INDEX IF NOT EXISTS idx_operations_risk_level ON public.operations USING btree (risk_level);
CREATE INDEX IF NOT EXISTS idx_operations_source ON public.operations USING btree (source_type, source_id);
CREATE INDEX IF NOT EXISTS idx_operations_workflow_id ON public.operations USING btree (workflow_id);
CREATE INDEX IF NOT EXISTS idx_pursuit_activities_event_type ON public.pursuit_activities USING btree (event_type);
CREATE INDEX IF NOT EXISTS idx_pursuit_activities_pursuit_id ON public.pursuit_activities USING btree (pursuit_id);
CREATE INDEX IF NOT EXISTS idx_pursuit_activities_source_id ON public.pursuit_activities USING btree (source_id);
CREATE INDEX IF NOT EXISTS idx_pursuit_activities_source_type ON public.pursuit_activities USING btree (source_type);
CREATE INDEX IF NOT EXISTS idx_pursuit_links_link_id ON public.pursuit_links USING btree (link_id);
CREATE INDEX IF NOT EXISTS idx_pursuit_links_link_type ON public.pursuit_links USING btree (link_type);
CREATE INDEX IF NOT EXISTS idx_pursuit_links_pursuit_id ON public.pursuit_links USING btree (pursuit_id);
CREATE INDEX IF NOT EXISTS idx_pursuit_links_relationship ON public.pursuit_links USING btree (relationship);
CREATE INDEX IF NOT EXISTS idx_pursuit_links_source_uri ON public.pursuit_links USING btree (source_uri);
CREATE INDEX IF NOT EXISTS idx_pursuit_task_attempts_automation_id ON public.pursuit_task_attempts USING btree (automation_id);
CREATE INDEX IF NOT EXISTS idx_pursuit_task_attempts_completed_at ON public.pursuit_task_attempts USING btree (completed_at);
CREATE INDEX IF NOT EXISTS idx_pursuit_task_attempts_launch_event_id ON public.pursuit_task_attempts USING btree (launch_event_id);
CREATE INDEX IF NOT EXISTS idx_pursuit_task_attempts_mode ON public.pursuit_task_attempts USING btree (mode);
CREATE INDEX IF NOT EXISTS idx_pursuit_task_attempts_owner_identity ON public.pursuit_task_attempts USING btree (owner_identity);
CREATE INDEX IF NOT EXISTS idx_pursuit_task_attempts_project_key ON public.pursuit_task_attempts USING btree (project_key);
CREATE INDEX IF NOT EXISTS idx_pursuit_task_attempts_pursuit_id ON public.pursuit_task_attempts USING btree (pursuit_id);
CREATE INDEX IF NOT EXISTS idx_pursuit_task_attempts_risk_level ON public.pursuit_task_attempts USING btree (risk_level);
CREATE INDEX IF NOT EXISTS idx_pursuit_task_attempts_started_at ON public.pursuit_task_attempts USING btree (started_at);
CREATE INDEX IF NOT EXISTS idx_pursuit_task_attempts_status ON public.pursuit_task_attempts USING btree (status);
CREATE UNIQUE INDEX IF NOT EXISTS idx_pursuit_task_attempts_task_plan_id ON public.pursuit_task_attempts USING btree (task_plan_id);
CREATE INDEX IF NOT EXISTS idx_pursuit_task_attempts_verification_status ON public.pursuit_task_attempts USING btree (verification_status);
CREATE INDEX IF NOT EXISTS idx_pursuits_archived ON public.pursuits USING btree (archived);
CREATE INDEX IF NOT EXISTS idx_pursuits_autonomy_level ON public.pursuits USING btree (autonomy_level);
CREATE INDEX IF NOT EXISTS idx_pursuits_completion_state ON public.pursuits USING btree (completion_state);
CREATE INDEX IF NOT EXISTS idx_pursuits_domain ON public.pursuits USING btree (domain);
CREATE INDEX IF NOT EXISTS idx_pursuits_last_activity_at ON public.pursuits USING btree (last_activity_at);
CREATE INDEX IF NOT EXISTS idx_pursuits_need_category ON public.pursuits USING btree (need_category);
CREATE INDEX IF NOT EXISTS idx_pursuits_next_review_at ON public.pursuits USING btree (next_review_at);
CREATE INDEX IF NOT EXISTS idx_pursuits_owner_identity ON public.pursuits USING btree (owner_identity);
CREATE INDEX IF NOT EXISTS idx_pursuits_priority_score ON public.pursuits USING btree (priority_score);
CREATE INDEX IF NOT EXISTS idx_pursuits_project_key ON public.pursuits USING btree (project_key);
CREATE INDEX IF NOT EXISTS idx_pursuits_risk_level ON public.pursuits USING btree (risk_level);
CREATE INDEX IF NOT EXISTS idx_pursuits_source_of_creation ON public.pursuits USING btree (source_of_creation);
CREATE INDEX IF NOT EXISTS idx_pursuits_status ON public.pursuits USING btree (status);
CREATE INDEX IF NOT EXISTS idx_pursuits_title ON public.pursuits USING btree (title);
CREATE INDEX IF NOT EXISTS idx_source_audit_logs_action ON public.source_audit_logs USING btree (action);
CREATE INDEX IF NOT EXISTS idx_source_audit_logs_source_id ON public.source_audit_logs USING btree (source_id);
CREATE INDEX IF NOT EXISTS idx_source_connectors_adapter_status ON public.source_connectors USING btree (adapter_status);
CREATE INDEX IF NOT EXISTS idx_source_connectors_category ON public.source_connectors USING btree (category);
CREATE UNIQUE INDEX IF NOT EXISTS idx_source_connectors_connector_key ON public.source_connectors USING btree (connector_key);
CREATE INDEX IF NOT EXISTS idx_source_connectors_enabled ON public.source_connectors USING btree (enabled);
CREATE INDEX IF NOT EXISTS idx_source_extractions_archived ON public.source_extractions USING btree (archived);
CREATE INDEX IF NOT EXISTS idx_source_extractions_content_hash ON public.source_extractions USING btree (content_hash);
CREATE INDEX IF NOT EXISTS idx_source_extractions_content_type ON public.source_extractions USING btree (content_type);
CREATE INDEX IF NOT EXISTS idx_source_extractions_project_key ON public.source_extractions USING btree (project_key);
CREATE INDEX IF NOT EXISTS idx_source_extractions_raw_item_id ON public.source_extractions USING btree (raw_item_id);
CREATE INDEX IF NOT EXISTS idx_source_extractions_sensitive ON public.source_extractions USING btree (sensitive);
CREATE INDEX IF NOT EXISTS idx_source_extractions_source_id ON public.source_extractions USING btree (source_id);
CREATE INDEX IF NOT EXISTS idx_source_extractions_uncertain ON public.source_extractions USING btree (uncertain);
CREATE INDEX IF NOT EXISTS idx_source_index_entries_extraction_id ON public.source_index_entries USING btree (extraction_id);
CREATE INDEX IF NOT EXISTS idx_source_index_entries_index_type ON public.source_index_entries USING btree (index_type);
CREATE INDEX IF NOT EXISTS idx_source_index_entries_project_key ON public.source_index_entries USING btree (project_key);
CREATE INDEX IF NOT EXISTS idx_source_index_entries_source_id ON public.source_index_entries USING btree (source_id);
CREATE INDEX IF NOT EXISTS idx_source_o_auth_tokens_provider ON public.source_o_auth_tokens USING btree (provider);
CREATE UNIQUE INDEX IF NOT EXISTS idx_source_o_auth_tokens_source_id ON public.source_o_auth_tokens USING btree (source_id);
CREATE INDEX IF NOT EXISTS idx_source_raw_items_content_hash ON public.source_raw_items USING btree (content_hash);
CREATE INDEX IF NOT EXISTS idx_source_raw_items_external_id ON public.source_raw_items USING btree (external_id);
CREATE INDEX IF NOT EXISTS idx_source_raw_items_item_type ON public.source_raw_items USING btree (item_type);
CREATE INDEX IF NOT EXISTS idx_source_raw_items_project_key ON public.source_raw_items USING btree (project_key);
CREATE INDEX IF NOT EXISTS idx_source_raw_items_source_id ON public.source_raw_items USING btree (source_id);
CREATE INDEX IF NOT EXISTS idx_source_sync_jobs_mode ON public.source_sync_jobs USING btree (mode);
CREATE INDEX IF NOT EXISTS idx_source_sync_jobs_source_id ON public.source_sync_jobs USING btree (source_id);
CREATE INDEX IF NOT EXISTS idx_source_sync_jobs_status ON public.source_sync_jobs USING btree (status);
CREATE INDEX IF NOT EXISTS idx_verification_audit_logs_action ON public.verification_audit_logs USING btree (action);
CREATE INDEX IF NOT EXISTS idx_verification_audit_logs_run_id ON public.verification_audit_logs USING btree (run_id);
CREATE INDEX IF NOT EXISTS idx_verification_claims_high_risk ON public.verification_claims USING btree (high_risk);
CREATE INDEX IF NOT EXISTS idx_verification_claims_needs_review ON public.verification_claims USING btree (needs_review);
CREATE INDEX IF NOT EXISTS idx_verification_claims_run_id ON public.verification_claims USING btree (run_id);
CREATE INDEX IF NOT EXISTS idx_verification_claims_status ON public.verification_claims USING btree (status);
CREATE INDEX IF NOT EXISTS idx_verification_evidences_rejected ON public.verification_evidences USING btree (rejected);
CREATE INDEX IF NOT EXISTS idx_verification_evidences_run_id ON public.verification_evidences USING btree (run_id);
CREATE INDEX IF NOT EXISTS idx_verification_evidences_source_id ON public.verification_evidences USING btree (source_id);
CREATE INDEX IF NOT EXISTS idx_verification_evidences_source_type ON public.verification_evidences USING btree (source_type);
CREATE INDEX IF NOT EXISTS idx_verification_evidences_used ON public.verification_evidences USING btree (used);
CREATE INDEX IF NOT EXISTS idx_verification_runs_mode ON public.verification_runs USING btree (mode);
CREATE INDEX IF NOT EXISTS idx_verification_runs_owner_identity ON public.verification_runs USING btree (owner_identity);
CREATE INDEX IF NOT EXISTS idx_verification_runs_project_key ON public.verification_runs USING btree (project_key);
CREATE INDEX IF NOT EXISTS idx_verification_runs_status ON public.verification_runs USING btree (status);
CREATE INDEX IF NOT EXISTS idx_workflow_checklist_items_due_at ON public.workflow_checklist_items USING btree (due_at);
CREATE INDEX IF NOT EXISTS idx_workflow_checklist_items_position ON public.workflow_checklist_items USING btree ("position");
CREATE INDEX IF NOT EXISTS idx_workflow_checklist_items_reminder_at ON public.workflow_checklist_items USING btree (reminder_at);
CREATE INDEX IF NOT EXISTS idx_workflow_checklist_items_requires_approval ON public.workflow_checklist_items USING btree (requires_approval);
CREATE INDEX IF NOT EXISTS idx_workflow_checklist_items_status ON public.workflow_checklist_items USING btree (status);
CREATE INDEX IF NOT EXISTS idx_workflow_checklist_items_workflow_id ON public.workflow_checklist_items USING btree (workflow_id);
CREATE INDEX IF NOT EXISTS idx_workflow_decisions_approved ON public.workflow_decisions USING btree (approved);
CREATE INDEX IF NOT EXISTS idx_workflow_decisions_decision ON public.workflow_decisions USING btree (decision);
CREATE INDEX IF NOT EXISTS idx_workflow_decisions_decision_type ON public.workflow_decisions USING btree (decision_type);
CREATE INDEX IF NOT EXISTS idx_workflow_decisions_workflow_id ON public.workflow_decisions USING btree (workflow_id);
CREATE INDEX IF NOT EXISTS idx_workflow_events_event_type ON public.workflow_events USING btree (event_type);
CREATE INDEX IF NOT EXISTS idx_workflow_events_from_state ON public.workflow_events USING btree (from_state);
CREATE INDEX IF NOT EXISTS idx_workflow_events_to_state ON public.workflow_events USING btree (to_state);
CREATE INDEX IF NOT EXISTS idx_workflow_events_workflow_id ON public.workflow_events USING btree (workflow_id);
CREATE INDEX IF NOT EXISTS idx_workflow_evidence_claims_needs_review ON public.workflow_evidence_claims USING btree (needs_review);
CREATE INDEX IF NOT EXISTS idx_workflow_evidence_claims_reliability ON public.workflow_evidence_claims USING btree (reliability);
CREATE INDEX IF NOT EXISTS idx_workflow_evidence_claims_source_uri ON public.workflow_evidence_claims USING btree (source_uri);
CREATE INDEX IF NOT EXISTS idx_workflow_evidence_claims_status ON public.workflow_evidence_claims USING btree (status);
CREATE INDEX IF NOT EXISTS idx_workflow_evidence_claims_workflow_id ON public.workflow_evidence_claims USING btree (workflow_id);
CREATE INDEX IF NOT EXISTS idx_workflow_intake_records_content_type ON public.workflow_intake_records USING btree (content_type);
CREATE INDEX IF NOT EXISTS idx_workflow_intake_records_possible_project ON public.workflow_intake_records USING btree (possible_project);
CREATE INDEX IF NOT EXISTS idx_workflow_intake_records_received_at ON public.workflow_intake_records USING btree (received_at);
CREATE INDEX IF NOT EXISTS idx_workflow_intake_records_sender ON public.workflow_intake_records USING btree (sender);
CREATE INDEX IF NOT EXISTS idx_workflow_intake_records_source_id ON public.workflow_intake_records USING btree (source_id);
CREATE INDEX IF NOT EXISTS idx_workflow_intake_records_source_type ON public.workflow_intake_records USING btree (source_type);
CREATE INDEX IF NOT EXISTS idx_workflow_intake_records_source_uri ON public.workflow_intake_records USING btree (source_uri);
CREATE INDEX IF NOT EXISTS idx_workflow_intake_records_urgency ON public.workflow_intake_records USING btree (urgency);
CREATE INDEX IF NOT EXISTS idx_workflow_intake_records_workflow_id ON public.workflow_intake_records USING btree (workflow_id);
CREATE INDEX IF NOT EXISTS idx_workflow_items_approval_status ON public.workflow_items USING btree (approval_status);
CREATE INDEX IF NOT EXISTS idx_workflow_items_archived ON public.workflow_items USING btree (archived);
CREATE INDEX IF NOT EXISTS idx_workflow_items_automation_id ON public.workflow_items USING btree (automation_id);
CREATE INDEX IF NOT EXISTS idx_workflow_items_autonomy_level ON public.workflow_items USING btree (autonomy_level);
CREATE INDEX IF NOT EXISTS idx_workflow_items_completed_at ON public.workflow_items USING btree (completed_at);
CREATE INDEX IF NOT EXISTS idx_workflow_items_current_state ON public.workflow_items USING btree (current_state);
CREATE INDEX IF NOT EXISTS idx_workflow_items_due_at ON public.workflow_items USING btree (due_at);
CREATE INDEX IF NOT EXISTS idx_workflow_items_next_run_at ON public.workflow_items USING btree (next_run_at);
CREATE INDEX IF NOT EXISTS idx_workflow_items_owner_identity ON public.workflow_items USING btree (owner_identity);
CREATE INDEX IF NOT EXISTS idx_workflow_items_priority_score ON public.workflow_items USING btree (priority_score);
CREATE INDEX IF NOT EXISTS idx_workflow_items_project_key ON public.workflow_items USING btree (project_key);
CREATE INDEX IF NOT EXISTS idx_workflow_items_recovery_status ON public.workflow_items USING btree (recovery_status);
CREATE INDEX IF NOT EXISTS idx_workflow_items_requires_approval ON public.workflow_items USING btree (requires_approval);
CREATE INDEX IF NOT EXISTS idx_workflow_items_retry_count ON public.workflow_items USING btree (retry_count);
CREATE INDEX IF NOT EXISTS idx_workflow_items_risk_level ON public.workflow_items USING btree (risk_level);
CREATE INDEX IF NOT EXISTS idx_workflow_items_source_id ON public.workflow_items USING btree (source_id);
CREATE INDEX IF NOT EXISTS idx_workflow_items_source_revision ON public.workflow_items USING btree (source_revision);
CREATE INDEX IF NOT EXISTS idx_workflow_items_source_type ON public.workflow_items USING btree (source_type);
CREATE INDEX IF NOT EXISTS idx_workflow_items_task_type ON public.workflow_items USING btree (task_type);
CREATE INDEX IF NOT EXISTS idx_workflow_items_title ON public.workflow_items USING btree (title);
CREATE INDEX IF NOT EXISTS idx_workflow_items_verification_status ON public.workflow_items USING btree (verification_status);
CREATE INDEX IF NOT EXISTS idx_workflow_items_worker_claim_id ON public.workflow_items USING btree (worker_claim_id);
CREATE INDEX IF NOT EXISTS idx_workflow_items_worker_lease_until ON public.workflow_items USING btree (worker_lease_until);
CREATE INDEX IF NOT EXISTS idx_workflow_open_loops_claim_id ON public.workflow_open_loops USING btree (claim_id);
CREATE INDEX IF NOT EXISTS idx_workflow_open_loops_follow_up_at ON public.workflow_open_loops USING btree (follow_up_at);
CREATE INDEX IF NOT EXISTS idx_workflow_open_loops_lease_until ON public.workflow_open_loops USING btree (lease_until);
CREATE INDEX IF NOT EXISTS idx_workflow_open_loops_responsible_party ON public.workflow_open_loops USING btree (responsible_party);
CREATE INDEX IF NOT EXISTS idx_workflow_open_loops_status ON public.workflow_open_loops USING btree (status);
CREATE INDEX IF NOT EXISTS idx_workflow_open_loops_workflow_id ON public.workflow_open_loops USING btree (workflow_id);
CREATE INDEX IF NOT EXISTS idx_workflow_project_matches_confidence ON public.workflow_project_matches USING btree (confidence);
CREATE INDEX IF NOT EXISTS idx_workflow_project_matches_project_key ON public.workflow_project_matches USING btree (project_key);
CREATE INDEX IF NOT EXISTS idx_workflow_project_matches_workflow_id ON public.workflow_project_matches USING btree (workflow_id);
CREATE INDEX IF NOT EXISTS idx_workflow_proposals_resolved_at ON public.workflow_proposals USING btree (resolved_at);
CREATE INDEX IF NOT EXISTS idx_workflow_proposals_status ON public.workflow_proposals USING btree (status);
CREATE INDEX IF NOT EXISTS idx_workflow_proposals_workflow_id ON public.workflow_proposals USING btree (workflow_id);
CREATE INDEX IF NOT EXISTS idx_workflow_quality_gates_gate ON public.workflow_quality_gates USING btree (gate);
CREATE INDEX IF NOT EXISTS idx_workflow_quality_gates_status ON public.workflow_quality_gates USING btree (status);
CREATE INDEX IF NOT EXISTS idx_workflow_quality_gates_workflow_id ON public.workflow_quality_gates USING btree (workflow_id);
CREATE INDEX IF NOT EXISTS idx_workflow_rules_category ON public.workflow_rules USING btree (category);
CREATE INDEX IF NOT EXISTS idx_workflow_rules_enabled ON public.workflow_rules USING btree (enabled);
CREATE UNIQUE INDEX IF NOT EXISTS idx_workflow_rules_rule_key ON public.workflow_rules USING btree (rule_key);
CREATE INDEX IF NOT EXISTS idx_workflow_source_links_relationship ON public.workflow_source_links USING btree (relationship);
CREATE INDEX IF NOT EXISTS idx_workflow_source_links_source_id ON public.workflow_source_links USING btree (source_id);
CREATE INDEX IF NOT EXISTS idx_workflow_source_links_source_type ON public.workflow_source_links USING btree (source_type);
CREATE INDEX IF NOT EXISTS idx_workflow_source_links_source_uri ON public.workflow_source_links USING btree (source_uri);
CREATE INDEX IF NOT EXISTS idx_workflow_source_links_workflow_id ON public.workflow_source_links USING btree (workflow_id);
CREATE INDEX IF NOT EXISTS idx_workflow_transitions_approved ON public.workflow_transitions USING btree (approved);
CREATE INDEX IF NOT EXISTS idx_workflow_transitions_from_state ON public.workflow_transitions USING btree (from_state);
CREATE INDEX IF NOT EXISTS idx_workflow_transitions_to_state ON public.workflow_transitions USING btree (to_state);
CREATE INDEX IF NOT EXISTS idx_workflow_transitions_trigger ON public.workflow_transitions USING btree (trigger);
CREATE INDEX IF NOT EXISTS idx_workflow_transitions_workflow_id ON public.workflow_transitions USING btree (workflow_id);
CREATE UNIQUE INDEX IF NOT EXISTS pursuit_link_unique ON public.pursuit_links USING btree (pursuit_id, link_type, link_id, relationship);
