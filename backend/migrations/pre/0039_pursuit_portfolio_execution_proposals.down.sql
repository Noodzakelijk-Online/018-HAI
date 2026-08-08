DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pursuit_portfolio_execution_proposal_items LIMIT 1)
       OR EXISTS (SELECT 1 FROM pursuit_portfolio_execution_proposals LIMIT 1) THEN
        RAISE EXCEPTION 'refusing to remove non-empty pursuit portfolio execution proposal audit state';
    END IF;
END;
$$;

DROP TRIGGER IF EXISTS pursuit_portfolio_execution_proposal_items_reject_truncate ON pursuit_portfolio_execution_proposal_items;
DROP TRIGGER IF EXISTS pursuit_portfolio_execution_proposal_items_reject_delete ON pursuit_portfolio_execution_proposal_items;
DROP TRIGGER IF EXISTS pursuit_portfolio_execution_proposal_items_reject_update ON pursuit_portfolio_execution_proposal_items;
DROP TRIGGER IF EXISTS pursuit_portfolio_execution_proposals_reject_truncate ON pursuit_portfolio_execution_proposals;
DROP TRIGGER IF EXISTS pursuit_portfolio_execution_proposals_reject_delete ON pursuit_portfolio_execution_proposals;
DROP TRIGGER IF EXISTS pursuit_portfolio_execution_proposals_reject_update ON pursuit_portfolio_execution_proposals;
DROP TRIGGER IF EXISTS pursuit_portfolio_execution_proposal_items_validate_insert ON pursuit_portfolio_execution_proposal_items;
DROP TRIGGER IF EXISTS pursuit_portfolio_execution_proposals_validate_insert ON pursuit_portfolio_execution_proposals;
DROP FUNCTION IF EXISTS reject_pursuit_portfolio_execution_proposal_mutation();
DROP FUNCTION IF EXISTS validate_pursuit_portfolio_execution_proposal_item_insert();
DROP FUNCTION IF EXISTS validate_pursuit_portfolio_execution_proposal_insert();
DROP TABLE IF EXISTS pursuit_portfolio_execution_proposal_items;
DROP TABLE IF EXISTS pursuit_portfolio_execution_proposals;
