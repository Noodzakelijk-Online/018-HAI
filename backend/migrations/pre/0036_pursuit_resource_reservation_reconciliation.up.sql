ALTER TABLE pursuit_resource_reservation_settlements
    ADD COLUMN IF NOT EXISTS reason TEXT NOT NULL DEFAULT '';

-- Releases created before this migration had no reason column. Temporarily
-- suspend only the update guard while assigning an explicit legacy marker;
-- append-only protection is restored before the new constraint is installed.
ALTER TABLE pursuit_resource_reservation_settlements
    DISABLE TRIGGER pursuit_resource_reservation_settlements_reject_update;

UPDATE pursuit_resource_reservation_settlements
SET reason = 'Legacy release recorded before reconciliation reason capture.'
WHERE disposition = 'released' AND length(btrim(reason)) = 0;

ALTER TABLE pursuit_resource_reservation_settlements
    ENABLE TRIGGER pursuit_resource_reservation_settlements_reject_update;

ALTER TABLE pursuit_resource_reservation_settlements
    DROP CONSTRAINT IF EXISTS pursuit_resource_settlement_reason_check;

ALTER TABLE pursuit_resource_reservation_settlements
    ADD CONSTRAINT pursuit_resource_settlement_reason_check CHECK (
        length(reason) <= 1000
        AND (disposition <> 'released' OR length(btrim(reason)) >= 12)
    );

COMMENT ON COLUMN pursuit_resource_reservation_settlements.reason IS
    'Immutable operator or engine reason for consuming or releasing the resource hold.';
