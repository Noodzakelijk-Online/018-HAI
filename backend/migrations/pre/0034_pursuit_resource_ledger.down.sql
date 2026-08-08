DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pursuit_resource_events LIMIT 1) THEN
        RAISE EXCEPTION 'refusing to remove non-empty pursuit resource ledger';
    END IF;
END;
$$;

DROP TRIGGER IF EXISTS pursuit_resource_events_reject_delete ON pursuit_resource_events;
DROP TRIGGER IF EXISTS pursuit_resource_events_reject_update ON pursuit_resource_events;
DROP TRIGGER IF EXISTS pursuit_resource_events_reject_truncate ON pursuit_resource_events;
DROP TRIGGER IF EXISTS pursuit_resource_events_validate_insert ON pursuit_resource_events;
DROP FUNCTION IF EXISTS reject_pursuit_resource_event_mutation();
DROP FUNCTION IF EXISTS validate_pursuit_resource_event_insert();
DROP TABLE IF EXISTS pursuit_resource_events;
