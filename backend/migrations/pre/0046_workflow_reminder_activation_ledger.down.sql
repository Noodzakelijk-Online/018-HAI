DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM public.workflow_reminder_activation_decisions LIMIT 1)
       OR EXISTS (SELECT 1 FROM public.workflow_reminder_activation_requests LIMIT 1) THEN
        RAISE EXCEPTION 'refusing to remove non-empty workflow reminder activation ledgers';
    END IF;
END
$$;

DROP TRIGGER IF EXISTS workflow_reminder_activation_decisions_reject_truncate
    ON public.workflow_reminder_activation_decisions;
DROP TRIGGER IF EXISTS workflow_reminder_activation_decisions_reject_delete
    ON public.workflow_reminder_activation_decisions;
DROP TRIGGER IF EXISTS workflow_reminder_activation_decisions_reject_update
    ON public.workflow_reminder_activation_decisions;
DROP TRIGGER IF EXISTS workflow_reminder_activation_requests_reject_truncate
    ON public.workflow_reminder_activation_requests;
DROP TRIGGER IF EXISTS workflow_reminder_activation_requests_reject_delete
    ON public.workflow_reminder_activation_requests;
DROP TRIGGER IF EXISTS workflow_reminder_activation_requests_reject_update
    ON public.workflow_reminder_activation_requests;
DROP TRIGGER IF EXISTS workflow_reminder_activation_decisions_validate_insert
    ON public.workflow_reminder_activation_decisions;
DROP TRIGGER IF EXISTS workflow_reminder_activation_requests_validate_insert
    ON public.workflow_reminder_activation_requests;

DROP FUNCTION IF EXISTS public.reject_workflow_reminder_activation_mutation();
DROP FUNCTION IF EXISTS public.validate_workflow_reminder_activation_decision_insert();
DROP FUNCTION IF EXISTS public.validate_workflow_reminder_activation_request_insert();
DROP TABLE IF EXISTS public.workflow_reminder_activation_decisions;
DROP TABLE IF EXISTS public.workflow_reminder_activation_requests;
