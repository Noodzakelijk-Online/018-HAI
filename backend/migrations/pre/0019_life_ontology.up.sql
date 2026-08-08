CREATE TABLE public.life_ontology_entities (
    owner_identity character varying(256) NOT NULL,
    entity_id character varying(76) NOT NULL,
    entity_type character varying(32) NOT NULL,
    life_domain character varying(64) NOT NULL,
    lifecycle_status character varying(32) NOT NULL,
    verification_status character varying(32) NOT NULL,
    sensitivity character varying(32) NOT NULL,
    local_only boolean NOT NULL,
    priority integer NOT NULL,
    entity_digest character varying(64) NOT NULL,
    provenance_digest character varying(64) NOT NULL,
    valid_from timestamp with time zone NOT NULL,
    valid_until timestamp with time zone,
    observed_at timestamp with time zone NOT NULL,
    created_at timestamp with time zone NOT NULL,
    payload jsonb NOT NULL,
    CONSTRAINT life_ontology_entities_pkey
        PRIMARY KEY (owner_identity, entity_id),
    CONSTRAINT uq_life_ontology_entity_digest
        UNIQUE (owner_identity, entity_digest),
    CONSTRAINT chk_life_ontology_entity_id CHECK (
        entity_id = 'life-entity-' || entity_digest
        AND entity_digest ~ '^[0-9a-f]{64}$'
        AND provenance_digest ~ '^[0-9a-f]{64}$'
    ),
    CONSTRAINT chk_life_ontology_entity_type CHECK (
        entity_type IN (
            'person', 'need', 'goal', 'asset', 'obligation',
            'project', 'case', 'opportunity', 'risk'
        )
    ),
    CONSTRAINT chk_life_ontology_entity_domain CHECK (
        life_domain IN (
            'safety_security', 'health_wellbeing', 'relationships_care',
            'housing_assets', 'financial', 'work_venture',
            'learning_growth', 'meaning_values', 'community_civic',
            'legal_government', 'personal_administration'
        )
    ),
    CONSTRAINT chk_life_ontology_entity_status CHECK (
        lifecycle_status IN ('open', 'active', 'waiting', 'completed', 'archived', 'unknown')
    ),
    CONSTRAINT chk_life_ontology_entity_verification CHECK (
        verification_status IN (
            'unverified', 'source_supported', 'schema_validated',
            'human_approved', 'verified', 'uncertain', 'conflicting',
            'unsupported', 'needs_review'
        )
    ),
    CONSTRAINT chk_life_ontology_entity_sensitivity CHECK (
        sensitivity IN ('public', 'internal', 'sensitive', 'restricted')
    ),
    CONSTRAINT chk_life_ontology_entity_bounds CHECK (
        priority BETWEEN 0 AND 100
        AND (valid_until IS NULL OR valid_until > valid_from)
        AND observed_at <= created_at
    ),
    CONSTRAINT chk_life_ontology_entity_payload CHECK (
        jsonb_typeof(payload) = 'object'
        AND octet_length(payload::text) BETWEEN 2 AND 1048576
        AND payload ?& ARRAY[
            'id', 'ownerIdentity', 'type', 'domain', 'name', 'status',
            'priority', 'validFrom', 'observedAt', 'confidence',
            'verificationStatus', 'provenance', 'provenanceDigest',
            'sensitivity', 'localOnly', 'entityDigest', 'createdAt'
        ]
        AND payload #>> '{id}' = entity_id
        AND payload #>> '{ownerIdentity}' = owner_identity
        AND payload #>> '{type}' = entity_type
        AND payload #>> '{domain}' = life_domain
        AND payload #>> '{status}' = lifecycle_status
        AND (payload #>> '{priority}')::integer = priority
        AND payload #>> '{verificationStatus}' = verification_status
        AND payload #>> '{sensitivity}' = sensitivity
        AND (payload #>> '{localOnly}')::boolean = local_only
        AND payload #>> '{entityDigest}' = entity_digest
        AND payload #>> '{provenanceDigest}' = provenance_digest
        AND (payload #>> '{validFrom}')::timestamp with time zone = valid_from
        AND (payload #>> '{observedAt}')::timestamp with time zone = observed_at
        AND (payload #>> '{createdAt}')::timestamp with time zone = created_at
        AND (
            (valid_until IS NULL AND NOT (payload ? 'validUntil'))
            OR (payload #>> '{validUntil}')::timestamp with time zone = valid_until
        )
        AND jsonb_typeof(payload -> 'provenance') = 'array'
        AND jsonb_array_length(payload -> 'provenance') BETWEEN 1 AND 16
    )
);

CREATE INDEX idx_life_ontology_entities_owner_domain_status
    ON public.life_ontology_entities
    (owner_identity, life_domain, lifecycle_status, priority DESC, entity_id ASC);
CREATE INDEX idx_life_ontology_entities_owner_observed
    ON public.life_ontology_entities (owner_identity, observed_at DESC, entity_id ASC);

CREATE TABLE public.life_ontology_relations (
    owner_identity character varying(256) NOT NULL,
    relation_id character varying(78) NOT NULL,
    relation_type character varying(32) NOT NULL,
    from_entity_id character varying(76) NOT NULL,
    to_entity_id character varying(76) NOT NULL,
    verification_status character varying(32) NOT NULL,
    sensitivity character varying(32) NOT NULL,
    local_only boolean NOT NULL,
    relation_digest character varying(64) NOT NULL,
    provenance_digest character varying(64) NOT NULL,
    valid_from timestamp with time zone NOT NULL,
    valid_until timestamp with time zone,
    observed_at timestamp with time zone NOT NULL,
    created_at timestamp with time zone NOT NULL,
    payload jsonb NOT NULL,
    CONSTRAINT life_ontology_relations_pkey
        PRIMARY KEY (owner_identity, relation_id),
    CONSTRAINT uq_life_ontology_relation_digest
        UNIQUE (owner_identity, relation_digest),
    CONSTRAINT fk_life_ontology_relation_from
        FOREIGN KEY (owner_identity, from_entity_id)
        REFERENCES public.life_ontology_entities (owner_identity, entity_id)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT fk_life_ontology_relation_to
        FOREIGN KEY (owner_identity, to_entity_id)
        REFERENCES public.life_ontology_entities (owner_identity, entity_id)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT chk_life_ontology_relation_id CHECK (
        relation_id = 'life-relation-' || relation_digest
        AND relation_digest ~ '^[0-9a-f]{64}$'
        AND provenance_digest ~ '^[0-9a-f]{64}$'
    ),
    CONSTRAINT chk_life_ontology_relation_type CHECK (
        relation_type IN (
            'has_need', 'pursues_goal', 'owns_asset', 'owes_obligation',
            'advances', 'belongs_to_project', 'related_to_case',
            'creates_opportunity', 'threatens', 'mitigates',
            'depends_on', 'supports', 'conflicts_with'
        )
    ),
    CONSTRAINT chk_life_ontology_relation_verification CHECK (
        verification_status IN (
            'unverified', 'source_supported', 'schema_validated',
            'human_approved', 'verified', 'uncertain', 'conflicting',
            'unsupported', 'needs_review'
        )
    ),
    CONSTRAINT chk_life_ontology_relation_sensitivity CHECK (
        sensitivity IN ('public', 'internal', 'sensitive', 'restricted')
    ),
    CONSTRAINT chk_life_ontology_relation_bounds CHECK (
        from_entity_id <> to_entity_id
        AND (valid_until IS NULL OR valid_until > valid_from)
        AND observed_at <= created_at
    ),
    CONSTRAINT chk_life_ontology_relation_payload CHECK (
        jsonb_typeof(payload) = 'object'
        AND octet_length(payload::text) BETWEEN 2 AND 1048576
        AND payload ?& ARRAY[
            'id', 'ownerIdentity', 'type', 'fromEntityId', 'toEntityId',
            'validFrom', 'observedAt', 'confidence', 'verificationStatus',
            'provenance', 'provenanceDigest', 'sensitivity', 'localOnly',
            'relationDigest', 'createdAt'
        ]
        AND payload #>> '{id}' = relation_id
        AND payload #>> '{ownerIdentity}' = owner_identity
        AND payload #>> '{type}' = relation_type
        AND payload #>> '{fromEntityId}' = from_entity_id
        AND payload #>> '{toEntityId}' = to_entity_id
        AND payload #>> '{verificationStatus}' = verification_status
        AND payload #>> '{sensitivity}' = sensitivity
        AND (payload #>> '{localOnly}')::boolean = local_only
        AND payload #>> '{relationDigest}' = relation_digest
        AND payload #>> '{provenanceDigest}' = provenance_digest
        AND (payload #>> '{validFrom}')::timestamp with time zone = valid_from
        AND (payload #>> '{observedAt}')::timestamp with time zone = observed_at
        AND (payload #>> '{createdAt}')::timestamp with time zone = created_at
        AND (
            (valid_until IS NULL AND NOT (payload ? 'validUntil'))
            OR (payload #>> '{validUntil}')::timestamp with time zone = valid_until
        )
        AND jsonb_typeof(payload -> 'provenance') = 'array'
        AND jsonb_array_length(payload -> 'provenance') BETWEEN 1 AND 16
    )
);

CREATE INDEX idx_life_ontology_relations_owner_from
    ON public.life_ontology_relations
    (owner_identity, from_entity_id, relation_type, relation_id ASC);
CREATE INDEX idx_life_ontology_relations_owner_to
    ON public.life_ontology_relations
    (owner_identity, to_entity_id, relation_type, relation_id ASC);

CREATE TABLE public.life_ontology_merge_proposals (
    owner_identity character varying(256) NOT NULL,
    proposal_id character varying(75) NOT NULL,
    candidate_left_id character varying(76) NOT NULL,
    candidate_right_id character varying(76) NOT NULL,
    match_type character varying(32) NOT NULL,
    proposal_status character varying(32) NOT NULL,
    confidence double precision NOT NULL,
    proposal_digest character varying(64) NOT NULL,
    created_at timestamp with time zone NOT NULL,
    payload jsonb NOT NULL,
    CONSTRAINT life_ontology_merge_proposals_pkey
        PRIMARY KEY (owner_identity, proposal_id),
    CONSTRAINT uq_life_ontology_merge_proposal_digest
        UNIQUE (owner_identity, proposal_digest),
    CONSTRAINT fk_life_ontology_merge_candidate_left
        FOREIGN KEY (owner_identity, candidate_left_id)
        REFERENCES public.life_ontology_entities (owner_identity, entity_id)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT fk_life_ontology_merge_candidate_right
        FOREIGN KEY (owner_identity, candidate_right_id)
        REFERENCES public.life_ontology_entities (owner_identity, entity_id)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT chk_life_ontology_merge_identity CHECK (
        proposal_id = 'life-merge-' || proposal_digest
        AND proposal_digest ~ '^[0-9a-f]{64}$'
        AND candidate_left_id < candidate_right_id
    ),
    CONSTRAINT chk_life_ontology_merge_state CHECK (
        match_type IN ('external_key', 'semantic_identity')
        AND proposal_status = 'proposed'
        AND confidence BETWEEN 0 AND 1
    ),
    CONSTRAINT chk_life_ontology_merge_payload CHECK (
        jsonb_typeof(payload) = 'object'
        AND octet_length(payload::text) BETWEEN 2 AND 262144
        AND payload ?& ARRAY[
            'id', 'ownerIdentity', 'candidateEntityIds', 'match',
            'reasons', 'confidence', 'status', 'proposalDigest',
            'createdAt', 'advisoryOnly', 'canExecute', 'grantsAuthority'
        ]
        AND payload #>> '{id}' = proposal_id
        AND payload #>> '{ownerIdentity}' = owner_identity
        AND payload #>> '{candidateEntityIds,0}' = candidate_left_id
        AND payload #>> '{candidateEntityIds,1}' = candidate_right_id
        AND jsonb_array_length(payload -> 'candidateEntityIds') = 2
        AND payload #>> '{match}' = match_type
        AND payload #>> '{status}' = proposal_status
        AND (payload #>> '{confidence}')::double precision = confidence
        AND payload #>> '{proposalDigest}' = proposal_digest
        AND (payload #>> '{createdAt}')::timestamp with time zone = created_at
        AND payload #>> '{advisoryOnly}' = 'true'
        AND payload #>> '{canExecute}' = 'false'
        AND payload #>> '{grantsAuthority}' = 'false'
    )
);

CREATE INDEX idx_life_ontology_merge_proposals_owner_time
    ON public.life_ontology_merge_proposals
    (owner_identity, created_at DESC, proposal_id ASC);

CREATE OR REPLACE FUNCTION public.hai_reject_life_ontology_mutation()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE EXCEPTION 'life ontology records are append-only'
        USING ERRCODE = 'integrity_constraint_violation';
END;
$$;

CREATE TRIGGER trg_life_ontology_entities_immutable
BEFORE UPDATE OR DELETE ON public.life_ontology_entities
FOR EACH ROW EXECUTE FUNCTION public.hai_reject_life_ontology_mutation();
CREATE TRIGGER trg_life_ontology_relations_immutable
BEFORE UPDATE OR DELETE ON public.life_ontology_relations
FOR EACH ROW EXECUTE FUNCTION public.hai_reject_life_ontology_mutation();
CREATE TRIGGER trg_life_ontology_merge_proposals_immutable
BEFORE UPDATE OR DELETE ON public.life_ontology_merge_proposals
FOR EACH ROW EXECUTE FUNCTION public.hai_reject_life_ontology_mutation();

CREATE TRIGGER trg_life_ontology_entities_no_truncate
BEFORE TRUNCATE ON public.life_ontology_entities
FOR EACH STATEMENT EXECUTE FUNCTION public.hai_reject_life_ontology_mutation();
CREATE TRIGGER trg_life_ontology_relations_no_truncate
BEFORE TRUNCATE ON public.life_ontology_relations
FOR EACH STATEMENT EXECUTE FUNCTION public.hai_reject_life_ontology_mutation();
CREATE TRIGGER trg_life_ontology_merge_proposals_no_truncate
BEFORE TRUNCATE ON public.life_ontology_merge_proposals
FOR EACH STATEMENT EXECUTE FUNCTION public.hai_reject_life_ontology_mutation();
