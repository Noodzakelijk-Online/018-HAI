DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pursuit_portfolio_allocation_items LIMIT 1)
       OR EXISTS (SELECT 1 FROM pursuit_portfolio_allocations LIMIT 1) THEN
        RAISE EXCEPTION 'refusing to remove non-empty pursuit portfolio allocation audit state';
    END IF;
END;
$$;

DROP TRIGGER IF EXISTS pursuit_portfolio_allocation_items_reject_truncate ON pursuit_portfolio_allocation_items;
DROP TRIGGER IF EXISTS pursuit_portfolio_allocation_items_reject_delete ON pursuit_portfolio_allocation_items;
DROP TRIGGER IF EXISTS pursuit_portfolio_allocation_items_reject_update ON pursuit_portfolio_allocation_items;
DROP TRIGGER IF EXISTS pursuit_portfolio_allocations_reject_truncate ON pursuit_portfolio_allocations;
DROP TRIGGER IF EXISTS pursuit_portfolio_allocations_reject_delete ON pursuit_portfolio_allocations;
DROP TRIGGER IF EXISTS pursuit_portfolio_allocations_reject_update ON pursuit_portfolio_allocations;
DROP TRIGGER IF EXISTS pursuit_portfolio_allocation_items_validate_insert ON pursuit_portfolio_allocation_items;
DROP FUNCTION IF EXISTS reject_pursuit_portfolio_allocation_mutation();
DROP FUNCTION IF EXISTS validate_pursuit_portfolio_allocation_item_insert();
DROP TABLE IF EXISTS pursuit_portfolio_allocation_items;
DROP TABLE IF EXISTS pursuit_portfolio_allocations;
