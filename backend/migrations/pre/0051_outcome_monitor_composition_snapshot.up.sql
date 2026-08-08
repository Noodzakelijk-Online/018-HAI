ALTER TABLE public.outcome_monitor_composition_deliveries
    ADD COLUMN snapshot_status character varying(24),
    ADD COLUMN composer_version character varying(120),
    ADD COLUMN snapshot_captured_at timestamp with time zone,
    ADD COLUMN outcome_revision bigint,
    ADD COLUMN outcome_audit_digest character(64),
    ADD COLUMN policy_idempotency_key text,
    ADD COLUMN policy_payload_digest character(64),
    ADD COLUMN policy_recorded_at timestamp with time zone,
    ADD COLUMN signal_watermark_at timestamp with time zone,
    ADD COLUMN signal_watermark_key text,
    ADD COLUMN decision_watermark_at timestamp with time zone,
    ADD COLUMN decision_watermark_key text,
    ADD COLUMN feedback_watermark_at timestamp with time zone,
    ADD COLUMN feedback_watermark_id uuid,
    ADD COLUMN feedback_watermark_digest character(64),
    ADD COLUMN attention_snapshot jsonb,
    ADD COLUMN attention_snapshot_digest character(64),
    ADD COLUMN snapshot_digest character(64);

ALTER TABLE public.outcome_monitor_composition_attempts
    ADD COLUMN snapshot_digest character(64);

CREATE OR REPLACE FUNCTION public.hai_snapshot_rfc3339_utc(
    p_value timestamp with time zone
)
RETURNS text
LANGUAGE sql
IMMUTABLE
PARALLEL SAFE
STRICT
AS $$
    SELECT to_char(p_value AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS')
        || CASE
            WHEN to_char(p_value AT TIME ZONE 'UTC', 'US') = '000000' THEN ''
            ELSE '.' || rtrim(to_char(p_value AT TIME ZONE 'UTC', 'US'), '0')
        END
        || 'Z'
$$;

CREATE OR REPLACE FUNCTION public.hai_outcome_monitor_attention_snapshot_json(
    p_snapshot jsonb
)
RETURNS text
LANGUAGE sql
IMMUTABLE
PARALLEL SAFE
STRICT
AS $$
    SELECT '{"contractVersion":' || (p_snapshot #>> '{contractVersion}')
        || ',"ownerIdentity":' || to_json(p_snapshot #>> '{ownerIdentity}')::text
        || ',"capturedAt":' || to_json(public.hai_snapshot_rfc3339_utc(
            (p_snapshot #>> '{capturedAt}')::timestamp with time zone))::text
        || ',"policy":{"idempotencyKey":'
        || to_json(p_snapshot #>> '{policy,idempotencyKey}')::text
        || ',"payloadDigest":' || to_json(p_snapshot #>> '{policy,payloadDigest}')::text
        || ',"recordedAt":' || to_json(public.hai_snapshot_rfc3339_utc(
            (p_snapshot #>> '{policy,recordedAt}')::timestamp with time zone))::text || '}'
        || ',"signals":{' || CASE
            WHEN p_snapshot #> '{signals,cursor}' IS NULL THEN ''
            ELSE '"cursor":{"recordedAt":' || to_json(public.hai_snapshot_rfc3339_utc(
                    (p_snapshot #>> '{signals,cursor,recordedAt}')::timestamp with time zone))::text
                || ',"idempotencyKey":' || to_json(p_snapshot #>> '{signals,cursor,idempotencyKey}')::text
                || ',"ordinal":' || (p_snapshot #>> '{signals,cursor,ordinal}')
                || ',"payloadDigest":' || to_json(p_snapshot #>> '{signals,cursor,payloadDigest}')::text
                || '},'
        END
        || '"count":' || (p_snapshot #>> '{signals,count}')
        || ',"windowDigest":' || to_json(p_snapshot #>> '{signals,windowDigest}')::text || '}'
        || ',"decisions":{' || CASE
            WHEN p_snapshot #> '{decisions,cursor}' IS NULL THEN ''
            ELSE '"cursor":{"recordedAt":' || to_json(public.hai_snapshot_rfc3339_utc(
                    (p_snapshot #>> '{decisions,cursor,recordedAt}')::timestamp with time zone))::text
                || ',"idempotencyKey":' || to_json(p_snapshot #>> '{decisions,cursor,idempotencyKey}')::text
                || ',"ordinal":' || (p_snapshot #>> '{decisions,cursor,ordinal}')
                || ',"payloadDigest":' || to_json(p_snapshot #>> '{decisions,cursor,payloadDigest}')::text
                || '},'
        END
        || '"count":' || (p_snapshot #>> '{decisions,count}')
        || ',"windowDigest":' || to_json(p_snapshot #>> '{decisions,windowDigest}')::text || '}'
        || ',"feedback":{' || CASE
            WHEN p_snapshot #> '{feedback,cursor}' IS NULL THEN ''
            ELSE '"cursor":{"recordedAt":' || to_json(public.hai_snapshot_rfc3339_utc(
                    (p_snapshot #>> '{feedback,cursor,recordedAt}')::timestamp with time zone))::text
                || ',"feedbackId":' || to_json(p_snapshot #>> '{feedback,cursor,feedbackId}')::text
                || ',"idempotencyKey":' || to_json(p_snapshot #>> '{feedback,cursor,idempotencyKey}')::text
                || ',"payloadDigest":' || to_json(p_snapshot #>> '{feedback,cursor,payloadDigest}')::text
                || ',"recordDigest":' || to_json(p_snapshot #>> '{feedback,cursor,recordDigest}')::text
                || '},'
        END
        || '"count":' || (p_snapshot #>> '{feedback,count}')
        || ',"windowDigest":' || to_json(p_snapshot #>> '{feedback,windowDigest}')::text || '}'
        || ',"inputDigest":' || to_json(p_snapshot #>> '{inputDigest}')::text || '}'
$$;

CREATE OR REPLACE FUNCTION public.hai_outcome_monitor_composition_snapshot_digest(
    p_snapshot_status text,
    p_composer_version text,
    p_snapshot_captured_at timestamp with time zone,
    p_outcome_revision bigint,
    p_outcome_audit_digest text,
    p_attention_snapshot jsonb
)
RETURNS character(64)
LANGUAGE plpgsql
IMMUTABLE
PARALLEL SAFE
AS $$
DECLARE
    encoded text;
BEGIN
    IF p_snapshot_status = 'legacy_unpinned' THEN
        encoded := '{"operation":"composition_snapshot","value":'
            || '{"contractVersion":1,"status":"legacy_unpinned",'
            || '"composerVersion":"ambient-monitor-composer/pre-0051-unknown","capturedAt":'
            || to_json(public.hai_snapshot_rfc3339_utc(p_snapshot_captured_at))::text
            || ',"attention":{"contractVersion":0,"ownerIdentity":"",'
            || '"capturedAt":"0001-01-01T00:00:00Z",'
            || '"policy":{"idempotencyKey":"","payloadDigest":"",'
            || '"recordedAt":"0001-01-01T00:00:00Z"},'
            || '"signals":{"count":0,"windowDigest":""},'
            || '"decisions":{"count":0,"windowDigest":""},'
            || '"feedback":{"count":0,"windowDigest":""},"inputDigest":""},'
            || '"snapshotDigest":""}}';
    ELSIF p_snapshot_status = 'pinned' THEN
        encoded := '{"operation":"composition_snapshot","value":'
            || '{"contractVersion":1,"status":"pinned","composerVersion":'
            || to_json(p_composer_version)::text
            || ',"capturedAt":' || to_json(public.hai_snapshot_rfc3339_utc(
                p_snapshot_captured_at))::text
            || ',"outcomeRevision":' || p_outcome_revision::text
            || ',"outcomeAuditDigest":' || to_json(p_outcome_audit_digest)::text
            || ',"attention":' || public.hai_outcome_monitor_attention_snapshot_json(
                p_attention_snapshot)
            || ',"snapshotDigest":""}}';
    ELSE
        RAISE EXCEPTION 'unsupported composition snapshot status for digest'
            USING ERRCODE = 'integrity_constraint_violation';
    END IF;
    RETURN encode(digest(convert_to(encoded, 'UTF8'), 'sha256'), 'hex')::character(64);
END;
$$;

CREATE OR REPLACE FUNCTION public.hai_outcome_monitor_composition_binding_digest(
    p_owner_identity text,
    p_workspace_key text,
    p_delivery_id uuid,
    p_target_id uuid,
    p_run_id uuid,
    p_run_digest text,
    p_observation_id uuid,
    p_observation_digest text,
    p_snapshot_digest text
)
RETURNS character(64)
LANGUAGE sql
IMMUTABLE
PARALLEL SAFE
AS $$
    SELECT encode(digest(convert_to(
        '{"operation":"composition_binding","value":'
        || '{"Scope":{"ownerId":' || to_json(p_owner_identity)::text
        || ',"workspaceId":' || to_json(p_workspace_key)::text || '}'
        || ',"ID":' || to_json('cmp-' || replace(p_delivery_id::text, '-', ''))::text
        || ',"TargetID":' || to_json(p_target_id::text)::text
        || ',"RunID":' || to_json('run-' || replace(p_run_id::text, '-', ''))::text
        || ',"RunDigest":' || to_json(p_run_digest)::text
        || ',"ObservationID":' || to_json(CASE
            WHEN p_observation_id IS NULL THEN ''
            ELSE 'obs-' || replace(p_observation_id::text, '-', '')
        END)::text
        || ',"ObservationDigest":' || to_json(COALESCE(p_observation_digest, ''))::text
        || ',"SnapshotDigest":' || to_json(p_snapshot_digest)::text || '}}',
        'UTF8'), 'sha256'), 'hex')::character(64)
$$;

CREATE OR REPLACE FUNCTION public.hai_validate_outcome_monitor_attention_snapshot(
    p_owner_identity text,
    p_snapshot_captured_at timestamp with time zone,
    p_attention_snapshot jsonb
)
RETURNS boolean
LANGUAGE plpgsql
STABLE
AS $$
DECLARE
    policy_key text;
    policy_digest text;
    policy_at timestamp with time zone;
    signal_count integer;
    signal_cursor jsonb;
    signal_at timestamp with time zone;
    signal_key text;
    signal_ordinal integer;
    signal_payload_digest text;
    actual_signal_count integer;
    decision_count integer;
    decision_cursor jsonb;
    decision_at timestamp with time zone;
    decision_key text;
    decision_ordinal integer;
    decision_payload_digest text;
    actual_decision_count integer;
    feedback_count integer;
    feedback_cursor jsonb;
    feedback_at timestamp with time zone;
    feedback_cursor_id uuid;
    feedback_key text;
    feedback_payload_digest text;
    feedback_record_digest text;
    actual_feedback_count integer;
BEGIN
    IF jsonb_typeof(p_attention_snapshot) <> 'object'
       OR octet_length(p_attention_snapshot::text) NOT BETWEEN 2 AND 131072
       OR NOT (p_attention_snapshot ?& ARRAY[
           'contractVersion', 'ownerIdentity', 'capturedAt', 'policy',
           'signals', 'decisions', 'feedback', 'inputDigest'
       ])
       OR p_attention_snapshot #>> '{contractVersion}' <> '1'
       OR p_attention_snapshot #>> '{ownerIdentity}' <> p_owner_identity
       OR (p_attention_snapshot #>> '{capturedAt}')::timestamp with time zone
            IS DISTINCT FROM p_snapshot_captured_at
       OR p_attention_snapshot #>> '{inputDigest}' !~ '^[0-9a-f]{64}$' THEN
        RAISE EXCEPTION 'attention snapshot envelope is invalid or outside its bounded contract'
            USING ERRCODE = 'integrity_constraint_violation';
    END IF;

    IF jsonb_typeof(p_attention_snapshot -> 'policy') <> 'object'
       OR NOT ((p_attention_snapshot -> 'policy') ?& ARRAY[
           'idempotencyKey', 'payloadDigest', 'recordedAt'
       ]) THEN
        RAISE EXCEPTION 'attention snapshot policy reference is incomplete'
            USING ERRCODE = 'integrity_constraint_violation';
    END IF;
    policy_key := p_attention_snapshot #>> '{policy,idempotencyKey}';
    policy_digest := p_attention_snapshot #>> '{policy,payloadDigest}';
    policy_at := (p_attention_snapshot #>> '{policy,recordedAt}')::timestamp with time zone;
    IF char_length(policy_key) NOT BETWEEN 1 AND 200
       OR policy_digest !~ '^[0-9a-f]{64}$'
       OR policy_at > p_snapshot_captured_at
       OR NOT EXISTS (
           SELECT 1 FROM public.proactivity_policy_records
            WHERE owner_identity = p_owner_identity
              AND idempotency_key = policy_key
              AND payload_digest = policy_digest
              AND recorded_at = policy_at
       ) THEN
        RAISE EXCEPTION 'attention snapshot policy does not resolve to exact immutable source material'
            USING ERRCODE = 'foreign_key_violation';
    END IF;

    IF jsonb_typeof(p_attention_snapshot -> 'signals') <> 'object'
       OR NOT ((p_attention_snapshot -> 'signals') ?& ARRAY['count', 'windowDigest'])
       OR p_attention_snapshot #>> '{signals,count}' !~ '^[0-9]+$'
       OR p_attention_snapshot #>> '{signals,windowDigest}' !~ '^[0-9a-f]{64}$' THEN
        RAISE EXCEPTION 'attention snapshot signal watermark is invalid'
            USING ERRCODE = 'integrity_constraint_violation';
    END IF;
    signal_count := (p_attention_snapshot #>> '{signals,count}')::integer;
    IF signal_count NOT BETWEEN 0 AND 512 THEN
        RAISE EXCEPTION 'attention snapshot signal count exceeds its bounded window'
            USING ERRCODE = 'integrity_constraint_violation';
    END IF;
    SELECT count(*) INTO actual_signal_count
      FROM (
          SELECT 1 FROM public.proactivity_signal_records
           WHERE owner_identity = p_owner_identity
             AND recorded_at <= p_snapshot_captured_at
           ORDER BY recorded_at DESC, batch_idempotency_key DESC, ordinal DESC
           LIMIT 512
      ) AS bounded_signals;
    signal_cursor := p_attention_snapshot #> '{signals,cursor}';
    IF signal_count <> actual_signal_count
       OR (signal_count = 0 AND signal_cursor IS NOT NULL)
       OR (signal_count > 0 AND jsonb_typeof(signal_cursor) <> 'object') THEN
        RAISE EXCEPTION 'attention snapshot signal count or cursor is not exact'
            USING ERRCODE = 'integrity_constraint_violation';
    END IF;
    IF signal_count > 0 THEN
        IF NOT (signal_cursor ?& ARRAY['recordedAt', 'idempotencyKey', 'ordinal', 'payloadDigest'])
           OR signal_cursor #>> '{ordinal}' !~ '^[0-9]+$' THEN
            RAISE EXCEPTION 'attention snapshot signal cursor is incomplete'
                USING ERRCODE = 'integrity_constraint_violation';
        END IF;
        signal_at := (signal_cursor #>> '{recordedAt}')::timestamp with time zone;
        signal_key := signal_cursor #>> '{idempotencyKey}';
        signal_ordinal := (signal_cursor #>> '{ordinal}')::integer;
        signal_payload_digest := signal_cursor #>> '{payloadDigest}';
        IF signal_at > p_snapshot_captured_at
           OR signal_ordinal NOT BETWEEN 0 AND 255
           OR signal_payload_digest !~ '^[0-9a-f]{64}$'
           OR NOT EXISTS (
               SELECT 1
                 FROM public.proactivity_signal_records AS record
                 JOIN public.proactivity_signal_batches AS batch
                   ON batch.owner_identity = record.owner_identity
                  AND batch.idempotency_key = record.batch_idempotency_key
                WHERE record.owner_identity = p_owner_identity
                  AND record.recorded_at = signal_at
                  AND record.batch_idempotency_key = signal_key
                  AND record.ordinal = signal_ordinal
                  AND batch.payload_digest = signal_payload_digest
           )
           OR (SELECT jsonb_build_array(record.batch_idempotency_key, record.ordinal)::text
                 FROM public.proactivity_signal_records AS record
                WHERE record.owner_identity = p_owner_identity
                  AND record.recorded_at <= p_snapshot_captured_at
                ORDER BY record.recorded_at DESC, record.batch_idempotency_key DESC, record.ordinal DESC
                LIMIT 1) IS DISTINCT FROM jsonb_build_array(signal_key, signal_ordinal)::text THEN
            RAISE EXCEPTION 'attention snapshot signal cursor does not identify the exact bounded history head'
                USING ERRCODE = 'foreign_key_violation';
        END IF;
    END IF;

    IF jsonb_typeof(p_attention_snapshot -> 'decisions') <> 'object'
       OR NOT ((p_attention_snapshot -> 'decisions') ?& ARRAY['count', 'windowDigest'])
       OR p_attention_snapshot #>> '{decisions,count}' !~ '^[0-9]+$'
       OR p_attention_snapshot #>> '{decisions,windowDigest}' !~ '^[0-9a-f]{64}$' THEN
        RAISE EXCEPTION 'attention snapshot decision watermark is invalid'
            USING ERRCODE = 'integrity_constraint_violation';
    END IF;
    decision_count := (p_attention_snapshot #>> '{decisions,count}')::integer;
    IF decision_count NOT BETWEEN 0 AND 2048 THEN
        RAISE EXCEPTION 'attention snapshot decision count exceeds its bounded window'
            USING ERRCODE = 'integrity_constraint_violation';
    END IF;
    SELECT count(*) INTO actual_decision_count
      FROM (
          SELECT 1 FROM public.proactivity_decision_records
           WHERE owner_identity = p_owner_identity
             AND recorded_at <= p_snapshot_captured_at
           ORDER BY recorded_at DESC, batch_idempotency_key DESC, ordinal DESC
           LIMIT 2048
      ) AS bounded_decisions;
    decision_cursor := p_attention_snapshot #> '{decisions,cursor}';
    IF decision_count <> actual_decision_count
       OR (decision_count = 0 AND decision_cursor IS NOT NULL)
       OR (decision_count > 0 AND jsonb_typeof(decision_cursor) <> 'object') THEN
        RAISE EXCEPTION 'attention snapshot decision count or cursor is not exact'
            USING ERRCODE = 'integrity_constraint_violation';
    END IF;
    IF decision_count > 0 THEN
        IF NOT (decision_cursor ?& ARRAY['recordedAt', 'idempotencyKey', 'ordinal', 'payloadDigest'])
           OR decision_cursor #>> '{ordinal}' !~ '^[0-9]+$' THEN
            RAISE EXCEPTION 'attention snapshot decision cursor is incomplete'
                USING ERRCODE = 'integrity_constraint_violation';
        END IF;
        decision_at := (decision_cursor #>> '{recordedAt}')::timestamp with time zone;
        decision_key := decision_cursor #>> '{idempotencyKey}';
        decision_ordinal := (decision_cursor #>> '{ordinal}')::integer;
        decision_payload_digest := decision_cursor #>> '{payloadDigest}';
        IF decision_at > p_snapshot_captured_at
           OR decision_ordinal NOT BETWEEN 0 AND 255
           OR decision_payload_digest !~ '^[0-9a-f]{64}$'
           OR NOT EXISTS (
               SELECT 1
                 FROM public.proactivity_decision_records AS record
                 JOIN public.proactivity_decision_batches AS batch
                   ON batch.owner_identity = record.owner_identity
                  AND batch.idempotency_key = record.batch_idempotency_key
                WHERE record.owner_identity = p_owner_identity
                  AND record.recorded_at = decision_at
                  AND record.batch_idempotency_key = decision_key
                  AND record.ordinal = decision_ordinal
                  AND batch.payload_digest = decision_payload_digest
           )
           OR (SELECT jsonb_build_array(record.batch_idempotency_key, record.ordinal)::text
                 FROM public.proactivity_decision_records AS record
                WHERE record.owner_identity = p_owner_identity
                  AND record.recorded_at <= p_snapshot_captured_at
                ORDER BY record.recorded_at DESC, record.batch_idempotency_key DESC, record.ordinal DESC
                LIMIT 1) IS DISTINCT FROM jsonb_build_array(decision_key, decision_ordinal)::text THEN
            RAISE EXCEPTION 'attention snapshot decision cursor does not identify the exact bounded history head'
                USING ERRCODE = 'foreign_key_violation';
        END IF;
    END IF;

    IF jsonb_typeof(p_attention_snapshot -> 'feedback') <> 'object'
       OR NOT ((p_attention_snapshot -> 'feedback') ?& ARRAY['count', 'windowDigest'])
       OR p_attention_snapshot #>> '{feedback,count}' !~ '^[0-9]+$'
       OR p_attention_snapshot #>> '{feedback,windowDigest}' !~ '^[0-9a-f]{64}$' THEN
        RAISE EXCEPTION 'attention snapshot feedback watermark is invalid'
            USING ERRCODE = 'integrity_constraint_violation';
    END IF;
    feedback_count := (p_attention_snapshot #>> '{feedback,count}')::integer;
    IF feedback_count NOT BETWEEN 0 AND 2048 THEN
        RAISE EXCEPTION 'attention snapshot feedback count exceeds its bounded window'
            USING ERRCODE = 'integrity_constraint_violation';
    END IF;
    SELECT count(*) INTO actual_feedback_count
      FROM (
          SELECT 1 FROM public.proactivity_feedback_records
           WHERE owner_identity = p_owner_identity
             AND recorded_at <= p_snapshot_captured_at
           ORDER BY recorded_at DESC, feedback_id DESC
           LIMIT 2048
      ) AS bounded_feedback;
    feedback_cursor := p_attention_snapshot #> '{feedback,cursor}';
    IF feedback_count <> actual_feedback_count
       OR (feedback_count = 0 AND feedback_cursor IS NOT NULL)
       OR (feedback_count > 0 AND jsonb_typeof(feedback_cursor) <> 'object') THEN
        RAISE EXCEPTION 'attention snapshot feedback count or cursor is not exact'
            USING ERRCODE = 'integrity_constraint_violation';
    END IF;
    IF feedback_count > 0 THEN
        IF NOT (feedback_cursor ?& ARRAY[
            'recordedAt', 'feedbackId', 'idempotencyKey', 'payloadDigest', 'recordDigest'
        ]) THEN
            RAISE EXCEPTION 'attention snapshot feedback cursor is incomplete'
                USING ERRCODE = 'integrity_constraint_violation';
        END IF;
        feedback_at := (feedback_cursor #>> '{recordedAt}')::timestamp with time zone;
        feedback_cursor_id := (feedback_cursor #>> '{feedbackId}')::uuid;
        feedback_key := feedback_cursor #>> '{idempotencyKey}';
        feedback_payload_digest := feedback_cursor #>> '{payloadDigest}';
        feedback_record_digest := feedback_cursor #>> '{recordDigest}';
        IF feedback_at > p_snapshot_captured_at
           OR feedback_payload_digest !~ '^[0-9a-f]{64}$'
           OR feedback_record_digest !~ '^[0-9a-f]{64}$'
           OR NOT EXISTS (
               SELECT 1 FROM public.proactivity_feedback_records AS source_feedback
                WHERE source_feedback.owner_identity = p_owner_identity
                  AND source_feedback.feedback_id = feedback_cursor_id
                  AND source_feedback.idempotency_key = feedback_key
                  AND source_feedback.request_digest = feedback_payload_digest
                  AND source_feedback.record_digest = feedback_record_digest
                  AND source_feedback.recorded_at = feedback_at
           )
           OR (SELECT record.feedback_id
                 FROM public.proactivity_feedback_records AS record
                WHERE record.owner_identity = p_owner_identity
                  AND record.recorded_at <= p_snapshot_captured_at
                ORDER BY record.recorded_at DESC, record.feedback_id DESC
                LIMIT 1) IS DISTINCT FROM feedback_cursor_id THEN
            RAISE EXCEPTION 'attention snapshot feedback cursor does not identify the exact bounded history head'
                USING ERRCODE = 'foreign_key_violation';
        END IF;
    END IF;
    RETURN true;
END;
$$;

CREATE OR REPLACE FUNCTION public.hai_pin_outcome_monitor_composition_snapshot()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    scoped_run record;
    exact_outcome record;
    attention_captured_at timestamp with time zone;
    attention_policy_key text;
    attention_policy_digest text;
    attention_policy_at timestamp with time zone;
    attention_signal_at timestamp with time zone;
    attention_signal_key text;
    attention_decision_at timestamp with time zone;
    attention_decision_key text;
    attention_feedback_at timestamp with time zone;
    attention_feedback_id uuid;
    attention_feedback_digest text;
    attention_input_digest text;
    expected_digest character(64);
BEGIN
    SELECT run.completed_at, target.outcome_key
      INTO scoped_run
      FROM public.outcome_monitor_runs AS run
      JOIN public.outcome_monitor_targets AS target
        ON target.owner_identity = run.owner_identity
       AND target.workspace_key = run.workspace_key
       AND target.target_id = run.target_id
     WHERE run.owner_identity = NEW.owner_identity
       AND run.workspace_key = NEW.workspace_key
       AND run.target_id = NEW.target_id
       AND run.run_id = NEW.run_id
       AND run.status = 'succeeded';
    IF NOT FOUND THEN
        RAISE EXCEPTION 'composition snapshot requires an exact successful owner-scoped run'
            USING ERRCODE = 'foreign_key_violation';
    END IF;

    IF NEW.snapshot_status = 'legacy_unpinned' THEN
        IF pg_trigger_depth() <= 1
           OR NEW.composer_version <> 'ambient-monitor-composer/pre-0051-unknown'
           OR NEW.snapshot_captured_at IS DISTINCT FROM scoped_run.completed_at
           OR NEW.outcome_revision IS NOT NULL
           OR NEW.outcome_audit_digest IS NOT NULL
           OR NEW.policy_idempotency_key IS NOT NULL
           OR NEW.policy_payload_digest IS NOT NULL
           OR NEW.policy_recorded_at IS NOT NULL
           OR NEW.signal_watermark_at IS NOT NULL
           OR NEW.signal_watermark_key IS NOT NULL
           OR NEW.decision_watermark_at IS NOT NULL
           OR NEW.decision_watermark_key IS NOT NULL
           OR NEW.feedback_watermark_at IS NOT NULL
           OR NEW.feedback_watermark_id IS NOT NULL
           OR NEW.feedback_watermark_digest IS NOT NULL
           OR NEW.attention_snapshot IS NOT NULL
           OR NEW.attention_snapshot_digest IS NOT NULL THEN
            RAISE EXCEPTION 'legacy_unpinned is reserved for migration backfill or trigger-only enqueue'
                USING ERRCODE = 'integrity_constraint_violation';
        END IF;
        expected_digest := public.hai_outcome_monitor_composition_snapshot_digest(
            NEW.snapshot_status, NEW.composer_version,
            NEW.snapshot_captured_at, NULL, NULL, NULL
        );
        IF NEW.snapshot_digest IS NULL THEN
            NEW.snapshot_digest := expected_digest;
        ELSIF NEW.snapshot_digest <> expected_digest THEN
            RAISE EXCEPTION 'legacy composition snapshot digest is inconsistent'
                USING ERRCODE = 'integrity_constraint_violation';
        END IF;
        RETURN NEW;
    END IF;

    IF NEW.attention_snapshot IS NULL THEN
        RAISE EXCEPTION 'new explicit composition delivery requires an exact EvaluationSnapshot JSON envelope'
            USING ERRCODE = 'integrity_constraint_violation';
    END IF;
    attention_captured_at := (NEW.attention_snapshot #>> '{capturedAt}')::timestamp with time zone;
    IF attention_captured_at < scoped_run.completed_at
       OR attention_captured_at > NEW.created_at
       OR NOT public.hai_validate_outcome_monitor_attention_snapshot(
           NEW.owner_identity, attention_captured_at, NEW.attention_snapshot
       ) THEN
        RAISE EXCEPTION 'composition attention snapshot does not match immutable source state at the run cutoff'
            USING ERRCODE = 'integrity_constraint_violation';
    END IF;

    IF NEW.outcome_revision IS NULL OR NEW.outcome_audit_digest IS NULL THEN
        RAISE EXCEPTION 'new composition delivery must provide an explicit outcome revision and audit digest'
            USING ERRCODE = 'integrity_constraint_violation';
    END IF;
    SELECT revision, audit_digest
      INTO exact_outcome
      FROM public.outcome_evaluation_outcome_revisions
     WHERE owner_identity = NEW.owner_identity
       AND workspace_id = NEW.workspace_key
       AND outcome_id = scoped_run.outcome_key
       AND revision = NEW.outcome_revision
       AND audit_digest = NEW.outcome_audit_digest
       AND recorded_at <= attention_captured_at;
    IF NOT FOUND THEN
        RAISE EXCEPTION 'new composition delivery cannot resolve its explicit outcome revision and audit digest'
            USING ERRCODE = 'foreign_key_violation';
    END IF;

    attention_policy_key := NEW.attention_snapshot #>> '{policy,idempotencyKey}';
    attention_policy_digest := NEW.attention_snapshot #>> '{policy,payloadDigest}';
    attention_policy_at := (NEW.attention_snapshot #>> '{policy,recordedAt}')::timestamp with time zone;
    IF (NEW.attention_snapshot #>> '{signals,count}')::integer > 0 THEN
        attention_signal_at := (NEW.attention_snapshot #>> '{signals,cursor,recordedAt}')::timestamp with time zone;
        attention_signal_key := jsonb_build_array(
            NEW.attention_snapshot #>> '{signals,cursor,idempotencyKey}',
            (NEW.attention_snapshot #>> '{signals,cursor,ordinal}')::integer
        )::text;
    END IF;
    IF (NEW.attention_snapshot #>> '{decisions,count}')::integer > 0 THEN
        attention_decision_at := (NEW.attention_snapshot #>> '{decisions,cursor,recordedAt}')::timestamp with time zone;
        attention_decision_key := jsonb_build_array(
            NEW.attention_snapshot #>> '{decisions,cursor,idempotencyKey}',
            (NEW.attention_snapshot #>> '{decisions,cursor,ordinal}')::integer
        )::text;
    END IF;
    IF (NEW.attention_snapshot #>> '{feedback,count}')::integer > 0 THEN
        attention_feedback_at := (NEW.attention_snapshot #>> '{feedback,cursor,recordedAt}')::timestamp with time zone;
        attention_feedback_id := (NEW.attention_snapshot #>> '{feedback,cursor,feedbackId}')::uuid;
        attention_feedback_digest := NEW.attention_snapshot #>> '{feedback,cursor,recordDigest}';
    END IF;
    attention_input_digest := NEW.attention_snapshot #>> '{inputDigest}';

    IF NEW.snapshot_status <> 'pinned'
       OR NEW.composer_version <> 'ambient-outcome-attention-v2'
       OR NEW.snapshot_captured_at IS DISTINCT FROM attention_captured_at
       OR NEW.outcome_revision IS DISTINCT FROM exact_outcome.revision
       OR NEW.outcome_audit_digest IS DISTINCT FROM exact_outcome.audit_digest
       OR NEW.policy_idempotency_key IS DISTINCT FROM attention_policy_key
       OR NEW.policy_payload_digest IS DISTINCT FROM attention_policy_digest
       OR NEW.policy_recorded_at IS DISTINCT FROM attention_policy_at
       OR NEW.signal_watermark_at IS DISTINCT FROM attention_signal_at
       OR NEW.signal_watermark_key IS DISTINCT FROM attention_signal_key
       OR NEW.decision_watermark_at IS DISTINCT FROM attention_decision_at
       OR NEW.decision_watermark_key IS DISTINCT FROM attention_decision_key
       OR NEW.feedback_watermark_at IS DISTINCT FROM attention_feedback_at
       OR NEW.feedback_watermark_id IS DISTINCT FROM attention_feedback_id
       OR NEW.feedback_watermark_digest IS DISTINCT FROM attention_feedback_digest
       OR NEW.attention_snapshot_digest IS DISTINCT FROM attention_input_digest THEN
        RAISE EXCEPTION 'composition snapshot does not match exact outcome and EvaluationSnapshot inputs'
            USING ERRCODE = 'integrity_constraint_violation';
    END IF;

    expected_digest := public.hai_outcome_monitor_composition_snapshot_digest(
        NEW.snapshot_status, NEW.composer_version, NEW.snapshot_captured_at,
        NEW.outcome_revision, NEW.outcome_audit_digest, NEW.attention_snapshot
    );
    IF NEW.snapshot_digest IS NULL OR NEW.snapshot_digest <> expected_digest THEN
        RAISE EXCEPTION 'stored snapshot_digest must equal the exact Go composition snapshot digest'
            USING ERRCODE = 'integrity_constraint_violation';
    END IF;
    RETURN NEW;
END;
$$;

DROP TRIGGER trg_outcome_monitor_composition_delivery_validate_update
    ON public.outcome_monitor_composition_deliveries;
DROP TRIGGER trg_outcome_monitor_composition_attempt_immutable
    ON public.outcome_monitor_composition_attempts;
ALTER TABLE public.outcome_monitor_composition_deliveries
    DROP CONSTRAINT chk_outcome_monitor_composition_delivery_attempt_projection;

WITH legacy_snapshot AS (
    SELECT delivery.*, run.completed_at AS run_completed_at,
           public.hai_outcome_monitor_composition_snapshot_digest(
               'legacy_unpinned', 'ambient-monitor-composer/pre-0051-unknown',
               run.completed_at, NULL, NULL, NULL
           ) AS resolved_snapshot_digest
      FROM public.outcome_monitor_composition_deliveries AS delivery
      JOIN public.outcome_monitor_runs AS run
        ON run.owner_identity = delivery.owner_identity
       AND run.workspace_key = delivery.workspace_key
       AND run.run_id = delivery.run_id
), legacy AS (
    SELECT legacy_snapshot.*,
           public.hai_outcome_monitor_composition_binding_digest(
               owner_identity, workspace_key, delivery_id, target_id, run_id,
               run_digest, observation_id, observation_digest,
               resolved_snapshot_digest
           ) AS resolved_binding_digest
      FROM legacy_snapshot
)
UPDATE public.outcome_monitor_composition_deliveries AS delivery
   SET status = 'dead_lettered',
       revision = delivery.revision + 1,
       next_attempt_at = NULL,
       lease_id = NULL,
       lease_owner = NULL,
       lease_until = NULL,
       last_failure_code = 'snapshot_unavailable',
       completed_at = GREATEST(legacy.run_completed_at, delivery.updated_at),
       updated_at = GREATEST(legacy.run_completed_at, delivery.updated_at),
       binding_digest = legacy.resolved_binding_digest,
       snapshot_status = 'legacy_unpinned',
       composer_version = 'ambient-monitor-composer/pre-0051-unknown',
       snapshot_captured_at = legacy.run_completed_at,
       outcome_revision = NULL,
       outcome_audit_digest = NULL,
       policy_idempotency_key = NULL,
       policy_payload_digest = NULL,
       policy_recorded_at = NULL,
       signal_watermark_at = NULL,
       signal_watermark_key = NULL,
       decision_watermark_at = NULL,
       decision_watermark_key = NULL,
       feedback_watermark_at = NULL,
       feedback_watermark_id = NULL,
       feedback_watermark_digest = NULL,
       attention_snapshot = NULL,
       attention_snapshot_digest = NULL,
       snapshot_digest = legacy.resolved_snapshot_digest
 FROM legacy
 WHERE delivery.delivery_id = legacy.delivery_id;

UPDATE public.outcome_monitor_composition_attempts AS attempt
   SET snapshot_digest = delivery.snapshot_digest
  FROM public.outcome_monitor_composition_deliveries AS delivery
 WHERE delivery.owner_identity = attempt.owner_identity
   AND delivery.workspace_key = attempt.workspace_key
   AND delivery.delivery_id = attempt.delivery_id;

ALTER TABLE public.outcome_monitor_composition_deliveries
    ALTER COLUMN snapshot_status SET NOT NULL,
    ALTER COLUMN composer_version SET NOT NULL,
    ALTER COLUMN snapshot_captured_at SET NOT NULL,
    ALTER COLUMN snapshot_digest SET NOT NULL,
    ADD CONSTRAINT chk_outcome_monitor_composition_snapshot_shape CHECK (
        snapshot_status IN ('pinned', 'legacy_unpinned')
        AND snapshot_digest ~ '^[0-9a-f]{64}$'
        AND snapshot_captured_at <= created_at
        AND (
            (
                snapshot_status = 'pinned'
                AND composer_version = 'ambient-outcome-attention-v2'
                AND outcome_revision > 0
                AND outcome_audit_digest ~ '^[0-9a-f]{64}$'
                AND char_length(policy_idempotency_key) BETWEEN 1 AND 200
                AND policy_payload_digest ~ '^[0-9a-f]{64}$'
                AND policy_recorded_at <= snapshot_captured_at
                AND jsonb_typeof(attention_snapshot) = 'object'
                AND octet_length(attention_snapshot::text) BETWEEN 2 AND 131072
                AND attention_snapshot_digest ~ '^[0-9a-f]{64}$'
                AND attention_snapshot #>> '{inputDigest}' = attention_snapshot_digest
                AND attention_snapshot #>> '{ownerIdentity}' = owner_identity
                AND (attention_snapshot #>> '{capturedAt}')::timestamp with time zone
                    = snapshot_captured_at
                AND (
                    (signal_watermark_at IS NULL AND signal_watermark_key IS NULL)
                    OR (
                        signal_watermark_at <= snapshot_captured_at
                        AND char_length(signal_watermark_key) BETWEEN 6 AND 1024
                    )
                )
                AND (
                    (decision_watermark_at IS NULL AND decision_watermark_key IS NULL)
                    OR (
                        decision_watermark_at <= snapshot_captured_at
                        AND char_length(decision_watermark_key) BETWEEN 6 AND 1024
                    )
                )
                AND (
                    (
                        feedback_watermark_at IS NULL
                        AND feedback_watermark_id IS NULL
                        AND feedback_watermark_digest IS NULL
                    )
                    OR (
                        feedback_watermark_at <= snapshot_captured_at
                        AND feedback_watermark_id IS NOT NULL
                        AND feedback_watermark_digest ~ '^[0-9a-f]{64}$'
                    )
                )
            )
            OR (
                snapshot_status = 'legacy_unpinned'
                AND composer_version = 'ambient-monitor-composer/pre-0051-unknown'
                AND status = 'dead_lettered'
                AND next_attempt_at IS NULL
                AND last_failure_code = 'snapshot_unavailable'
                AND completed_at IS NOT NULL
                AND outcome_revision IS NULL
                AND outcome_audit_digest IS NULL
                AND policy_idempotency_key IS NULL
                AND policy_payload_digest IS NULL
                AND policy_recorded_at IS NULL
                AND signal_watermark_at IS NULL
                AND signal_watermark_key IS NULL
                AND decision_watermark_at IS NULL
                AND decision_watermark_key IS NULL
                AND feedback_watermark_at IS NULL
                AND feedback_watermark_id IS NULL
                AND feedback_watermark_digest IS NULL
                AND attention_snapshot IS NULL
                AND attention_snapshot_digest IS NULL
            )
        )
    ),
    ADD CONSTRAINT fk_outcome_monitor_composition_snapshot_policy
        FOREIGN KEY (owner_identity, policy_idempotency_key)
        REFERENCES public.proactivity_policy_records
            (owner_identity, idempotency_key)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    ADD CONSTRAINT fk_outcome_monitor_composition_snapshot_feedback
        FOREIGN KEY (owner_identity, feedback_watermark_id)
        REFERENCES public.proactivity_feedback_records
            (owner_identity, feedback_id)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    ADD CONSTRAINT uq_outcome_monitor_composition_delivery_snapshot
        UNIQUE (owner_identity, workspace_key, delivery_id, snapshot_digest),
    ADD CONSTRAINT chk_outcome_monitor_composition_delivery_attempt_projection CHECK (
        (
            snapshot_status = 'legacy_unpinned'
            AND status = 'dead_lettered'
            AND last_failure_code = 'snapshot_unavailable'
            AND completed_at IS NOT NULL
        )
        OR (
            snapshot_status = 'pinned'
            AND (
                (
                    attempt_count = 0
                    AND last_attempt_at IS NULL
                    AND last_failure_code IS NULL
                )
                OR (
                    attempt_count > 0
                    AND last_attempt_at IS NOT NULL
                    AND last_attempt_at <= updated_at
                    AND (
                        last_failure_code IS NULL
                        OR last_failure_code ~ '^[a-z][a-z0-9_]{0,79}$'
                    )
                )
            )
        )
    );

ALTER TABLE public.outcome_monitor_composition_attempts
    ALTER COLUMN snapshot_digest SET NOT NULL,
    ADD CONSTRAINT chk_outcome_monitor_composition_attempt_snapshot_digest
        CHECK (snapshot_digest ~ '^[0-9a-f]{64}$'),
    ADD CONSTRAINT fk_outcome_monitor_composition_attempt_snapshot
        FOREIGN KEY (owner_identity, workspace_key, delivery_id, snapshot_digest)
        REFERENCES public.outcome_monitor_composition_deliveries
            (owner_identity, workspace_key, delivery_id, snapshot_digest)
        ON UPDATE RESTRICT ON DELETE RESTRICT;

CREATE OR REPLACE FUNCTION public.hai_validate_outcome_monitor_composition_attempt_insert()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    delivery_record public.outcome_monitor_composition_deliveries%ROWTYPE;
BEGIN
    SELECT * INTO delivery_record
      FROM public.outcome_monitor_composition_deliveries
     WHERE owner_identity = NEW.owner_identity
       AND workspace_key = NEW.workspace_key
       AND delivery_id = NEW.delivery_id
     FOR UPDATE;
    IF NEW.queue_revision = 0 THEN
        NEW.queue_revision := delivery_record.revision;
    END IF;
    IF NOT FOUND
       OR NEW.snapshot_digest IS NULL
       OR NEW.snapshot_digest IS DISTINCT FROM delivery_record.snapshot_digest
       OR delivery_record.status <> 'pending'
       OR delivery_record.revision <> NEW.queue_revision
       OR delivery_record.lease_generation <> NEW.lease_generation
       OR delivery_record.lease_id IS NULL
       OR delivery_record.lease_id <> NEW.claim_id
       OR delivery_record.lease_owner IS DISTINCT FROM NEW.worker_id
       OR delivery_record.lease_until IS NULL
       OR delivery_record.lease_until < NEW.finished_at
       OR delivery_record.target_id <> NEW.target_id
       OR delivery_record.run_id <> NEW.run_id
       OR delivery_record.run_digest <> NEW.run_digest
       OR NEW.attempt_number <> delivery_record.attempt_count + 1
       OR NEW.attempt_number > delivery_record.max_attempts THEN
        RAISE EXCEPTION 'composition attempt does not match the current owner-scoped fenced lease and immutable snapshot'
            USING ERRCODE = 'serialization_failure';
    END IF;
    RETURN NEW;
END;
$$;

CREATE OR REPLACE FUNCTION public.hai_validate_outcome_monitor_composition_delivery_insert()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    run_record public.outcome_monitor_runs%ROWTYPE;
    observation_record public.outcome_observation_records%ROWTYPE;
    expected_binding character(64);
BEGIN
    SELECT * INTO run_record
      FROM public.outcome_monitor_runs
     WHERE owner_identity = NEW.owner_identity
       AND workspace_key = NEW.workspace_key
       AND run_id = NEW.run_id;
    IF NOT FOUND
       OR run_record.status <> 'succeeded'
       OR run_record.target_id <> NEW.target_id
       OR run_record.record_digest <> NEW.run_digest
       OR NEW.delivery_id <> NEW.run_id THEN
        RAISE EXCEPTION 'composition delivery requires the exact owner-scoped successful monitor run'
            USING ERRCODE = 'foreign_key_violation';
    END IF;
    IF NEW.observation_id IS NOT NULL THEN
        SELECT * INTO observation_record
          FROM public.outcome_observation_records
         WHERE owner_identity = NEW.owner_identity
           AND workspace_key = NEW.workspace_key
           AND observation_id = NEW.observation_id;
        IF NOT FOUND OR observation_record.record_digest <> NEW.observation_digest THEN
            RAISE EXCEPTION 'composition delivery observation digest does not match owner-scoped evidence'
                USING ERRCODE = 'foreign_key_violation';
        END IF;
    END IF;
    expected_binding := public.hai_outcome_monitor_composition_binding_digest(
        NEW.owner_identity, NEW.workspace_key, NEW.delivery_id, NEW.target_id,
        NEW.run_id, NEW.run_digest, NEW.observation_id,
        NEW.observation_digest, NEW.snapshot_digest
    );
    IF NEW.binding_digest IS DISTINCT FROM expected_binding THEN
        RAISE EXCEPTION 'composition delivery binding must equal the exact Go snapshot-bound binding digest'
            USING ERRCODE = 'integrity_constraint_violation';
    END IF;
    IF NEW.revision <> 1
       OR NEW.lease_generation <> 0
       OR NEW.lease_id IS NOT NULL
       OR NEW.lease_owner IS NOT NULL
       OR NEW.lease_until IS NOT NULL
       OR NEW.attempt_count <> 0
       OR NEW.last_attempt_at IS NOT NULL
       OR NEW.created_at <> NEW.updated_at
       OR NEW.created_at <> NEW.snapshot_captured_at THEN
        RAISE EXCEPTION 'new composition delivery must start at its immutable snapshot capture revision'
            USING ERRCODE = 'integrity_constraint_violation';
    END IF;
    IF NEW.snapshot_status = 'pinned' THEN
        IF NEW.status <> 'pending'
           OR NEW.next_attempt_at < run_record.completed_at
           OR NEW.last_failure_code IS NOT NULL
           OR NEW.completed_at IS NOT NULL THEN
            RAISE EXCEPTION 'new pinned composition delivery must start as an unclaimed pending revision one'
                USING ERRCODE = 'integrity_constraint_violation';
        END IF;
    ELSIF NEW.snapshot_status = 'legacy_unpinned' THEN
        IF NEW.status <> 'dead_lettered'
           OR NEW.next_attempt_at IS NOT NULL
           OR NEW.last_failure_code <> 'snapshot_unavailable'
           OR NEW.completed_at IS DISTINCT FROM NEW.created_at THEN
            RAISE EXCEPTION 'legacy composition delivery must be quarantined as snapshot_unavailable'
                USING ERRCODE = 'integrity_constraint_violation';
        END IF;
    ELSE
        RAISE EXCEPTION 'composition delivery snapshot status is invalid'
            USING ERRCODE = 'integrity_constraint_violation';
    END IF;
    RETURN NEW;
END;
$$;

CREATE OR REPLACE FUNCTION public.hai_validate_outcome_monitor_composition_delivery_write()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    attempt_record public.outcome_monitor_composition_attempts%ROWTYPE;
    expected_binding character(64);
BEGIN
    IF TG_OP = 'DELETE' THEN
        RAISE EXCEPTION 'outcome monitor composition deliveries cannot be deleted'
            USING ERRCODE = '55000';
    END IF;
    expected_binding := public.hai_outcome_monitor_composition_binding_digest(
        NEW.owner_identity, NEW.workspace_key, NEW.delivery_id, NEW.target_id,
        NEW.run_id, NEW.run_digest, NEW.observation_id,
        NEW.observation_digest, NEW.snapshot_digest
    );
    IF NEW.binding_digest IS DISTINCT FROM expected_binding THEN
        RAISE EXCEPTION 'composition delivery binding no longer matches its immutable Go snapshot-bound digest'
            USING ERRCODE = 'integrity_constraint_violation';
    END IF;
    IF NEW.delivery_id <> OLD.delivery_id
       OR NEW.owner_identity <> OLD.owner_identity
       OR NEW.workspace_key <> OLD.workspace_key
       OR NEW.target_id <> OLD.target_id
       OR NEW.run_id <> OLD.run_id
       OR NEW.run_digest <> OLD.run_digest
       OR NEW.binding_digest <> OLD.binding_digest
       OR NEW.observation_id IS DISTINCT FROM OLD.observation_id
       OR NEW.observation_digest IS DISTINCT FROM OLD.observation_digest
       OR NEW.max_attempts <> OLD.max_attempts
       OR NEW.base_backoff_seconds <> OLD.base_backoff_seconds
       OR NEW.max_backoff_seconds <> OLD.max_backoff_seconds
       OR NEW.created_at <> OLD.created_at
       OR NEW.authority <> OLD.authority
       OR NEW.can_execute <> OLD.can_execute
       OR NEW.delivery_authorized <> OLD.delivery_authorized
       OR NEW.execution_authorized <> OLD.execution_authorized
       OR NEW.notification_authorized <> OLD.notification_authorized
       OR NEW.external_effects_authorized <> OLD.external_effects_authorized
       OR NEW.learning_mutation_authorized <> OLD.learning_mutation_authorized THEN
        RAISE EXCEPTION 'composition delivery identity, evidence, snapshot binding, policy, and capabilities are immutable'
            USING ERRCODE = 'integrity_constraint_violation';
    END IF;
    IF OLD.status <> 'pending' THEN
        RAISE EXCEPTION 'terminal composition delivery cannot transition'
            USING ERRCODE = '55000';
    END IF;
    IF NEW.revision <> OLD.revision + 1 OR NEW.updated_at <= OLD.updated_at THEN
        RAISE EXCEPTION 'composition delivery update must advance revision and time exactly once'
            USING ERRCODE = 'serialization_failure';
    END IF;
    IF OLD.lease_id IS NULL AND NEW.lease_id IS NOT NULL THEN
        IF NEW.status <> 'pending'
           OR NEW.lease_generation <> OLD.lease_generation + 1
           OR NEW.attempt_count <> OLD.attempt_count
           OR NEW.last_attempt_at IS DISTINCT FROM OLD.last_attempt_at
           OR NEW.last_failure_code IS DISTINCT FROM OLD.last_failure_code
           OR NEW.next_attempt_at IS DISTINCT FROM OLD.next_attempt_at
           OR OLD.next_attempt_at > NEW.updated_at THEN
            RAISE EXCEPTION 'composition delivery claim must fence one due pending revision'
                USING ERRCODE = 'serialization_failure';
        END IF;
        RETURN NEW;
    END IF;
    IF OLD.lease_id IS NULL OR NEW.lease_id IS NOT NULL THEN
        RAISE EXCEPTION 'composition delivery update must be a fenced claim or receipt-backed settlement'
            USING ERRCODE = 'serialization_failure';
    END IF;
    IF OLD.lease_until <= NEW.updated_at
       AND NEW.status = 'pending'
       AND NEW.lease_generation = OLD.lease_generation
       AND NEW.attempt_count = OLD.attempt_count
       AND NEW.next_attempt_at IS NOT DISTINCT FROM OLD.next_attempt_at
       AND NEW.last_attempt_at IS NOT DISTINCT FROM OLD.last_attempt_at
       AND NEW.last_failure_code IS NOT DISTINCT FROM OLD.last_failure_code
       AND NEW.completed_at IS NOT DISTINCT FROM OLD.completed_at THEN
        RETURN NEW;
    END IF;
    SELECT * INTO attempt_record
      FROM public.outcome_monitor_composition_attempts
     WHERE owner_identity = OLD.owner_identity
       AND workspace_key = OLD.workspace_key
       AND delivery_id = OLD.delivery_id
       AND attempt_number = OLD.attempt_count + 1;
    IF NOT FOUND
       OR attempt_record.snapshot_digest IS DISTINCT FROM OLD.snapshot_digest
       OR attempt_record.queue_revision <> OLD.revision
       OR attempt_record.lease_generation <> OLD.lease_generation
       OR attempt_record.claim_id <> OLD.lease_id
       OR attempt_record.worker_id IS DISTINCT FROM OLD.lease_owner
       OR NEW.attempt_count <> attempt_record.attempt_number
       OR NEW.last_attempt_at <> attempt_record.finished_at
       OR NEW.updated_at < attempt_record.finished_at
       OR NEW.lease_generation <> OLD.lease_generation THEN
        RAISE EXCEPTION 'composition delivery settlement does not match immutable snapshot attempt receipt'
            USING ERRCODE = 'integrity_constraint_violation';
    END IF;
    IF attempt_record.status = 'succeeded' THEN
        IF NEW.status <> 'succeeded'
           OR NEW.completed_at <> attempt_record.finished_at
           OR NEW.last_failure_code IS NOT NULL THEN
            RAISE EXCEPTION 'successful composition receipt requires an exact terminal projection'
                USING ERRCODE = 'integrity_constraint_violation';
        END IF;
    ELSIF NEW.status = 'dead_lettered' THEN
        IF NEW.completed_at <> attempt_record.finished_at
           OR NEW.last_failure_code IS DISTINCT FROM attempt_record.failure_code THEN
            RAISE EXCEPTION 'failed composition receipt requires an exact dead-letter projection'
                USING ERRCODE = 'integrity_constraint_violation';
        END IF;
    ELSE
        IF NEW.status <> 'pending'
           OR NEW.completed_at IS NOT NULL
           OR NEW.attempt_count >= NEW.max_attempts
           OR NEW.next_attempt_at < NEW.updated_at + make_interval(secs => OLD.base_backoff_seconds)
           OR NEW.next_attempt_at > NEW.updated_at + make_interval(secs => OLD.max_backoff_seconds)
           OR NEW.last_failure_code IS DISTINCT FROM attempt_record.failure_code THEN
            RAISE EXCEPTION 'failed composition receipt requires bounded retry or dead-letter projection'
                USING ERRCODE = 'integrity_constraint_violation';
        END IF;
    END IF;
    RETURN NEW;
END;
$$;

CREATE OR REPLACE FUNCTION public.hai_reject_outcome_monitor_composition_snapshot_mutation()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    IF TG_OP = 'TRUNCATE' THEN
        RAISE EXCEPTION 'outcome monitor composition snapshots cannot be truncated'
            USING ERRCODE = '55000';
    END IF;
    IF TG_OP = 'DELETE' THEN
        RAISE EXCEPTION 'outcome monitor composition snapshots cannot be deleted'
            USING ERRCODE = '55000';
    END IF;
    IF NEW.snapshot_status IS DISTINCT FROM OLD.snapshot_status
       OR NEW.composer_version IS DISTINCT FROM OLD.composer_version
       OR NEW.snapshot_captured_at IS DISTINCT FROM OLD.snapshot_captured_at
       OR NEW.outcome_revision IS DISTINCT FROM OLD.outcome_revision
       OR NEW.outcome_audit_digest IS DISTINCT FROM OLD.outcome_audit_digest
       OR NEW.policy_idempotency_key IS DISTINCT FROM OLD.policy_idempotency_key
       OR NEW.policy_payload_digest IS DISTINCT FROM OLD.policy_payload_digest
       OR NEW.policy_recorded_at IS DISTINCT FROM OLD.policy_recorded_at
       OR NEW.signal_watermark_at IS DISTINCT FROM OLD.signal_watermark_at
       OR NEW.signal_watermark_key IS DISTINCT FROM OLD.signal_watermark_key
       OR NEW.decision_watermark_at IS DISTINCT FROM OLD.decision_watermark_at
       OR NEW.decision_watermark_key IS DISTINCT FROM OLD.decision_watermark_key
       OR NEW.feedback_watermark_at IS DISTINCT FROM OLD.feedback_watermark_at
       OR NEW.feedback_watermark_id IS DISTINCT FROM OLD.feedback_watermark_id
       OR NEW.feedback_watermark_digest IS DISTINCT FROM OLD.feedback_watermark_digest
       OR NEW.attention_snapshot IS DISTINCT FROM OLD.attention_snapshot
       OR NEW.attention_snapshot_digest IS DISTINCT FROM OLD.attention_snapshot_digest
       OR NEW.snapshot_digest IS DISTINCT FROM OLD.snapshot_digest THEN
        RAISE EXCEPTION 'outcome monitor composition snapshot pins are immutable'
            USING ERRCODE = 'integrity_constraint_violation';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_outcome_monitor_composition_delivery_0051_snapshot_insert
    BEFORE INSERT ON public.outcome_monitor_composition_deliveries
    FOR EACH ROW EXECUTE FUNCTION public.hai_pin_outcome_monitor_composition_snapshot();
CREATE TRIGGER trg_outcome_monitor_composition_delivery_0051_snapshot_immutable
    BEFORE UPDATE OR DELETE ON public.outcome_monitor_composition_deliveries
    FOR EACH ROW EXECUTE FUNCTION public.hai_reject_outcome_monitor_composition_snapshot_mutation();
CREATE TRIGGER trg_outcome_monitor_composition_delivery_0051_snapshot_no_truncate
    BEFORE TRUNCATE ON public.outcome_monitor_composition_deliveries
    FOR EACH STATEMENT EXECUTE FUNCTION public.hai_reject_outcome_monitor_composition_snapshot_mutation();

CREATE TRIGGER trg_outcome_monitor_composition_delivery_validate_update
    BEFORE UPDATE ON public.outcome_monitor_composition_deliveries
    FOR EACH ROW EXECUTE FUNCTION public.hai_validate_outcome_monitor_composition_delivery_write();
CREATE TRIGGER trg_outcome_monitor_composition_attempt_immutable
    BEFORE UPDATE OR DELETE ON public.outcome_monitor_composition_attempts
    FOR EACH ROW EXECUTE FUNCTION public.hai_reject_outcome_monitor_composition_attempt_mutation();

CREATE OR REPLACE FUNCTION public.hai_enqueue_outcome_monitor_composition_delivery()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    matched_observation_id uuid;
    matched_observation_digest character(64);
    legacy_snapshot_digest character(64);
    derived_binding_digest character(64);
BEGIN
    IF COALESCE(current_setting('hai.outcome_monitor_pinned_enqueue', true), '') = 'on' THEN
        RETURN NEW;
    END IF;
    IF NEW.status = 'succeeded' THEN
        SELECT observation.observation_id, observation.record_digest
          INTO matched_observation_id, matched_observation_digest
          FROM public.outcome_observation_records AS observation
          JOIN public.outcome_monitor_targets AS target
            ON target.owner_identity = NEW.owner_identity
           AND target.workspace_key = NEW.workspace_key
           AND target.target_id = NEW.target_id
         WHERE observation.owner_identity = NEW.owner_identity
           AND observation.workspace_key = NEW.workspace_key
           AND observation.outcome_key = target.outcome_key
           AND observation.indicator_key = target.indicator_key
           AND observation.source_kind = target.source_kind
           AND observation.recorded_at <= NEW.completed_at
         ORDER BY
            (observation.idempotency_key = NEW.idempotency_key) DESC,
            (observation.recorded_at BETWEEN NEW.started_at AND NEW.completed_at) DESC,
            observation.recorded_at DESC,
            observation.observation_id DESC
         LIMIT 1;
        legacy_snapshot_digest := public.hai_outcome_monitor_composition_snapshot_digest(
            'legacy_unpinned', 'ambient-monitor-composer/pre-0051-unknown', NEW.completed_at,
            NULL, NULL, NULL
        );
        derived_binding_digest := public.hai_outcome_monitor_composition_binding_digest(
            NEW.owner_identity, NEW.workspace_key, NEW.run_id, NEW.target_id,
            NEW.run_id, NEW.record_digest, matched_observation_id,
            matched_observation_digest, legacy_snapshot_digest
        );
        INSERT INTO public.outcome_monitor_composition_deliveries (
            delivery_id, owner_identity, workspace_key, target_id, run_id,
            run_digest, observation_id, observation_digest,
            status, revision, lease_generation, attempt_count, max_attempts,
            base_backoff_seconds, max_backoff_seconds, next_attempt_at,
            last_failure_code, completed_at, created_at, updated_at,
            binding_digest, snapshot_status, composer_version,
            snapshot_captured_at, snapshot_digest
        ) VALUES (
            NEW.run_id, NEW.owner_identity, NEW.workspace_key, NEW.target_id, NEW.run_id,
            NEW.record_digest, matched_observation_id, matched_observation_digest,
            'dead_lettered', 1, 0, 0, 5, 30, 3600, NULL,
            'snapshot_unavailable', NEW.completed_at, NEW.completed_at,
            NEW.completed_at, derived_binding_digest, 'legacy_unpinned',
            'ambient-monitor-composer/pre-0051-unknown', NEW.completed_at,
            legacy_snapshot_digest
        ) ON CONFLICT (delivery_id) DO NOTHING;
    END IF;
    RETURN NEW;
END;
$$;

COMMENT ON COLUMN public.outcome_monitor_composition_deliveries.snapshot_status IS
    'pinned only with an exact EvaluationSnapshot. legacy_unpinned is limited to terminal needs-review quarantine for pre-0051 backfill or trigger-only enqueue.';
COMMENT ON COLUMN public.outcome_monitor_composition_deliveries.signal_watermark_key IS
    'Canonical JSON array [batch_idempotency_key, ordinal] identifying the exact signal history tip at snapshot_captured_at.';
COMMENT ON COLUMN public.outcome_monitor_composition_deliveries.decision_watermark_key IS
    'Canonical JSON array [batch_idempotency_key, ordinal] identifying the exact decision history tip at snapshot_captured_at.';
COMMENT ON COLUMN public.outcome_monitor_composition_deliveries.attention_snapshot IS
    'Bounded proactivity EvaluationSnapshot with exact policy, cursor ordinals, counts, window digests, and input digest.';
COMMENT ON COLUMN public.outcome_monitor_composition_deliveries.attention_snapshot_digest IS
    'Exact EvaluationSnapshot inputDigest supplied by the proactivity snapshot implementation and never reconstructed by SQL.';
COMMENT ON COLUMN public.outcome_monitor_composition_deliveries.snapshot_digest IS
    'Exact Go CompositionSnapshot.SnapshotDigest and never replaced by a distinct SQL envelope digest.';
COMMENT ON COLUMN public.outcome_monitor_composition_attempts.snapshot_digest IS
    'Immutable copy of the delivery CompositionSnapshot.SnapshotDigest used for this replay receipt.';
