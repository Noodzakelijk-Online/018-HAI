DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM public.outcome_evaluation_outcome_revisions LIMIT 1)
        OR EXISTS (SELECT 1 FROM public.outcome_evaluation_evaluations LIMIT 1)
        OR EXISTS (SELECT 1 FROM public.outcome_evaluation_corrections LIMIT 1)
        OR EXISTS (SELECT 1 FROM public.resilience_idempotency_records LIMIT 1)
        OR EXISTS (SELECT 1 FROM public.resilience_leases LIMIT 1)
        OR EXISTS (SELECT 1 FROM public.resilience_worker_heartbeats LIMIT 1)
        OR EXISTS (SELECT 1 FROM public.resilience_circuits LIMIT 1)
        OR EXISTS (SELECT 1 FROM public.resilience_retry_records LIMIT 1)
        OR EXISTS (SELECT 1 FROM public.resilience_recovery_records LIMIT 1)
        OR EXISTS (SELECT 1 FROM public.resilience_event_records LIMIT 1)
    THEN
        RAISE EXCEPTION 'refusing to roll back non-empty outcome or resilience ledgers';
    END IF;
END;
$$;

DROP TABLE IF EXISTS public.resilience_event_records;
DROP TABLE IF EXISTS public.resilience_recovery_records;
DROP TABLE IF EXISTS public.resilience_retry_records;
DROP TABLE IF EXISTS public.resilience_circuits;
DROP TABLE IF EXISTS public.resilience_worker_heartbeats;
DROP TABLE IF EXISTS public.resilience_leases;
DROP TABLE IF EXISTS public.resilience_idempotency_records;
DROP FUNCTION IF EXISTS public.hai_reject_resilience_history_mutation();

DROP TABLE IF EXISTS public.outcome_evaluation_corrections;
DROP TABLE IF EXISTS public.outcome_evaluation_evaluations;
DROP TABLE IF EXISTS public.outcome_evaluation_outcome_revisions;
DROP FUNCTION IF EXISTS public.hai_reject_outcome_evaluation_mutation();
