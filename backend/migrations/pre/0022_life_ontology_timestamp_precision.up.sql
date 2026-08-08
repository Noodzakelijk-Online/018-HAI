CREATE OR REPLACE FUNCTION public.hai_life_ontology_timestamp_matches(
    payload_value text,
    stored_value timestamp with time zone
)
RETURNS boolean
LANGUAGE sql
IMMUTABLE
STRICT
AS $$
    SELECT abs(extract(epoch FROM (payload_value::timestamp with time zone - stored_value))) <= 0.000001;
$$;

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
        AND public.hai_life_ontology_timestamp_matches(payload #>> '{validFrom}', valid_from)
        AND public.hai_life_ontology_timestamp_matches(payload #>> '{observedAt}', observed_at)
        AND public.hai_life_ontology_timestamp_matches(payload #>> '{createdAt}', created_at)
        AND (
            (valid_until IS NULL AND NOT (payload ? 'validUntil'))
            OR public.hai_life_ontology_timestamp_matches(payload #>> '{validUntil}', valid_until)
        )
        AND jsonb_typeof(payload -> 'provenance') = 'array'
        AND jsonb_array_length(payload -> 'provenance') BETWEEN 1 AND 16
    );

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
        AND public.hai_life_ontology_timestamp_matches(payload #>> '{validFrom}', valid_from)
        AND public.hai_life_ontology_timestamp_matches(payload #>> '{observedAt}', observed_at)
        AND public.hai_life_ontology_timestamp_matches(payload #>> '{createdAt}', created_at)
        AND (
            (valid_until IS NULL AND NOT (payload ? 'validUntil'))
            OR public.hai_life_ontology_timestamp_matches(payload #>> '{validUntil}', valid_until)
        )
        AND jsonb_typeof(payload -> 'provenance') = 'array'
        AND jsonb_array_length(payload -> 'provenance') BETWEEN 1 AND 16
    );
