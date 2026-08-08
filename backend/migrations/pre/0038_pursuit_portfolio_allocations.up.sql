CREATE TABLE IF NOT EXISTS pursuit_portfolio_allocations (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    owner_identity VARCHAR(255) NOT NULL,
    plan_id VARCHAR(96) NOT NULL,
    request_digest CHAR(64) NOT NULL,
    decision_digest CHAR(64) NOT NULL,
    status VARCHAR(32) NOT NULL,
    duration_mode VARCHAR(16) NOT NULL,
    horizon_start TIMESTAMPTZ NOT NULL,
    horizon_end TIMESTAMPTZ NOT NULL,
    actor VARCHAR(255) NOT NULL,
    confirmation VARCHAR(255) NOT NULL,
    record_digest CHAR(64) NOT NULL,
    accepted_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT pursuit_portfolio_allocations_owner_plan_unique UNIQUE (owner_identity, plan_id),
    CONSTRAINT pursuit_portfolio_allocations_owner_check CHECK (length(btrim(owner_identity)) > 0),
    CONSTRAINT pursuit_portfolio_allocations_plan_check CHECK (
        plan_id ~ '^[A-Za-z0-9._-]{1,96}$'
    ),
    CONSTRAINT pursuit_portfolio_allocations_digest_check CHECK (
        request_digest ~ '^[0-9a-f]{64}$'
        AND decision_digest ~ '^[0-9a-f]{64}$'
        AND record_digest ~ '^[0-9a-f]{64}$'
    ),
    CONSTRAINT pursuit_portfolio_allocations_status_check CHECK (
        status IN ('accepted', 'accepted_needs_approval')
    ),
    CONSTRAINT pursuit_portfolio_allocations_duration_mode_check CHECK (
        duration_mode IN ('expected', 'conservative')
    ),
    CONSTRAINT pursuit_portfolio_allocations_horizon_check CHECK (horizon_end > horizon_start),
    CONSTRAINT pursuit_portfolio_allocations_actor_check CHECK (length(btrim(actor)) > 0),
    CONSTRAINT pursuit_portfolio_allocations_confirmation_check CHECK (length(btrim(confirmation)) > 0)
);

CREATE INDEX IF NOT EXISTS pursuit_portfolio_allocations_owner_time_idx
    ON pursuit_portfolio_allocations (owner_identity, accepted_at DESC, id DESC);

CREATE TABLE IF NOT EXISTS pursuit_portfolio_allocation_items (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    allocation_id UUID NOT NULL REFERENCES pursuit_portfolio_allocations(id) ON DELETE RESTRICT,
    pursuit_id UUID NOT NULL REFERENCES pursuits(id) ON DELETE RESTRICT,
    owner_identity VARCHAR(255) NOT NULL,
    scheduled_start TIMESTAMPTZ NOT NULL,
    scheduled_end TIMESTAMPTZ NOT NULL,
    duration_minutes BIGINT NOT NULL,
    estimated_cost_micros BIGINT NOT NULL DEFAULT 0,
    requires_approval BOOLEAN NOT NULL DEFAULT FALSE,
    approval_reasons JSONB NOT NULL DEFAULT '[]'::jsonb,
    reservation_id UUID NOT NULL UNIQUE REFERENCES pursuit_resource_reservations(id) ON DELETE RESTRICT,
    record_digest CHAR(64) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT pursuit_portfolio_allocation_items_allocation_pursuit_unique UNIQUE (allocation_id, pursuit_id),
    CONSTRAINT pursuit_portfolio_allocation_items_owner_check CHECK (length(btrim(owner_identity)) > 0),
    CONSTRAINT pursuit_portfolio_allocation_items_schedule_check CHECK (
        duration_minutes > 0
        AND scheduled_end > scheduled_start
        AND scheduled_end = scheduled_start + duration_minutes * INTERVAL '1 minute'
    ),
    CONSTRAINT pursuit_portfolio_allocation_items_cost_check CHECK (estimated_cost_micros >= 0),
    CONSTRAINT pursuit_portfolio_allocation_items_approval_check CHECK (
        jsonb_typeof(approval_reasons) = 'array'
        AND (
            (requires_approval AND jsonb_array_length(approval_reasons) > 0)
            OR
            (NOT requires_approval AND jsonb_array_length(approval_reasons) = 0)
        )
    ),
    CONSTRAINT pursuit_portfolio_allocation_items_digest_check CHECK (record_digest ~ '^[0-9a-f]{64}$')
);

CREATE INDEX IF NOT EXISTS pursuit_portfolio_allocation_items_owner_pursuit_idx
    ON pursuit_portfolio_allocation_items (owner_identity, pursuit_id, created_at DESC, id DESC);

CREATE OR REPLACE FUNCTION validate_pursuit_portfolio_allocation_item_insert()
RETURNS trigger AS $$
DECLARE
    allocation_horizon_start TIMESTAMPTZ;
    allocation_horizon_end TIMESTAMPTZ;
BEGIN
    SELECT horizon_start, horizon_end
    INTO allocation_horizon_start, allocation_horizon_end
    FROM pursuit_portfolio_allocations
    WHERE id = NEW.allocation_id AND owner_identity = NEW.owner_identity;

    IF NOT FOUND THEN
        RAISE EXCEPTION 'portfolio allocation item does not match its owner-scoped allocation';
    END IF;
    IF NEW.scheduled_start < allocation_horizon_start OR NEW.scheduled_end > allocation_horizon_end THEN
        RAISE EXCEPTION 'portfolio allocation item is outside its accepted horizon';
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM pursuits
        WHERE id = NEW.pursuit_id AND owner_identity = NEW.owner_identity
    ) THEN
        RAISE EXCEPTION 'portfolio allocation pursuit is unavailable to this owner';
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM pursuit_resource_reservations
        WHERE id = NEW.reservation_id
          AND pursuit_id = NEW.pursuit_id
          AND owner_identity = NEW.owner_identity
          AND estimated_effort_minutes = NEW.duration_minutes
          AND estimated_cost_micros = NEW.estimated_cost_micros
    ) THEN
        RAISE EXCEPTION 'portfolio allocation item does not match its resource reservation';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS pursuit_portfolio_allocation_items_validate_insert
    ON pursuit_portfolio_allocation_items;
CREATE TRIGGER pursuit_portfolio_allocation_items_validate_insert
BEFORE INSERT ON pursuit_portfolio_allocation_items
FOR EACH ROW EXECUTE FUNCTION validate_pursuit_portfolio_allocation_item_insert();

CREATE OR REPLACE FUNCTION reject_pursuit_portfolio_allocation_mutation()
RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'pursuit portfolio allocations are append-only';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER pursuit_portfolio_allocations_reject_update
BEFORE UPDATE ON pursuit_portfolio_allocations
FOR EACH ROW EXECUTE FUNCTION reject_pursuit_portfolio_allocation_mutation();
CREATE TRIGGER pursuit_portfolio_allocations_reject_delete
BEFORE DELETE ON pursuit_portfolio_allocations
FOR EACH ROW EXECUTE FUNCTION reject_pursuit_portfolio_allocation_mutation();
CREATE TRIGGER pursuit_portfolio_allocations_reject_truncate
BEFORE TRUNCATE ON pursuit_portfolio_allocations
FOR EACH STATEMENT EXECUTE FUNCTION reject_pursuit_portfolio_allocation_mutation();
CREATE TRIGGER pursuit_portfolio_allocation_items_reject_update
BEFORE UPDATE ON pursuit_portfolio_allocation_items
FOR EACH ROW EXECUTE FUNCTION reject_pursuit_portfolio_allocation_mutation();
CREATE TRIGGER pursuit_portfolio_allocation_items_reject_delete
BEFORE DELETE ON pursuit_portfolio_allocation_items
FOR EACH ROW EXECUTE FUNCTION reject_pursuit_portfolio_allocation_mutation();
CREATE TRIGGER pursuit_portfolio_allocation_items_reject_truncate
BEFORE TRUNCATE ON pursuit_portfolio_allocation_items
FOR EACH STATEMENT EXECUTE FUNCTION reject_pursuit_portfolio_allocation_mutation();
