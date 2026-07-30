CREATE TABLE IF NOT EXISTS public.knowledge_graph_nodes (
    id character varying(160) NOT NULL,
    owner_identity character varying(255) NOT NULL,
    kind character varying(40) NOT NULL,
    deduplication_key character varying(512) NOT NULL,
    label character varying(1000) DEFAULT ''::character varying NOT NULL,
    content text DEFAULT ''::text NOT NULL,
    properties_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    project_keys_json jsonb DEFAULT '[]'::jsonb NOT NULL,
    tags_json jsonb DEFAULT '[]'::jsonb NOT NULL,
    confidence numeric(5,4) NOT NULL,
    verification_status character varying(40) NOT NULL,
    sources_json jsonb DEFAULT '[]'::jsonb NOT NULL,
    valid_from timestamp with time zone,
    valid_until timestamp with time zone,
    sensitivity character varying(32) NOT NULL,
    local_only boolean DEFAULT false NOT NULL,
    conflict_group_id character varying(160) DEFAULT ''::character varying NOT NULL,
    supersedes_id character varying(160) DEFAULT ''::character varying NOT NULL,
    corrected_by_id character varying(160) DEFAULT ''::character varying NOT NULL,
    archived boolean DEFAULT false NOT NULL,
    revision bigint DEFAULT 1 NOT NULL,
    transaction_from timestamp with time zone NOT NULL,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL,
    deleted_at timestamp with time zone,
    CONSTRAINT knowledge_graph_nodes_pkey PRIMARY KEY (id),
    CONSTRAINT uq_knowledge_graph_nodes_owner_id UNIQUE (owner_identity, id),
    CONSTRAINT chk_knowledge_graph_nodes_owner CHECK (btrim(owner_identity) <> ''),
    CONSTRAINT chk_knowledge_graph_nodes_id CHECK (btrim(id) <> ''),
    CONSTRAINT chk_knowledge_graph_nodes_kind CHECK (
        kind::text = ANY (ARRAY[
            'person', 'organization', 'project', 'goal', 'task', 'event',
            'document', 'source', 'claim', 'preference', 'decision',
            'obligation', 'deadline', 'place', 'account', 'capability'
        ]::text[])
    ),
    CONSTRAINT chk_knowledge_graph_nodes_content CHECK (
        btrim(label) <> '' OR btrim(content) <> ''
    ),
    CONSTRAINT chk_knowledge_graph_nodes_properties_object CHECK (
        jsonb_typeof(properties_json) = 'object'
    ),
    CONSTRAINT chk_knowledge_graph_nodes_project_keys_array CHECK (
        jsonb_typeof(project_keys_json) = 'array'
    ),
    CONSTRAINT chk_knowledge_graph_nodes_tags_array CHECK (
        jsonb_typeof(tags_json) = 'array'
    ),
    CONSTRAINT chk_knowledge_graph_nodes_sources_array CHECK (
        jsonb_typeof(sources_json) = 'array'
    ),
    CONSTRAINT chk_knowledge_graph_nodes_confidence CHECK (
        confidence BETWEEN 0 AND 1
    ),
    CONSTRAINT chk_knowledge_graph_nodes_verification CHECK (
        verification_status::text = ANY (ARRAY[
            'unverified', 'source_supported', 'schema_validated',
            'test_passed', 'human_approved', 'verified', 'uncertain',
            'conflicting', 'unsupported', 'needs_review'
        ]::text[])
    ),
    CONSTRAINT chk_knowledge_graph_nodes_sensitivity CHECK (
        sensitivity::text = ANY (
            ARRAY['public', 'internal', 'sensitive', 'restricted']::text[]
        )
    ),
    CONSTRAINT chk_knowledge_graph_nodes_valid_time CHECK (
        valid_until IS NULL OR valid_from IS NULL OR valid_until >= valid_from
    ),
    CONSTRAINT chk_knowledge_graph_nodes_revision CHECK (revision > 0),
    CONSTRAINT chk_knowledge_graph_nodes_transaction_time CHECK (
        updated_at >= created_at AND transaction_from >= updated_at
    ),
    CONSTRAINT chk_knowledge_graph_nodes_tombstone CHECK (
        deleted_at IS NULL OR (
            archived
            AND deleted_at >= created_at
            AND deleted_at <= transaction_from
        )
    ),
    CONSTRAINT chk_knowledge_graph_nodes_correction_self CHECK (
        (supersedes_id = '' OR supersedes_id <> id)
        AND (corrected_by_id = '' OR corrected_by_id <> id)
    )
);

CREATE INDEX IF NOT EXISTS idx_knowledge_graph_nodes_owner_kind
    ON public.knowledge_graph_nodes USING btree (owner_identity, kind);
CREATE INDEX IF NOT EXISTS idx_knowledge_graph_nodes_owner_updated
    ON public.knowledge_graph_nodes USING btree (owner_identity, updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_knowledge_graph_nodes_dedup
    ON public.knowledge_graph_nodes USING btree (
        owner_identity, kind, deduplication_key
    );
CREATE INDEX IF NOT EXISTS idx_knowledge_graph_nodes_conflict
    ON public.knowledge_graph_nodes USING btree (owner_identity, conflict_group_id)
    WHERE conflict_group_id <> '';
CREATE INDEX IF NOT EXISTS idx_knowledge_graph_nodes_active
    ON public.knowledge_graph_nodes USING btree (owner_identity, kind, updated_at DESC)
    WHERE archived = false AND deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS public.knowledge_graph_edges (
    id character varying(160) NOT NULL,
    owner_identity character varying(255) NOT NULL,
    from_node_id character varying(160) NOT NULL,
    to_node_id character varying(160) NOT NULL,
    relationship character varying(48) NOT NULL,
    label character varying(1000) DEFAULT ''::character varying NOT NULL,
    properties_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    project_keys_json jsonb DEFAULT '[]'::jsonb NOT NULL,
    confidence numeric(5,4) NOT NULL,
    verification_status character varying(40) NOT NULL,
    sources_json jsonb DEFAULT '[]'::jsonb NOT NULL,
    valid_from timestamp with time zone,
    valid_until timestamp with time zone,
    sensitivity character varying(32) NOT NULL,
    local_only boolean DEFAULT false NOT NULL,
    archived boolean DEFAULT false NOT NULL,
    revision bigint DEFAULT 1 NOT NULL,
    transaction_from timestamp with time zone NOT NULL,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL,
    deleted_at timestamp with time zone,
    CONSTRAINT knowledge_graph_edges_pkey PRIMARY KEY (id),
    CONSTRAINT uq_knowledge_graph_edges_owner_id UNIQUE (owner_identity, id),
    CONSTRAINT fk_knowledge_graph_edges_from_node FOREIGN KEY (
        owner_identity, from_node_id
    ) REFERENCES public.knowledge_graph_nodes(owner_identity, id)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT fk_knowledge_graph_edges_to_node FOREIGN KEY (
        owner_identity, to_node_id
    ) REFERENCES public.knowledge_graph_nodes(owner_identity, id)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT chk_knowledge_graph_edges_owner CHECK (btrim(owner_identity) <> ''),
    CONSTRAINT chk_knowledge_graph_edges_id CHECK (btrim(id) <> ''),
    CONSTRAINT chk_knowledge_graph_edges_endpoints CHECK (
        btrim(from_node_id) <> ''
        AND btrim(to_node_id) <> ''
        AND from_node_id <> to_node_id
    ),
    CONSTRAINT chk_knowledge_graph_edges_relationship CHECK (
        relationship::text = ANY (ARRAY[
            'related_to', 'member_of', 'owns', 'works_on', 'supports',
            'depends_on', 'parent_of', 'assigned_to', 'caused_by',
            'derived_from', 'evidenced_by', 'contradicts', 'confirms',
            'prefers', 'decided', 'obligated_to', 'due_at', 'located_at',
            'capable_of', 'mentions', 'supersedes', 'corrected_by'
        ]::text[])
    ),
    CONSTRAINT chk_knowledge_graph_edges_properties_object CHECK (
        jsonb_typeof(properties_json) = 'object'
    ),
    CONSTRAINT chk_knowledge_graph_edges_project_keys_array CHECK (
        jsonb_typeof(project_keys_json) = 'array'
    ),
    CONSTRAINT chk_knowledge_graph_edges_sources_array CHECK (
        jsonb_typeof(sources_json) = 'array'
    ),
    CONSTRAINT chk_knowledge_graph_edges_confidence CHECK (
        confidence BETWEEN 0 AND 1
    ),
    CONSTRAINT chk_knowledge_graph_edges_verification CHECK (
        verification_status::text = ANY (ARRAY[
            'unverified', 'source_supported', 'schema_validated',
            'test_passed', 'human_approved', 'verified', 'uncertain',
            'conflicting', 'unsupported', 'needs_review'
        ]::text[])
    ),
    CONSTRAINT chk_knowledge_graph_edges_sensitivity CHECK (
        sensitivity::text = ANY (
            ARRAY['public', 'internal', 'sensitive', 'restricted']::text[]
        )
    ),
    CONSTRAINT chk_knowledge_graph_edges_valid_time CHECK (
        valid_until IS NULL OR valid_from IS NULL OR valid_until >= valid_from
    ),
    CONSTRAINT chk_knowledge_graph_edges_revision CHECK (revision > 0),
    CONSTRAINT chk_knowledge_graph_edges_transaction_time CHECK (
        updated_at >= created_at AND transaction_from >= updated_at
    ),
    CONSTRAINT chk_knowledge_graph_edges_tombstone CHECK (
        deleted_at IS NULL OR (
            archived
            AND deleted_at >= created_at
            AND deleted_at <= transaction_from
        )
    )
);

CREATE INDEX IF NOT EXISTS idx_knowledge_graph_edges_owner_relationship
    ON public.knowledge_graph_edges USING btree (owner_identity, relationship);
CREATE INDEX IF NOT EXISTS idx_knowledge_graph_edges_owner_updated
    ON public.knowledge_graph_edges USING btree (owner_identity, updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_knowledge_graph_edges_from
    ON public.knowledge_graph_edges USING btree (owner_identity, from_node_id);
CREATE INDEX IF NOT EXISTS idx_knowledge_graph_edges_to
    ON public.knowledge_graph_edges USING btree (owner_identity, to_node_id);
CREATE INDEX IF NOT EXISTS idx_knowledge_graph_edges_active
    ON public.knowledge_graph_edges USING btree (
        owner_identity, relationship, updated_at DESC
    ) WHERE archived = false AND deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS public.knowledge_graph_node_revisions (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    owner_identity character varying(255) NOT NULL,
    node_id character varying(160) NOT NULL,
    revision bigint NOT NULL,
    operation character varying(40) NOT NULL,
    snapshot_json jsonb NOT NULL,
    transaction_at timestamp with time zone NOT NULL,
    CONSTRAINT knowledge_graph_node_revisions_pkey PRIMARY KEY (id),
    CONSTRAINT uq_knowledge_graph_node_revisions UNIQUE (
        owner_identity, node_id, revision
    ),
    CONSTRAINT fk_knowledge_graph_node_revisions_node FOREIGN KEY (
        owner_identity, node_id
    ) REFERENCES public.knowledge_graph_nodes(owner_identity, id)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT chk_knowledge_graph_node_revisions_revision CHECK (revision > 0),
    CONSTRAINT chk_knowledge_graph_node_revisions_operation CHECK (
        operation::text = ANY (
            ARRAY['created', 'updated', 'archived', 'restored',
                  'corrected', 'tombstoned', 'conflict_recorded']::text[]
        )
    ),
    CONSTRAINT chk_knowledge_graph_node_revisions_snapshot CHECK (
        jsonb_typeof(snapshot_json) = 'object'
    )
);

CREATE INDEX IF NOT EXISTS idx_knowledge_graph_node_revisions_owner_node
    ON public.knowledge_graph_node_revisions USING btree (
        owner_identity, node_id, transaction_at DESC
    );

CREATE TABLE IF NOT EXISTS public.knowledge_graph_edge_revisions (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    owner_identity character varying(255) NOT NULL,
    edge_id character varying(160) NOT NULL,
    revision bigint NOT NULL,
    operation character varying(40) NOT NULL,
    snapshot_json jsonb NOT NULL,
    transaction_at timestamp with time zone NOT NULL,
    CONSTRAINT knowledge_graph_edge_revisions_pkey PRIMARY KEY (id),
    CONSTRAINT uq_knowledge_graph_edge_revisions UNIQUE (
        owner_identity, edge_id, revision
    ),
    CONSTRAINT fk_knowledge_graph_edge_revisions_edge FOREIGN KEY (
        owner_identity, edge_id
    ) REFERENCES public.knowledge_graph_edges(owner_identity, id)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT chk_knowledge_graph_edge_revisions_revision CHECK (revision > 0),
    CONSTRAINT chk_knowledge_graph_edge_revisions_operation CHECK (
        operation::text = ANY (
            ARRAY['created', 'updated', 'archived', 'restored',
                  'tombstoned']::text[]
        )
    ),
    CONSTRAINT chk_knowledge_graph_edge_revisions_snapshot CHECK (
        jsonb_typeof(snapshot_json) = 'object'
    )
);

CREATE INDEX IF NOT EXISTS idx_knowledge_graph_edge_revisions_owner_edge
    ON public.knowledge_graph_edge_revisions USING btree (
        owner_identity, edge_id, transaction_at DESC
    );

CREATE TABLE IF NOT EXISTS public.knowledge_graph_provenance_events (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    owner_identity character varying(255) NOT NULL,
    entity_type character varying(16) NOT NULL,
    entity_id character varying(160) NOT NULL,
    revision bigint NOT NULL,
    operation character varying(40) NOT NULL,
    sources_json jsonb DEFAULT '[]'::jsonb NOT NULL,
    recorded_at timestamp with time zone NOT NULL,
    CONSTRAINT knowledge_graph_provenance_events_pkey PRIMARY KEY (id),
    CONSTRAINT uq_knowledge_graph_provenance_revision UNIQUE (
        owner_identity, entity_type, entity_id, revision
    ),
    CONSTRAINT chk_knowledge_graph_provenance_type CHECK (
        entity_type::text = ANY (ARRAY['node', 'edge']::text[])
    ),
    CONSTRAINT chk_knowledge_graph_provenance_revision CHECK (revision > 0),
    CONSTRAINT chk_knowledge_graph_provenance_sources CHECK (
        jsonb_typeof(sources_json) = 'array'
    )
);

CREATE INDEX IF NOT EXISTS idx_knowledge_graph_provenance_entity
    ON public.knowledge_graph_provenance_events USING btree (
        owner_identity, entity_type, entity_id, recorded_at DESC
    );

CREATE TABLE IF NOT EXISTS public.knowledge_graph_conflict_records (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    owner_identity character varying(255) NOT NULL,
    conflict_group_id character varying(160) NOT NULL,
    node_id character varying(160) NOT NULL,
    detected_at timestamp with time zone NOT NULL,
    CONSTRAINT knowledge_graph_conflict_records_pkey PRIMARY KEY (id),
    CONSTRAINT uq_knowledge_graph_conflict_member UNIQUE (
        owner_identity, conflict_group_id, node_id
    ),
    CONSTRAINT fk_knowledge_graph_conflict_node FOREIGN KEY (
        owner_identity, node_id
    ) REFERENCES public.knowledge_graph_nodes(owner_identity, id)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT chk_knowledge_graph_conflict_group CHECK (
        btrim(conflict_group_id) <> ''
    )
);

CREATE INDEX IF NOT EXISTS idx_knowledge_graph_conflict_group
    ON public.knowledge_graph_conflict_records USING btree (
        owner_identity, conflict_group_id
    );

CREATE TABLE IF NOT EXISTS public.knowledge_graph_deletion_signals (
    id character varying(160) NOT NULL,
    owner_identity character varying(255) NOT NULL,
    entity_type character varying(16) NOT NULL,
    entity_id character varying(160) NOT NULL,
    propagated_edge_ids_json jsonb DEFAULT '[]'::jsonb NOT NULL,
    reason text NOT NULL,
    created_at timestamp with time zone NOT NULL,
    CONSTRAINT knowledge_graph_deletion_signals_pkey PRIMARY KEY (id),
    CONSTRAINT uq_knowledge_graph_deletion_entity UNIQUE (
        owner_identity, entity_type, entity_id
    ),
    CONSTRAINT chk_knowledge_graph_deletion_owner CHECK (
        btrim(owner_identity) <> ''
    ),
    CONSTRAINT chk_knowledge_graph_deletion_type CHECK (
        entity_type::text = ANY (ARRAY['node', 'edge']::text[])
    ),
    CONSTRAINT chk_knowledge_graph_deletion_edges CHECK (
        jsonb_typeof(propagated_edge_ids_json) = 'array'
    ),
    CONSTRAINT chk_knowledge_graph_deletion_reason CHECK (btrim(reason) <> '')
);

CREATE INDEX IF NOT EXISTS idx_knowledge_graph_deletions_owner_created
    ON public.knowledge_graph_deletion_signals USING btree (
        owner_identity, created_at, id
    );

CREATE OR REPLACE FUNCTION public.hai_enforce_knowledge_graph_current_update()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    IF NEW.id <> OLD.id
        OR NEW.owner_identity <> OLD.owner_identity
        OR NEW.created_at <> OLD.created_at
    THEN
        RAISE EXCEPTION 'knowledge graph identity and creation time are immutable';
    END IF;
    IF OLD.deleted_at IS NOT NULL THEN
        RAISE EXCEPTION 'knowledge graph tombstones cannot be mutated';
    END IF;
    IF NEW.revision <> OLD.revision + 1
        OR NEW.transaction_from < OLD.transaction_from
        OR NEW.updated_at < OLD.updated_at
    THEN
        RAISE EXCEPTION 'knowledge graph updates must advance revision and transaction time';
    END IF;
    RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS trg_knowledge_graph_nodes_update
    ON public.knowledge_graph_nodes;
CREATE TRIGGER trg_knowledge_graph_nodes_update
    BEFORE UPDATE ON public.knowledge_graph_nodes
    FOR EACH ROW EXECUTE FUNCTION public.hai_enforce_knowledge_graph_current_update();

DROP TRIGGER IF EXISTS trg_knowledge_graph_edges_update
    ON public.knowledge_graph_edges;
CREATE TRIGGER trg_knowledge_graph_edges_update
    BEFORE UPDATE ON public.knowledge_graph_edges
    FOR EACH ROW EXECUTE FUNCTION public.hai_enforce_knowledge_graph_current_update();

CREATE OR REPLACE FUNCTION public.hai_validate_knowledge_graph_correction_links()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    IF NEW.supersedes_id <> '' AND NOT EXISTS (
        SELECT 1 FROM public.knowledge_graph_nodes candidate
        WHERE candidate.owner_identity = NEW.owner_identity
          AND candidate.id = NEW.supersedes_id
    ) THEN
        RAISE EXCEPTION 'superseded node is not available to owner';
    END IF;
    IF NEW.corrected_by_id <> '' AND NOT EXISTS (
        SELECT 1 FROM public.knowledge_graph_nodes candidate
        WHERE candidate.owner_identity = NEW.owner_identity
          AND candidate.id = NEW.corrected_by_id
    ) THEN
        RAISE EXCEPTION 'correcting node is not available to owner';
    END IF;
    RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS trg_knowledge_graph_nodes_correction_links
    ON public.knowledge_graph_nodes;
CREATE CONSTRAINT TRIGGER trg_knowledge_graph_nodes_correction_links
    AFTER INSERT OR UPDATE ON public.knowledge_graph_nodes
    DEFERRABLE INITIALLY DEFERRED
    FOR EACH ROW EXECUTE FUNCTION public.hai_validate_knowledge_graph_correction_links();

CREATE OR REPLACE FUNCTION public.hai_reject_knowledge_graph_physical_delete()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE EXCEPTION 'knowledge graph current records must be tombstoned, not deleted';
END;
$$;

DROP TRIGGER IF EXISTS trg_knowledge_graph_nodes_no_delete
    ON public.knowledge_graph_nodes;
CREATE TRIGGER trg_knowledge_graph_nodes_no_delete
    BEFORE DELETE OR TRUNCATE ON public.knowledge_graph_nodes
    FOR EACH STATEMENT EXECUTE FUNCTION public.hai_reject_knowledge_graph_physical_delete();

DROP TRIGGER IF EXISTS trg_knowledge_graph_edges_no_delete
    ON public.knowledge_graph_edges;
CREATE TRIGGER trg_knowledge_graph_edges_no_delete
    BEFORE DELETE OR TRUNCATE ON public.knowledge_graph_edges
    FOR EACH STATEMENT EXECUTE FUNCTION public.hai_reject_knowledge_graph_physical_delete();

CREATE OR REPLACE FUNCTION public.hai_reject_knowledge_graph_history_mutation()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE EXCEPTION 'knowledge graph evidence and history records are immutable';
END;
$$;

DROP TRIGGER IF EXISTS trg_knowledge_graph_node_revisions_immutable
    ON public.knowledge_graph_node_revisions;
CREATE TRIGGER trg_knowledge_graph_node_revisions_immutable
    BEFORE UPDATE OR DELETE ON public.knowledge_graph_node_revisions
    FOR EACH ROW EXECUTE FUNCTION public.hai_reject_knowledge_graph_history_mutation();
DROP TRIGGER IF EXISTS trg_knowledge_graph_node_revisions_no_truncate
    ON public.knowledge_graph_node_revisions;
CREATE TRIGGER trg_knowledge_graph_node_revisions_no_truncate
    BEFORE TRUNCATE ON public.knowledge_graph_node_revisions
    FOR EACH STATEMENT EXECUTE FUNCTION public.hai_reject_knowledge_graph_history_mutation();

DROP TRIGGER IF EXISTS trg_knowledge_graph_edge_revisions_immutable
    ON public.knowledge_graph_edge_revisions;
CREATE TRIGGER trg_knowledge_graph_edge_revisions_immutable
    BEFORE UPDATE OR DELETE ON public.knowledge_graph_edge_revisions
    FOR EACH ROW EXECUTE FUNCTION public.hai_reject_knowledge_graph_history_mutation();
DROP TRIGGER IF EXISTS trg_knowledge_graph_edge_revisions_no_truncate
    ON public.knowledge_graph_edge_revisions;
CREATE TRIGGER trg_knowledge_graph_edge_revisions_no_truncate
    BEFORE TRUNCATE ON public.knowledge_graph_edge_revisions
    FOR EACH STATEMENT EXECUTE FUNCTION public.hai_reject_knowledge_graph_history_mutation();

DROP TRIGGER IF EXISTS trg_knowledge_graph_provenance_immutable
    ON public.knowledge_graph_provenance_events;
CREATE TRIGGER trg_knowledge_graph_provenance_immutable
    BEFORE UPDATE OR DELETE ON public.knowledge_graph_provenance_events
    FOR EACH ROW EXECUTE FUNCTION public.hai_reject_knowledge_graph_history_mutation();
DROP TRIGGER IF EXISTS trg_knowledge_graph_provenance_no_truncate
    ON public.knowledge_graph_provenance_events;
CREATE TRIGGER trg_knowledge_graph_provenance_no_truncate
    BEFORE TRUNCATE ON public.knowledge_graph_provenance_events
    FOR EACH STATEMENT EXECUTE FUNCTION public.hai_reject_knowledge_graph_history_mutation();

DROP TRIGGER IF EXISTS trg_knowledge_graph_conflicts_immutable
    ON public.knowledge_graph_conflict_records;
CREATE TRIGGER trg_knowledge_graph_conflicts_immutable
    BEFORE UPDATE OR DELETE ON public.knowledge_graph_conflict_records
    FOR EACH ROW EXECUTE FUNCTION public.hai_reject_knowledge_graph_history_mutation();
DROP TRIGGER IF EXISTS trg_knowledge_graph_conflicts_no_truncate
    ON public.knowledge_graph_conflict_records;
CREATE TRIGGER trg_knowledge_graph_conflicts_no_truncate
    BEFORE TRUNCATE ON public.knowledge_graph_conflict_records
    FOR EACH STATEMENT EXECUTE FUNCTION public.hai_reject_knowledge_graph_history_mutation();

DROP TRIGGER IF EXISTS trg_knowledge_graph_deletions_immutable
    ON public.knowledge_graph_deletion_signals;
CREATE TRIGGER trg_knowledge_graph_deletions_immutable
    BEFORE UPDATE OR DELETE ON public.knowledge_graph_deletion_signals
    FOR EACH ROW EXECUTE FUNCTION public.hai_reject_knowledge_graph_history_mutation();
DROP TRIGGER IF EXISTS trg_knowledge_graph_deletions_no_truncate
    ON public.knowledge_graph_deletion_signals;
CREATE TRIGGER trg_knowledge_graph_deletions_no_truncate
    BEFORE TRUNCATE ON public.knowledge_graph_deletion_signals
    FOR EACH STATEMENT EXECUTE FUNCTION public.hai_reject_knowledge_graph_history_mutation();
