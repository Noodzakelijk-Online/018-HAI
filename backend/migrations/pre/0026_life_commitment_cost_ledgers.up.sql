CREATE TABLE IF NOT EXISTS public.life_ledger_commitment_revisions (
    owner_identity character varying(255) NOT NULL,
    commitment_key character varying(256) NOT NULL,
    revision bigint NOT NULL,
    idempotency_key character varying(255) NOT NULL,
    request_digest character(64) NOT NULL,
    record_digest character(64) NOT NULL,
    observed_at timestamp with time zone NOT NULL,
    recorded_at timestamp with time zone NOT NULL,
    payload jsonb NOT NULL,
    CONSTRAINT pk_life_ledger_commitment_revisions
        PRIMARY KEY (owner_identity, commitment_key, revision),
    CONSTRAINT uq_life_ledger_commitment_idempotency
        UNIQUE (owner_identity, idempotency_key),
    CONSTRAINT chk_life_ledger_commitment_revision_positive CHECK (revision > 0),
    CONSTRAINT chk_life_ledger_commitment_request_digest
        CHECK (request_digest ~ '^[0-9a-f]{64}$'),
    CONSTRAINT chk_life_ledger_commitment_record_digest
        CHECK (record_digest ~ '^[0-9a-f]{64}$'),
    CONSTRAINT chk_life_ledger_commitment_payload_object
        CHECK (jsonb_typeof(payload) = 'object'),
    CONSTRAINT chk_life_ledger_commitment_payload_contract
        CHECK (COALESCE(
            payload ->> 'contractVersion' = 'life-ledger.v1'
            AND payload ->> 'ownerIdentity' = owner_identity
            AND payload ->> 'commitmentKey' = commitment_key
            AND (payload ->> 'revision')::bigint = revision
            AND payload ->> 'idempotencyKey' = idempotency_key
            AND payload ->> 'requestDigest' = request_digest
            AND payload ->> 'recordDigest' = record_digest
            AND payload ->> 'localOnly' = 'true',
            FALSE
        ))
);

CREATE INDEX IF NOT EXISTS idx_life_ledger_commitment_owner_observed
    ON public.life_ledger_commitment_revisions (owner_identity, observed_at DESC);

CREATE TABLE IF NOT EXISTS public.life_ledger_cost_entries (
    id uuid NOT NULL,
    owner_identity character varying(255) NOT NULL,
    idempotency_key character varying(255) NOT NULL,
    request_digest character(64) NOT NULL,
    record_digest character(64) NOT NULL,
    kind character varying(32) NOT NULL,
    currency character(3) NOT NULL,
    amount_minor bigint NOT NULL,
    observed_at timestamp with time zone NOT NULL,
    recorded_at timestamp with time zone NOT NULL,
    payload jsonb NOT NULL,
    CONSTRAINT pk_life_ledger_cost_entries PRIMARY KEY (id),
    CONSTRAINT uq_life_ledger_cost_idempotency UNIQUE (owner_identity, idempotency_key),
    CONSTRAINT chk_life_ledger_cost_kind
        CHECK (kind IN ('estimate', 'incurred', 'paid', 'refund')),
    CONSTRAINT chk_life_ledger_cost_currency CHECK (currency ~ '^[A-Z]{3}$'),
    CONSTRAINT chk_life_ledger_cost_amount_positive CHECK (amount_minor > 0),
    CONSTRAINT chk_life_ledger_cost_request_digest
        CHECK (request_digest ~ '^[0-9a-f]{64}$'),
    CONSTRAINT chk_life_ledger_cost_record_digest
        CHECK (record_digest ~ '^[0-9a-f]{64}$'),
    CONSTRAINT chk_life_ledger_cost_payload_object CHECK (jsonb_typeof(payload) = 'object'),
    CONSTRAINT chk_life_ledger_cost_payload_contract
        CHECK (COALESCE(
            payload ->> 'contractVersion' = 'life-ledger.v1'
            AND payload ->> 'id' = id::text
            AND payload ->> 'ownerIdentity' = owner_identity
            AND payload ->> 'idempotencyKey' = idempotency_key
            AND payload ->> 'requestDigest' = request_digest
            AND payload ->> 'recordDigest' = record_digest
            AND payload ->> 'kind' = kind
            AND payload ->> 'currency' = currency
            AND (payload ->> 'amountMinor')::bigint = amount_minor
            AND payload ->> 'localOnly' = 'true',
            FALSE
        ))
);

CREATE INDEX IF NOT EXISTS idx_life_ledger_cost_owner_observed
    ON public.life_ledger_cost_entries (owner_identity, observed_at DESC);

CREATE OR REPLACE FUNCTION public.hai_reject_life_ledger_mutation()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE EXCEPTION 'life commitment and cost ledgers are append-only';
END;
$$;

DROP TRIGGER IF EXISTS trg_life_ledger_commitments_immutable
    ON public.life_ledger_commitment_revisions;
CREATE TRIGGER trg_life_ledger_commitments_immutable
    BEFORE UPDATE OR DELETE ON public.life_ledger_commitment_revisions
    FOR EACH ROW EXECUTE FUNCTION public.hai_reject_life_ledger_mutation();

DROP TRIGGER IF EXISTS trg_life_ledger_commitments_no_truncate
    ON public.life_ledger_commitment_revisions;
CREATE TRIGGER trg_life_ledger_commitments_no_truncate
    BEFORE TRUNCATE ON public.life_ledger_commitment_revisions
    FOR EACH STATEMENT EXECUTE FUNCTION public.hai_reject_life_ledger_mutation();

DROP TRIGGER IF EXISTS trg_life_ledger_costs_immutable
    ON public.life_ledger_cost_entries;
CREATE TRIGGER trg_life_ledger_costs_immutable
    BEFORE UPDATE OR DELETE ON public.life_ledger_cost_entries
    FOR EACH ROW EXECUTE FUNCTION public.hai_reject_life_ledger_mutation();

DROP TRIGGER IF EXISTS trg_life_ledger_costs_no_truncate
    ON public.life_ledger_cost_entries;
CREATE TRIGGER trg_life_ledger_costs_no_truncate
    BEFORE TRUNCATE ON public.life_ledger_cost_entries
    FOR EACH STATEMENT EXECUTE FUNCTION public.hai_reject_life_ledger_mutation();
