DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM public.outcome_monitor_runs LIMIT 1)
       OR EXISTS (SELECT 1 FROM public.outcome_observation_records LIMIT 1)
       OR EXISTS (SELECT 1 FROM public.outcome_monitor_commands LIMIT 1)
       OR EXISTS (SELECT 1 FROM public.outcome_monitor_targets LIMIT 1) THEN
        RAISE EXCEPTION 'refusing to roll back non-empty outcome attention monitor tables';
    END IF;
END;
$$;

DROP TRIGGER IF EXISTS trg_outcome_monitor_runs_no_truncate
    ON public.outcome_monitor_runs;
DROP TRIGGER IF EXISTS trg_outcome_monitor_runs_immutable
    ON public.outcome_monitor_runs;
DROP TRIGGER IF EXISTS trg_outcome_monitor_runs_validate_insert
    ON public.outcome_monitor_runs;
DROP TRIGGER IF EXISTS trg_outcome_monitor_commands_no_truncate
    ON public.outcome_monitor_commands;
DROP TRIGGER IF EXISTS trg_outcome_monitor_commands_immutable
    ON public.outcome_monitor_commands;
DROP TRIGGER IF EXISTS trg_outcome_monitor_targets_reject_delete
    ON public.outcome_monitor_targets;
DROP TRIGGER IF EXISTS trg_outcome_monitor_targets_validate_update
    ON public.outcome_monitor_targets;
DROP TRIGGER IF EXISTS trg_outcome_monitor_targets_validate_insert
    ON public.outcome_monitor_targets;
DROP TRIGGER IF EXISTS trg_outcome_observation_records_no_truncate
    ON public.outcome_observation_records;
DROP TRIGGER IF EXISTS trg_outcome_observation_records_immutable
    ON public.outcome_observation_records;

DROP FUNCTION IF EXISTS public.hai_validate_outcome_monitor_run_insert();
DROP FUNCTION IF EXISTS public.hai_validate_outcome_monitor_target_write();
DROP FUNCTION IF EXISTS public.hai_reject_outcome_monitor_ledger_mutation();

DROP TABLE IF EXISTS public.outcome_monitor_runs;
DROP TABLE IF EXISTS public.outcome_observation_records;
DROP TABLE IF EXISTS public.outcome_monitor_commands;
DROP TABLE IF EXISTS public.outcome_monitor_targets;
