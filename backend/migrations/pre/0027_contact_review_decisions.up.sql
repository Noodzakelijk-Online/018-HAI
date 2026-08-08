CREATE TABLE public.life_ontology_contact_review_decisions (
    owner_identity character varying(256) NOT NULL,
    decision_id character varying(96) NOT NULL,
    idempotency_key character varying(128) NOT NULL,
    subject_kind character varying(32) NOT NULL,
    subject_id character varying(128) NOT NULL,
    action character varying(32) NOT NULL,
    candidate_left_id character varying(76) NOT NULL,
    candidate_right_id character varying(76),
    merge_proposal_id character varying(75),
    canonical_entity_id character varying(76),
    request_digest character(64) NOT NULL,
    record_digest character(64) NOT NULL,
    decided_at timestamp with time zone NOT NULL,
    recorded_at timestamp with time zone NOT NULL,
    payload jsonb NOT NULL,
    CONSTRAINT life_ontology_contact_review_decisions_pkey
        PRIMARY KEY (owner_identity, decision_id),
    CONSTRAINT uq_life_ontology_contact_review_idempotency
        UNIQUE (owner_identity, idempotency_key),
    CONSTRAINT uq_life_ontology_contact_review_subject
        UNIQUE (owner_identity, subject_kind, subject_id),
    CONSTRAINT fk_life_ontology_contact_review_canonical
        FOREIGN KEY (owner_identity, canonical_entity_id)
        REFERENCES public.life_ontology_entities (owner_identity, entity_id)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT fk_life_ontology_contact_review_candidate_left
        FOREIGN KEY (owner_identity, candidate_left_id)
        REFERENCES public.life_ontology_entities (owner_identity, entity_id)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT fk_life_ontology_contact_review_candidate_right
        FOREIGN KEY (owner_identity, candidate_right_id)
        REFERENCES public.life_ontology_entities (owner_identity, entity_id)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT fk_life_ontology_contact_review_merge_proposal
        FOREIGN KEY (owner_identity, merge_proposal_id)
        REFERENCES public.life_ontology_merge_proposals (owner_identity, proposal_id)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT chk_life_ontology_contact_review_identity CHECK (
        decision_id = 'life-contact-review-' || record_digest
        AND request_digest ~ '^[0-9a-f]{64}$'
        AND record_digest ~ '^[0-9a-f]{64}$'
    ),
    CONSTRAINT chk_life_ontology_contact_review_action CHECK (
        (subject_kind = 'candidate'
            AND action IN ('promote', 'correct', 'reject')
            AND subject_id = candidate_left_id
            AND candidate_right_id IS NULL
            AND merge_proposal_id IS NULL)
        OR
        (subject_kind = 'merge_proposal'
            AND action IN ('merge', 'keep_distinct', 'reject')
            AND subject_id = merge_proposal_id
            AND candidate_right_id IS NOT NULL
            AND candidate_left_id < candidate_right_id)
    ),
    CONSTRAINT chk_life_ontology_contact_review_canonical CHECK (
        (action IN ('promote', 'correct', 'merge') AND canonical_entity_id IS NOT NULL)
        OR
        (action IN ('reject', 'keep_distinct') AND canonical_entity_id IS NULL)
    ),
    CONSTRAINT chk_life_ontology_contact_review_time CHECK (
        recorded_at >= decided_at
    ),
    CONSTRAINT chk_life_ontology_contact_review_payload CHECK (
        jsonb_typeof(payload) = 'object'
        AND octet_length(payload::text) BETWEEN 2 AND 262144
        AND payload ?& ARRAY[
            'contractVersion', 'id', 'ownerIdentity', 'idempotencyKey',
            'subject', 'subjectId', 'action', 'candidateEntityIds',
            'reason', 'decidedAt', 'recordedAt', 'requestDigest',
            'recordDigest', 'localOnly', 'canExecute', 'grantsAuthority'
        ]
        AND payload #>> '{contractVersion}' = 'life-contact-review.v1'
        AND payload #>> '{id}' = decision_id
        AND payload #>> '{ownerIdentity}' = owner_identity
        AND payload #>> '{idempotencyKey}' = idempotency_key
        AND payload #>> '{subject}' = subject_kind
        AND payload #>> '{subjectId}' = subject_id
        AND payload #>> '{action}' = action
        AND payload #>> '{candidateEntityIds,0}' = candidate_left_id
        AND (
            (candidate_right_id IS NULL AND jsonb_array_length(payload -> 'candidateEntityIds') = 1)
            OR (
                candidate_right_id IS NOT NULL
                AND jsonb_array_length(payload -> 'candidateEntityIds') = 2
                AND payload #>> '{candidateEntityIds,1}' = candidate_right_id
            )
        )
        AND payload #>> '{requestDigest}' = request_digest
        AND payload #>> '{recordDigest}' = record_digest
        AND (payload #>> '{decidedAt}')::timestamp with time zone = decided_at
        AND (payload #>> '{recordedAt}')::timestamp with time zone = recorded_at
        AND payload #>> '{localOnly}' = 'true'
        AND payload #>> '{canExecute}' = 'false'
        AND payload #>> '{grantsAuthority}' = 'false'
        AND (
            (canonical_entity_id IS NULL AND NOT (payload ? 'canonicalEntityId'))
            OR payload #>> '{canonicalEntityId}' = canonical_entity_id
        )
    )
);

CREATE INDEX idx_life_ontology_contact_review_owner_time
    ON public.life_ontology_contact_review_decisions
    (owner_identity, recorded_at DESC, decision_id ASC);

CREATE FUNCTION public.hai_validate_contact_review_decision()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    IF NEW.subject_kind = 'merge_proposal' AND NOT EXISTS (
        SELECT 1
        FROM public.life_ontology_merge_proposals proposal
        WHERE proposal.owner_identity = NEW.owner_identity
          AND proposal.proposal_id = NEW.merge_proposal_id
          AND proposal.candidate_left_id = NEW.candidate_left_id
          AND proposal.candidate_right_id = NEW.candidate_right_id
    ) THEN
        RAISE EXCEPTION 'contact review candidates do not match merge proposal'
            USING ERRCODE = 'integrity_constraint_violation';
    END IF;

    IF NEW.canonical_entity_id IS NOT NULL AND NOT EXISTS (
        SELECT 1
        FROM public.life_ontology_entities entity
        WHERE entity.owner_identity = NEW.owner_identity
          AND entity.entity_id = NEW.canonical_entity_id
          AND entity.entity_type = 'person'
          AND entity.verification_status = 'human_approved'
          AND entity.local_only = TRUE
          AND entity.payload #>> '{attributes,canonical}' = 'true'
    ) THEN
        RAISE EXCEPTION 'contact review canonical entity is not a human-approved local person'
            USING ERRCODE = 'integrity_constraint_violation';
    END IF;

    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_life_ontology_contact_review_validate
BEFORE INSERT ON public.life_ontology_contact_review_decisions
FOR EACH ROW EXECUTE FUNCTION public.hai_validate_contact_review_decision();

CREATE TRIGGER trg_life_ontology_contact_review_immutable
BEFORE UPDATE OR DELETE ON public.life_ontology_contact_review_decisions
FOR EACH ROW EXECUTE FUNCTION public.hai_reject_life_ontology_mutation();

CREATE TRIGGER trg_life_ontology_contact_review_no_truncate
BEFORE TRUNCATE ON public.life_ontology_contact_review_decisions
FOR EACH STATEMENT EXECUTE FUNCTION public.hai_reject_life_ontology_mutation();
