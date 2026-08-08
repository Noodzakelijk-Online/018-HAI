CREATE TABLE IF NOT EXISTS pursuit_portfolio_execution_proposal_decisions (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    proposal_item_id UUID NOT NULL REFERENCES pursuit_portfolio_execution_proposal_items(id) ON DELETE RESTRICT,
    proposal_id UUID NOT NULL REFERENCES pursuit_portfolio_execution_proposals(id) ON DELETE RESTRICT,
    pursuit_id UUID NOT NULL REFERENCES pursuits(id) ON DELETE RESTRICT,
    owner_identity VARCHAR(255) NOT NULL,
    decision VARCHAR(40) NOT NULL,
    reason TEXT NOT NULL,
    actor VARCHAR(255) NOT NULL,
    confirmation VARCHAR(255) NOT NULL,
    proposal_item_digest CHAR(64) NOT NULL,
    state_digest CHAR(64) NOT NULL,
    authority VARCHAR(40) NOT NULL,
    request_digest CHAR(64) NOT NULL,
    record_digest CHAR(64) NOT NULL,
    previous_decision_id UUID REFERENCES pursuit_portfolio_execution_proposal_decisions(id) ON DELETE RESTRICT,
    decided_at TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ,
    CONSTRAINT pursuit_portfolio_execution_proposal_decisions_owner_check
        CHECK (length(btrim(owner_identity)) > 0),
    CONSTRAINT pursuit_portfolio_execution_proposal_decisions_decision_check
        CHECK (decision IN ('approved', 'rejected', 'needs_clarification', 'revoked')),
    CONSTRAINT pursuit_portfolio_execution_proposal_decisions_reason_check
        CHECK (length(btrim(reason)) > 0 AND length(reason) <= 2000),
    CONSTRAINT pursuit_portfolio_execution_proposal_decisions_actor_check
        CHECK (length(btrim(actor)) > 0),
    CONSTRAINT pursuit_portfolio_execution_proposal_decisions_confirmation_check CHECK (
        (decision = 'approved' AND confirmation = 'APPROVE EXECUTION PROPOSAL ITEM')
        OR (decision = 'rejected' AND confirmation = 'REJECT EXECUTION PROPOSAL ITEM')
        OR (decision = 'needs_clarification' AND confirmation = 'REQUEST CLARIFICATION FOR EXECUTION PROPOSAL ITEM')
        OR (decision = 'revoked' AND confirmation = 'REVOKE EXECUTION PROPOSAL ITEM')
    ),
    CONSTRAINT pursuit_portfolio_execution_proposal_decisions_digest_check CHECK (
        proposal_item_digest ~ '^[0-9a-f]{64}$'
        AND state_digest ~ '^[0-9a-f]{64}$'
        AND request_digest ~ '^[0-9a-f]{64}$'
        AND record_digest ~ '^[0-9a-f]{64}$'
    ),
    CONSTRAINT pursuit_portfolio_execution_proposal_decisions_authority_check
        CHECK (authority = 'approval_decision_only'),
    CONSTRAINT pursuit_portfolio_execution_proposal_decisions_expiry_check CHECK (
        (decision = 'approved' AND expires_at IS NOT NULL AND expires_at > decided_at)
        OR (decision <> 'approved' AND expires_at IS NULL)
    ),
    CONSTRAINT pursuit_portfolio_execution_proposal_decisions_previous_check
        CHECK (previous_decision_id IS NULL OR previous_decision_id <> id)
);

CREATE INDEX IF NOT EXISTS pursuit_portfolio_execution_proposal_decisions_owner_item_time_idx
    ON pursuit_portfolio_execution_proposal_decisions
    (owner_identity, proposal_item_id, decided_at DESC, id DESC);

CREATE OR REPLACE FUNCTION validate_pursuit_portfolio_execution_proposal_decision_insert()
RETURNS trigger AS $$
DECLARE
    item_proposal UUID;
    item_pursuit UUID;
    item_owner VARCHAR(255);
    item_record_digest CHAR(64);
    item_state_digest CHAR(64);
    item_status VARCHAR(32);
    previous_item UUID;
    previous_owner VARCHAR(255);
    previous_value VARCHAR(40);
BEGIN
    SELECT proposal_id, pursuit_id, owner_identity, record_digest, state_digest, status
    INTO item_proposal, item_pursuit, item_owner, item_record_digest, item_state_digest, item_status
    FROM pursuit_portfolio_execution_proposal_items
    WHERE id = NEW.proposal_item_id;

    IF NOT FOUND
       OR item_proposal <> NEW.proposal_id
       OR item_pursuit <> NEW.pursuit_id
       OR item_owner <> NEW.owner_identity
       OR item_record_digest <> NEW.proposal_item_digest
       OR item_state_digest <> NEW.state_digest THEN
        RAISE EXCEPTION 'proposal decision does not match its owner-scoped immutable proposal item';
    END IF;
    IF item_status = 'blocked' THEN
        RAISE EXCEPTION 'blocked proposal items cannot receive an approval decision';
    END IF;
    IF NEW.previous_decision_id IS NOT NULL THEN
        SELECT proposal_item_id, owner_identity, decision
        INTO previous_item, previous_owner, previous_value
        FROM pursuit_portfolio_execution_proposal_decisions
        WHERE id = NEW.previous_decision_id;
        IF NOT FOUND OR previous_item <> NEW.proposal_item_id OR previous_owner <> NEW.owner_identity THEN
            RAISE EXCEPTION 'proposal decision chain crossed its owner or item boundary';
        END IF;
    END IF;
    IF NEW.decision = 'revoked' AND (NEW.previous_decision_id IS NULL OR previous_value <> 'approved') THEN
        RAISE EXCEPTION 'only the latest approved proposal decision can be revoked';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER pursuit_portfolio_execution_proposal_decisions_validate_insert
BEFORE INSERT ON pursuit_portfolio_execution_proposal_decisions
FOR EACH ROW EXECUTE FUNCTION validate_pursuit_portfolio_execution_proposal_decision_insert();

CREATE OR REPLACE FUNCTION reject_pursuit_portfolio_execution_proposal_decision_mutation()
RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'pursuit portfolio execution proposal decisions are append-only';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER pursuit_portfolio_execution_proposal_decisions_reject_update
BEFORE UPDATE ON pursuit_portfolio_execution_proposal_decisions
FOR EACH ROW EXECUTE FUNCTION reject_pursuit_portfolio_execution_proposal_decision_mutation();
CREATE TRIGGER pursuit_portfolio_execution_proposal_decisions_reject_delete
BEFORE DELETE ON pursuit_portfolio_execution_proposal_decisions
FOR EACH ROW EXECUTE FUNCTION reject_pursuit_portfolio_execution_proposal_decision_mutation();
CREATE TRIGGER pursuit_portfolio_execution_proposal_decisions_reject_truncate
BEFORE TRUNCATE ON pursuit_portfolio_execution_proposal_decisions
FOR EACH STATEMENT EXECUTE FUNCTION reject_pursuit_portfolio_execution_proposal_decision_mutation();
