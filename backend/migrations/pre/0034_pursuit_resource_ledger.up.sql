CREATE TABLE IF NOT EXISTS pursuit_resource_events (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    pursuit_id UUID NOT NULL REFERENCES pursuits(id) ON DELETE RESTRICT,
    owner_identity VARCHAR(255) NOT NULL,
    kind VARCHAR(40) NOT NULL,
    effort_minutes BIGINT NOT NULL DEFAULT 0,
    amount_minor BIGINT NOT NULL DEFAULT 0,
    currency VARCHAR(3) NOT NULL DEFAULT '',
    note TEXT NOT NULL DEFAULT '',
    evidence_uri VARCHAR(2048) NOT NULL DEFAULT '',
    actor VARCHAR(255) NOT NULL,
    idempotency_key VARCHAR(120) NOT NULL,
    record_digest CHAR(64) NOT NULL,
    occurred_at TIMESTAMPTZ NOT NULL,
    recorded_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT pursuit_resource_events_kind_check CHECK (
        kind IN ('effort_recorded', 'spend_incurred', 'spend_refund')
    ),
    CONSTRAINT pursuit_resource_events_owner_check CHECK (length(btrim(owner_identity)) > 0),
    CONSTRAINT pursuit_resource_events_actor_check CHECK (length(btrim(actor)) > 0),
    CONSTRAINT pursuit_resource_events_idempotency_check CHECK (length(btrim(idempotency_key)) > 0),
    CONSTRAINT pursuit_resource_events_digest_check CHECK (record_digest ~ '^[0-9a-f]{64}$'),
    CONSTRAINT pursuit_resource_events_value_check CHECK (
        (kind = 'effort_recorded' AND effort_minutes > 0 AND amount_minor = 0 AND currency = '')
        OR
        (kind IN ('spend_incurred', 'spend_refund') AND effort_minutes = 0 AND amount_minor > 0 AND currency = 'EUR')
    )
);

CREATE UNIQUE INDEX IF NOT EXISTS pursuit_resource_events_owner_idempotency_idx
    ON pursuit_resource_events (owner_identity, pursuit_id, idempotency_key);

CREATE INDEX IF NOT EXISTS pursuit_resource_events_owner_pursuit_time_idx
    ON pursuit_resource_events (owner_identity, pursuit_id, occurred_at DESC, recorded_at DESC);

CREATE OR REPLACE FUNCTION validate_pursuit_resource_event_insert()
RETURNS trigger AS $$
DECLARE
    current_net_minor BIGINT;
BEGIN
    PERFORM pg_advisory_xact_lock(hashtextextended(NEW.owner_identity || ':' || NEW.pursuit_id::text, 0));
    IF EXISTS (
        SELECT 1 FROM pursuit_resource_events
        WHERE owner_identity = NEW.owner_identity
          AND pursuit_id = NEW.pursuit_id
          AND idempotency_key = NEW.idempotency_key
    ) THEN
        RETURN NEW;
    END IF;
    IF NEW.kind = 'spend_refund' THEN
        SELECT COALESCE(SUM(CASE
            WHEN kind = 'spend_incurred' THEN amount_minor
            WHEN kind = 'spend_refund' THEN -amount_minor
            ELSE 0
        END), 0)
        INTO current_net_minor
        FROM pursuit_resource_events
        WHERE owner_identity = NEW.owner_identity AND pursuit_id = NEW.pursuit_id;

        IF NEW.amount_minor > current_net_minor THEN
            RAISE EXCEPTION 'pursuit resource refund exceeds recorded net spend';
        END IF;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS pursuit_resource_events_validate_insert ON pursuit_resource_events;
CREATE TRIGGER pursuit_resource_events_validate_insert
BEFORE INSERT ON pursuit_resource_events
FOR EACH ROW EXECUTE FUNCTION validate_pursuit_resource_event_insert();

CREATE OR REPLACE FUNCTION reject_pursuit_resource_event_mutation()
RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'pursuit resource events are append-only';
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS pursuit_resource_events_reject_update ON pursuit_resource_events;
CREATE TRIGGER pursuit_resource_events_reject_update
BEFORE UPDATE ON pursuit_resource_events
FOR EACH ROW EXECUTE FUNCTION reject_pursuit_resource_event_mutation();

DROP TRIGGER IF EXISTS pursuit_resource_events_reject_delete ON pursuit_resource_events;
CREATE TRIGGER pursuit_resource_events_reject_delete
BEFORE DELETE ON pursuit_resource_events
FOR EACH ROW EXECUTE FUNCTION reject_pursuit_resource_event_mutation();

DROP TRIGGER IF EXISTS pursuit_resource_events_reject_truncate ON pursuit_resource_events;
CREATE TRIGGER pursuit_resource_events_reject_truncate
BEFORE TRUNCATE ON pursuit_resource_events
FOR EACH STATEMENT EXECUTE FUNCTION reject_pursuit_resource_event_mutation();
