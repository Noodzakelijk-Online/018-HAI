DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pursuit_resource_reservation_settlements LIMIT 1)
       OR EXISTS (SELECT 1 FROM pursuit_resource_reservations LIMIT 1) THEN
        RAISE EXCEPTION 'refusing to remove non-empty pursuit resource reservation ledger';
    END IF;
END $$;

DROP TRIGGER IF EXISTS pursuit_resource_reservation_settlements_reject_truncate ON pursuit_resource_reservation_settlements;
DROP TRIGGER IF EXISTS pursuit_resource_reservation_settlements_reject_delete ON pursuit_resource_reservation_settlements;
DROP TRIGGER IF EXISTS pursuit_resource_reservation_settlements_reject_update ON pursuit_resource_reservation_settlements;
DROP TRIGGER IF EXISTS pursuit_resource_reservations_reject_truncate ON pursuit_resource_reservations;
DROP TRIGGER IF EXISTS pursuit_resource_reservations_reject_delete ON pursuit_resource_reservations;
DROP TRIGGER IF EXISTS pursuit_resource_reservations_reject_update ON pursuit_resource_reservations;
DROP TRIGGER IF EXISTS pursuit_resource_reservation_settlements_validate_insert ON pursuit_resource_reservation_settlements;
DROP TRIGGER IF EXISTS pursuit_resource_reservations_validate_insert ON pursuit_resource_reservations;
DROP FUNCTION IF EXISTS reject_pursuit_resource_reservation_mutation();
DROP FUNCTION IF EXISTS validate_pursuit_resource_settlement_insert();
DROP FUNCTION IF EXISTS validate_pursuit_resource_reservation_insert();
DROP TABLE IF EXISTS pursuit_resource_reservation_settlements;
DROP TABLE IF EXISTS pursuit_resource_reservations;

-- Restore the refund-only validator installed by migration 0034.
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
