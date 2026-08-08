DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM pursuit_resource_reservation_settlements
        WHERE length(btrim(reason)) > 0
        LIMIT 1
    ) THEN
        RAISE EXCEPTION 'refusing to discard pursuit resource reservation reconciliation reasons';
    END IF;
END $$;

ALTER TABLE pursuit_resource_reservation_settlements
    DROP CONSTRAINT IF EXISTS pursuit_resource_settlement_reason_check;

ALTER TABLE pursuit_resource_reservation_settlements
    DROP COLUMN IF EXISTS reason;
