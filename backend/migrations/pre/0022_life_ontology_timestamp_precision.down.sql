DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM public.life_ontology_entities
        WHERE (payload #>> '{validFrom}')::timestamp with time zone <> valid_from
           OR (payload #>> '{observedAt}')::timestamp with time zone <> observed_at
           OR (payload #>> '{createdAt}')::timestamp with time zone <> created_at
           OR (valid_until IS NOT NULL AND (payload #>> '{validUntil}')::timestamp with time zone <> valid_until)
    ) OR EXISTS (
        SELECT 1 FROM public.life_ontology_relations
        WHERE (payload #>> '{validFrom}')::timestamp with time zone <> valid_from
           OR (payload #>> '{observedAt}')::timestamp with time zone <> observed_at
           OR (payload #>> '{createdAt}')::timestamp with time zone <> created_at
           OR (valid_until IS NOT NULL AND (payload #>> '{validUntil}')::timestamp with time zone <> valid_until)
    ) THEN
        RAISE EXCEPTION 'refusing to restore exact timestamp checks while precision-normalized ontology records exist';
    END IF;
END;
$$;

ALTER TABLE public.life_ontology_relations
    DROP CONSTRAINT chk_life_ontology_relation_payload,
    ADD CONSTRAINT chk_life_ontology_relation_payload CHECK (
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
    );

ALTER TABLE public.life_ontology_entities
    DROP CONSTRAINT chk_life_ontology_entity_payload,
    ADD CONSTRAINT chk_life_ontology_entity_payload CHECK (
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
    );

DROP FUNCTION IF EXISTS public.hai_life_ontology_timestamp_matches(text, timestamp with time zone);
