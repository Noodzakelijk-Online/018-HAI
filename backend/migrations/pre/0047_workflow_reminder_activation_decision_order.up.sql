CREATE OR REPLACE FUNCTION public.validate_workflow_reminder_activation_decision_order_insert()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    latest_record public.workflow_reminder_activation_decisions%ROWTYPE;
BEGIN
    SELECT * INTO latest_record
    FROM public.workflow_reminder_activation_decisions
    WHERE activation_request_id = NEW.activation_request_id
      AND owner_identity = NEW.owner_identity
    ORDER BY decided_at DESC, id DESC
    LIMIT 1;

    IF latest_record.id IS NOT NULL
       AND NEW.decided_at <= latest_record.decided_at THEN
        RAISE EXCEPTION 'reminder activation decision time must advance after the current chain tip';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER workflow_reminder_activation_decisions_validate_order_insert
BEFORE INSERT ON public.workflow_reminder_activation_decisions
FOR EACH ROW EXECUTE FUNCTION public.validate_workflow_reminder_activation_decision_order_insert();
