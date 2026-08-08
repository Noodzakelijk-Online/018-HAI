DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM public.workflow_reminder_delivery_attempts LIMIT 1)
       OR EXISTS (SELECT 1 FROM public.workflow_reminder_delivery_authorizations LIMIT 1) THEN
        RAISE EXCEPTION 'refusing to remove non-empty workflow reminder delivery ledgers';
    END IF;
END
$$;
DROP TRIGGER IF EXISTS workflow_reminder_delivery_attempts_reject_truncate ON public.workflow_reminder_delivery_attempts;
DROP TRIGGER IF EXISTS workflow_reminder_delivery_attempts_reject_delete ON public.workflow_reminder_delivery_attempts;
DROP TRIGGER IF EXISTS workflow_reminder_delivery_attempts_reject_update ON public.workflow_reminder_delivery_attempts;
DROP TRIGGER IF EXISTS workflow_reminder_delivery_authorizations_reject_truncate ON public.workflow_reminder_delivery_authorizations;
DROP TRIGGER IF EXISTS workflow_reminder_delivery_authorizations_reject_delete ON public.workflow_reminder_delivery_authorizations;
DROP TRIGGER IF EXISTS workflow_reminder_delivery_authorizations_reject_update ON public.workflow_reminder_delivery_authorizations;
DROP TRIGGER IF EXISTS workflow_reminder_delivery_authorizations_validate_insert ON public.workflow_reminder_delivery_authorizations;
DROP FUNCTION IF EXISTS public.validate_workflow_reminder_delivery_authorization_insert();
DROP TABLE IF EXISTS public.workflow_reminder_delivery_attempts;
DROP TABLE IF EXISTS public.workflow_reminder_delivery_authorizations;
