CREATE TABLE IF NOT EXISTS pursuit_portfolio_execution_proposals (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    allocation_id UUID NOT NULL REFERENCES pursuit_portfolio_allocations(id) ON DELETE RESTRICT,
    owner_identity VARCHAR(255) NOT NULL,
    allocation_record_digest CHAR(64) NOT NULL,
    snapshot_digest CHAR(64) NOT NULL,
    status VARCHAR(40) NOT NULL,
    actor VARCHAR(255) NOT NULL,
    confirmation VARCHAR(255) NOT NULL,
    authority VARCHAR(32) NOT NULL,
    record_digest CHAR(64) NOT NULL,
    prepared_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT pursuit_portfolio_execution_proposals_snapshot_unique
        UNIQUE (owner_identity, allocation_id, snapshot_digest),
    CONSTRAINT pursuit_portfolio_execution_proposals_owner_check
        CHECK (length(btrim(owner_identity)) > 0),
    CONSTRAINT pursuit_portfolio_execution_proposals_digest_check CHECK (
        allocation_record_digest ~ '^[0-9a-f]{64}$'
        AND snapshot_digest ~ '^[0-9a-f]{64}$'
        AND record_digest ~ '^[0-9a-f]{64}$'
    ),
    CONSTRAINT pursuit_portfolio_execution_proposals_status_check CHECK (
        status IN ('prepared', 'prepared_needs_approval', 'prepared_blocked')
    ),
    CONSTRAINT pursuit_portfolio_execution_proposals_actor_check
        CHECK (length(btrim(actor)) > 0),
    CONSTRAINT pursuit_portfolio_execution_proposals_confirmation_check
        CHECK (confirmation = 'PREPARE EXECUTION PROPOSALS'),
    CONSTRAINT pursuit_portfolio_execution_proposals_authority_check
        CHECK (authority = 'proposal_only')
);

CREATE INDEX IF NOT EXISTS pursuit_portfolio_execution_proposals_owner_time_idx
    ON pursuit_portfolio_execution_proposals (owner_identity, prepared_at DESC, id DESC);

CREATE TABLE IF NOT EXISTS pursuit_portfolio_execution_proposal_items (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    proposal_id UUID NOT NULL REFERENCES pursuit_portfolio_execution_proposals(id) ON DELETE RESTRICT,
    allocation_item_id UUID NOT NULL REFERENCES pursuit_portfolio_allocation_items(id) ON DELETE RESTRICT,
    pursuit_id UUID NOT NULL REFERENCES pursuits(id) ON DELETE RESTRICT,
    reservation_id UUID NOT NULL REFERENCES pursuit_resource_reservations(id) ON DELETE RESTRICT,
    owner_identity VARCHAR(255) NOT NULL,
    action_summary TEXT NOT NULL,
    pursuit_status VARCHAR(80) NOT NULL,
    risk_level VARCHAR(80) NOT NULL,
    autonomy_level VARCHAR(80) NOT NULL,
    status VARCHAR(32) NOT NULL,
    requires_approval BOOLEAN NOT NULL DEFAULT FALSE,
    approval_reasons JSONB NOT NULL DEFAULT '[]'::jsonb,
    blocked_reasons JSONB NOT NULL DEFAULT '[]'::jsonb,
    allocation_item_digest CHAR(64) NOT NULL,
    state_digest CHAR(64) NOT NULL,
    record_digest CHAR(64) NOT NULL,
    prepared_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT pursuit_portfolio_execution_proposal_items_allocation_item_unique
        UNIQUE (proposal_id, allocation_item_id),
    CONSTRAINT pursuit_portfolio_execution_proposal_items_owner_check
        CHECK (length(btrim(owner_identity)) > 0),
    CONSTRAINT pursuit_portfolio_execution_proposal_items_action_check
        CHECK (length(btrim(action_summary)) > 0),
    CONSTRAINT pursuit_portfolio_execution_proposal_items_status_check CHECK (
        status IN ('proposed', 'needs_approval', 'blocked')
    ),
    CONSTRAINT pursuit_portfolio_execution_proposal_items_reason_check CHECK (
        jsonb_typeof(approval_reasons) = 'array'
        AND jsonb_typeof(blocked_reasons) = 'array'
        AND (
            (status = 'blocked' AND jsonb_array_length(blocked_reasons) > 0)
            OR (status <> 'blocked' AND jsonb_array_length(blocked_reasons) = 0)
        )
        AND (
            (requires_approval AND jsonb_array_length(approval_reasons) > 0)
            OR (NOT requires_approval AND jsonb_array_length(approval_reasons) = 0)
        )
    ),
    CONSTRAINT pursuit_portfolio_execution_proposal_items_digest_check CHECK (
        allocation_item_digest ~ '^[0-9a-f]{64}$'
        AND state_digest ~ '^[0-9a-f]{64}$'
        AND record_digest ~ '^[0-9a-f]{64}$'
    )
);

CREATE INDEX IF NOT EXISTS pursuit_portfolio_execution_proposal_items_owner_pursuit_idx
    ON pursuit_portfolio_execution_proposal_items (owner_identity, pursuit_id, prepared_at DESC, id DESC);

CREATE OR REPLACE FUNCTION validate_pursuit_portfolio_execution_proposal_insert()
RETURNS trigger AS $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pursuit_portfolio_allocations
        WHERE id = NEW.allocation_id
          AND owner_identity = NEW.owner_identity
          AND record_digest = NEW.allocation_record_digest
    ) THEN
        RAISE EXCEPTION 'execution proposal does not match its owner-scoped allocation evidence';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER pursuit_portfolio_execution_proposals_validate_insert
BEFORE INSERT ON pursuit_portfolio_execution_proposals
FOR EACH ROW EXECUTE FUNCTION validate_pursuit_portfolio_execution_proposal_insert();

CREATE OR REPLACE FUNCTION validate_pursuit_portfolio_execution_proposal_item_insert()
RETURNS trigger AS $$
DECLARE
    proposal_owner VARCHAR(255);
    proposal_allocation UUID;
BEGIN
    SELECT owner_identity, allocation_id
    INTO proposal_owner, proposal_allocation
    FROM pursuit_portfolio_execution_proposals
    WHERE id = NEW.proposal_id;

    IF NOT FOUND OR proposal_owner <> NEW.owner_identity THEN
        RAISE EXCEPTION 'execution proposal item does not match its owner-scoped proposal';
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM pursuit_portfolio_allocation_items
        WHERE id = NEW.allocation_item_id
          AND allocation_id = proposal_allocation
          AND pursuit_id = NEW.pursuit_id
          AND reservation_id = NEW.reservation_id
          AND owner_identity = NEW.owner_identity
          AND record_digest = NEW.allocation_item_digest
    ) THEN
        RAISE EXCEPTION 'execution proposal item does not match accepted allocation evidence';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER pursuit_portfolio_execution_proposal_items_validate_insert
BEFORE INSERT ON pursuit_portfolio_execution_proposal_items
FOR EACH ROW EXECUTE FUNCTION validate_pursuit_portfolio_execution_proposal_item_insert();

CREATE OR REPLACE FUNCTION reject_pursuit_portfolio_execution_proposal_mutation()
RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'pursuit portfolio execution proposals are append-only';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER pursuit_portfolio_execution_proposals_reject_update
BEFORE UPDATE ON pursuit_portfolio_execution_proposals
FOR EACH ROW EXECUTE FUNCTION reject_pursuit_portfolio_execution_proposal_mutation();
CREATE TRIGGER pursuit_portfolio_execution_proposals_reject_delete
BEFORE DELETE ON pursuit_portfolio_execution_proposals
FOR EACH ROW EXECUTE FUNCTION reject_pursuit_portfolio_execution_proposal_mutation();
CREATE TRIGGER pursuit_portfolio_execution_proposals_reject_truncate
BEFORE TRUNCATE ON pursuit_portfolio_execution_proposals
FOR EACH STATEMENT EXECUTE FUNCTION reject_pursuit_portfolio_execution_proposal_mutation();
CREATE TRIGGER pursuit_portfolio_execution_proposal_items_reject_update
BEFORE UPDATE ON pursuit_portfolio_execution_proposal_items
FOR EACH ROW EXECUTE FUNCTION reject_pursuit_portfolio_execution_proposal_mutation();
CREATE TRIGGER pursuit_portfolio_execution_proposal_items_reject_delete
BEFORE DELETE ON pursuit_portfolio_execution_proposal_items
FOR EACH ROW EXECUTE FUNCTION reject_pursuit_portfolio_execution_proposal_mutation();
CREATE TRIGGER pursuit_portfolio_execution_proposal_items_reject_truncate
BEFORE TRUNCATE ON pursuit_portfolio_execution_proposal_items
FOR EACH STATEMENT EXECUTE FUNCTION reject_pursuit_portfolio_execution_proposal_mutation();
