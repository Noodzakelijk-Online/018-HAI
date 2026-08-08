DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM public.outcome_monitor_composition_attempts LIMIT 1)
       OR EXISTS (SELECT 1 FROM public.outcome_monitor_composition_deliveries LIMIT 1) THEN
        RAISE EXCEPTION 'refusing to roll back non-empty outcome monitor composition delivery ledgers';
    END IF;
END;
$$;

DROP TRIGGER IF EXISTS trg_outcome_monitor_run_enqueue_composition_delivery
    ON public.outcome_monitor_runs;
DROP TRIGGER IF EXISTS trg_outcome_monitor_composition_attempt_no_truncate
    ON public.outcome_monitor_composition_attempts;
DROP TRIGGER IF EXISTS trg_outcome_monitor_composition_attempt_immutable
    ON public.outcome_monitor_composition_attempts;
DROP TRIGGER IF EXISTS trg_outcome_monitor_composition_attempt_validate_insert
    ON public.outcome_monitor_composition_attempts;
DROP TRIGGER IF EXISTS trg_outcome_monitor_composition_delivery_no_truncate
    ON public.outcome_monitor_composition_deliveries;
DROP TRIGGER IF EXISTS trg_outcome_monitor_composition_delivery_reject_delete
    ON public.outcome_monitor_composition_deliveries;
DROP TRIGGER IF EXISTS trg_outcome_monitor_composition_delivery_validate_update
    ON public.outcome_monitor_composition_deliveries;
DROP TRIGGER IF EXISTS trg_outcome_monitor_composition_delivery_validate_insert
    ON public.outcome_monitor_composition_deliveries;

DROP FUNCTION IF EXISTS public.hai_enqueue_outcome_monitor_composition_delivery();
DROP FUNCTION IF EXISTS public.hai_validate_outcome_monitor_composition_delivery_write();
DROP FUNCTION IF EXISTS public.hai_validate_outcome_monitor_composition_attempt_insert();
DROP FUNCTION IF EXISTS public.hai_validate_outcome_monitor_composition_delivery_insert();
DROP FUNCTION IF EXISTS public.hai_reject_outcome_monitor_composition_attempt_mutation();

DROP TABLE IF EXISTS public.outcome_monitor_composition_attempts;
DROP TABLE IF EXISTS public.outcome_monitor_composition_deliveries;

ALTER TABLE public.outcome_observation_records
    DROP CONSTRAINT IF EXISTS uq_outcome_observation_owner_workspace_id;
ALTER TABLE public.outcome_monitor_runs
    DROP CONSTRAINT IF EXISTS uq_outcome_monitor_run_owner_workspace_id;
