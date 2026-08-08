CREATE TABLE IF NOT EXISTS pursuit_resource_reservations (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    pursuit_id UUID NOT NULL REFERENCES pursuits(id) ON DELETE RESTRICT,
    owner_identity VARCHAR(255) NOT NULL,
    operation_id VARCHAR(160) NOT NULL,
    estimated_effort_minutes BIGINT NOT NULL DEFAULT 0,
    estimated_cost_micros BIGINT NOT NULL DEFAULT 0,
    currency VARCHAR(3) NOT NULL DEFAULT '',
    reason TEXT NOT NULL DEFAULT '',
    actor VARCHAR(255) NOT NULL,
    record_digest CHAR(64) NOT NULL,
    reserved_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT pursuit_resource_reservation_owner_check CHECK (length(btrim(owner_identity)) > 0),
    CONSTRAINT pursuit_resource_reservation_operation_check CHECK (length(btrim(operation_id)) > 0),
    CONSTRAINT pursuit_resource_reservation_actor_check CHECK (length(btrim(actor)) > 0),
    CONSTRAINT pursuit_resource_reservation_digest_check CHECK (record_digest ~ '^[0-9a-f]{64}$'),
    CONSTRAINT pursuit_resource_reservation_value_check CHECK (
        estimated_effort_minutes >= 0
        AND estimated_cost_micros >= 0
        AND (estimated_effort_minutes > 0 OR estimated_cost_micros > 0)
        AND ((estimated_cost_micros = 0 AND currency = '') OR (estimated_cost_micros > 0 AND currency = 'EUR'))
    )
);

CREATE UNIQUE INDEX IF NOT EXISTS pursuit_resource_reservation_operation_idx
    ON pursuit_resource_reservations (owner_identity, pursuit_id, operation_id);

CREATE INDEX IF NOT EXISTS pursuit_resource_reservation_scope_time_idx
    ON pursuit_resource_reservations (owner_identity, pursuit_id, reserved_at DESC);

CREATE TABLE IF NOT EXISTS pursuit_resource_reservation_settlements (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    reservation_id UUID NOT NULL UNIQUE REFERENCES pursuit_resource_reservations(id) ON DELETE RESTRICT,
    pursuit_id UUID NOT NULL REFERENCES pursuits(id) ON DELETE RESTRICT,
    owner_identity VARCHAR(255) NOT NULL,
    disposition VARCHAR(20) NOT NULL,
    actual_effort_minutes BIGINT NOT NULL DEFAULT 0,
    actual_cost_micros BIGINT NOT NULL DEFAULT 0,
    currency VARCHAR(3) NOT NULL DEFAULT '',
    evidence_uri VARCHAR(2048) NOT NULL DEFAULT '',
    actor VARCHAR(255) NOT NULL,
    record_digest CHAR(64) NOT NULL,
    settled_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT pursuit_resource_settlement_disposition_check CHECK (disposition IN ('consumed', 'released')),
    CONSTRAINT pursuit_resource_settlement_owner_check CHECK (length(btrim(owner_identity)) > 0),
    CONSTRAINT pursuit_resource_settlement_actor_check CHECK (length(btrim(actor)) > 0),
    CONSTRAINT pursuit_resource_settlement_digest_check CHECK (record_digest ~ '^[0-9a-f]{64}$'),
    CONSTRAINT pursuit_resource_settlement_value_check CHECK (
        actual_effort_minutes >= 0
        AND actual_cost_micros >= 0
        AND ((actual_cost_micros = 0 AND currency = '') OR (actual_cost_micros > 0 AND currency = 'EUR'))
        AND (disposition = 'consumed' OR (actual_effort_minutes = 0 AND actual_cost_micros = 0))
    )
);

CREATE INDEX IF NOT EXISTS pursuit_resource_settlement_scope_time_idx
    ON pursuit_resource_reservation_settlements (owner_identity, pursuit_id, settled_at DESC);

CREATE OR REPLACE FUNCTION validate_pursuit_resource_reservation_insert()
RETURNS trigger AS $$
DECLARE
    max_effort_minutes BIGINT;
    max_spend_micros BIGINT;
    recorded_effort BIGINT;
    recorded_spend_micros BIGINT;
    held_effort BIGINT;
    held_spend_micros BIGINT;
BEGIN
    PERFORM pg_advisory_xact_lock(hashtextextended(NEW.owner_identity || ':' || NEW.pursuit_id::text, 0));

    IF EXISTS (
        SELECT 1 FROM pursuit_resource_reservations
        WHERE owner_identity = NEW.owner_identity
          AND pursuit_id = NEW.pursuit_id
          AND operation_id = NEW.operation_id
    ) THEN
        RETURN NEW;
    END IF;

    SELECT
        ROUND(COALESCE(NULLIF(resource_limits->>'maxEffortHours', ''), '0')::numeric * 60)::bigint,
        ROUND(COALESCE(NULLIF(resource_limits->>'maxSpendEur', ''), '0')::numeric * 1000000)::bigint
    INTO max_effort_minutes, max_spend_micros
    FROM pursuits
    WHERE id = NEW.pursuit_id
      AND (owner_identity = NEW.owner_identity OR owner_identity = '' OR owner_identity IS NULL)
      AND archived = false
      AND lower(COALESCE(status, '')) <> 'completed'
      AND lower(COALESCE(completion_state, '')) <> 'verified'
      AND lower(COALESCE(source_of_creation, '')) NOT LIKE '%pursuit_candidate%';

    IF NOT FOUND THEN
        RAISE EXCEPTION 'pursuit is unavailable for resource reservation';
    END IF;

    SELECT
        COALESCE(SUM(CASE WHEN kind = 'effort_recorded' THEN effort_minutes ELSE 0 END), 0),
        GREATEST(COALESCE(SUM(CASE WHEN kind = 'spend_incurred' THEN amount_minor * 10000 WHEN kind = 'spend_refund' THEN -amount_minor * 10000 ELSE 0 END), 0), 0)
    INTO recorded_effort, recorded_spend_micros
    FROM pursuit_resource_events
    WHERE owner_identity = NEW.owner_identity AND pursuit_id = NEW.pursuit_id;

    SELECT
        COALESCE(SUM(r.estimated_effort_minutes), 0),
        COALESCE(SUM(r.estimated_cost_micros), 0)
    INTO held_effort, held_spend_micros
    FROM pursuit_resource_reservations r
    LEFT JOIN pursuit_resource_reservation_settlements s ON s.reservation_id = r.id
    WHERE r.owner_identity = NEW.owner_identity
      AND r.pursuit_id = NEW.pursuit_id
      AND s.id IS NULL;

    IF max_effort_minutes > 0 AND recorded_effort + held_effort + NEW.estimated_effort_minutes > max_effort_minutes THEN
        RAISE EXCEPTION 'pursuit effort reservation exceeds remaining ceiling';
    END IF;
    IF max_spend_micros > 0 AND recorded_spend_micros + held_spend_micros + NEW.estimated_cost_micros > max_spend_micros THEN
        RAISE EXCEPTION 'pursuit spend reservation exceeds remaining ceiling';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER pursuit_resource_reservations_validate_insert
BEFORE INSERT ON pursuit_resource_reservations
FOR EACH ROW EXECUTE FUNCTION validate_pursuit_resource_reservation_insert();

CREATE OR REPLACE FUNCTION validate_pursuit_resource_settlement_insert()
RETURNS trigger AS $$
BEGIN
    PERFORM pg_advisory_xact_lock(hashtextextended(NEW.owner_identity || ':' || NEW.pursuit_id::text, 0));
    IF NOT EXISTS (
        SELECT 1 FROM pursuit_resource_reservations
        WHERE id = NEW.reservation_id
          AND owner_identity = NEW.owner_identity
          AND pursuit_id = NEW.pursuit_id
    ) THEN
        RAISE EXCEPTION 'resource settlement does not match its reservation';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER pursuit_resource_reservation_settlements_validate_insert
BEFORE INSERT ON pursuit_resource_reservation_settlements
FOR EACH ROW EXECUTE FUNCTION validate_pursuit_resource_settlement_insert();

-- Replace the 0034 insert validator so direct accounting and reservations use
-- the same lock and cannot jointly oversubscribe a pursuit.
CREATE OR REPLACE FUNCTION validate_pursuit_resource_event_insert()
RETURNS trigger AS $$
DECLARE
    max_effort_minutes BIGINT;
    max_spend_minor BIGINT;
    recorded_effort BIGINT;
    recorded_spend BIGINT;
    held_effort BIGINT;
    held_spend_minor BIGINT;
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

    SELECT
        ROUND(COALESCE(NULLIF(resource_limits->>'maxEffortHours', ''), '0')::numeric * 60)::bigint,
        ROUND(COALESCE(NULLIF(resource_limits->>'maxSpendEur', ''), '0')::numeric * 100)::bigint
    INTO max_effort_minutes, max_spend_minor
    FROM pursuits
    WHERE id = NEW.pursuit_id
      AND (owner_identity = NEW.owner_identity OR owner_identity = '' OR owner_identity IS NULL)
      AND archived = false;

    IF NOT FOUND THEN
        RAISE EXCEPTION 'pursuit is unavailable for resource accounting';
    END IF;

    SELECT
        COALESCE(SUM(CASE WHEN kind = 'effort_recorded' THEN effort_minutes ELSE 0 END), 0),
        GREATEST(COALESCE(SUM(CASE WHEN kind = 'spend_incurred' THEN amount_minor WHEN kind = 'spend_refund' THEN -amount_minor ELSE 0 END), 0), 0)
    INTO recorded_effort, recorded_spend
    FROM pursuit_resource_events
    WHERE owner_identity = NEW.owner_identity AND pursuit_id = NEW.pursuit_id;

    SELECT
        COALESCE(SUM(r.estimated_effort_minutes), 0),
        CEIL(COALESCE(SUM(r.estimated_cost_micros), 0)::numeric / 10000)::bigint
    INTO held_effort, held_spend_minor
    FROM pursuit_resource_reservations r
    LEFT JOIN pursuit_resource_reservation_settlements s ON s.reservation_id = r.id
    WHERE r.owner_identity = NEW.owner_identity
      AND r.pursuit_id = NEW.pursuit_id
      AND s.id IS NULL;

    IF NEW.kind = 'effort_recorded'
       AND max_effort_minutes > 0
       AND recorded_effort + held_effort + NEW.effort_minutes > max_effort_minutes THEN
        RAISE EXCEPTION 'pursuit effort accounting exceeds remaining ceiling';
    END IF;
    IF NEW.kind = 'spend_incurred'
       AND max_spend_minor > 0
       AND recorded_spend + held_spend_minor + NEW.amount_minor > max_spend_minor THEN
        RAISE EXCEPTION 'pursuit spend accounting exceeds remaining ceiling';
    END IF;
    IF NEW.kind = 'spend_refund' AND NEW.amount_minor > recorded_spend THEN
        RAISE EXCEPTION 'pursuit resource refund exceeds recorded net spend';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION reject_pursuit_resource_reservation_mutation()
RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'pursuit resource reservations and settlements are append-only';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER pursuit_resource_reservations_reject_update
BEFORE UPDATE ON pursuit_resource_reservations
FOR EACH ROW EXECUTE FUNCTION reject_pursuit_resource_reservation_mutation();
CREATE TRIGGER pursuit_resource_reservations_reject_delete
BEFORE DELETE ON pursuit_resource_reservations
FOR EACH ROW EXECUTE FUNCTION reject_pursuit_resource_reservation_mutation();
CREATE TRIGGER pursuit_resource_reservations_reject_truncate
BEFORE TRUNCATE ON pursuit_resource_reservations
FOR EACH STATEMENT EXECUTE FUNCTION reject_pursuit_resource_reservation_mutation();
CREATE TRIGGER pursuit_resource_reservation_settlements_reject_update
BEFORE UPDATE ON pursuit_resource_reservation_settlements
FOR EACH ROW EXECUTE FUNCTION reject_pursuit_resource_reservation_mutation();
CREATE TRIGGER pursuit_resource_reservation_settlements_reject_delete
BEFORE DELETE ON pursuit_resource_reservation_settlements
FOR EACH ROW EXECUTE FUNCTION reject_pursuit_resource_reservation_mutation();
CREATE TRIGGER pursuit_resource_reservation_settlements_reject_truncate
BEFORE TRUNCATE ON pursuit_resource_reservation_settlements
FOR EACH STATEMENT EXECUTE FUNCTION reject_pursuit_resource_reservation_mutation();
