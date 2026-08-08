DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM public.proactivity_decision_records LIMIT 1)
        OR EXISTS (SELECT 1 FROM public.proactivity_decision_batches LIMIT 1)
        OR EXISTS (SELECT 1 FROM public.proactivity_signal_records LIMIT 1)
        OR EXISTS (SELECT 1 FROM public.proactivity_signal_batches LIMIT 1)
        OR EXISTS (SELECT 1 FROM public.proactivity_policy_records LIMIT 1)
        OR EXISTS (SELECT 1 FROM public.proactivity_idempotency LIMIT 1)
    THEN
        RAISE EXCEPTION 'refusing to roll back non-empty proactivity advisory tables';
    END IF;
END;
$$;

DROP TRIGGER IF EXISTS trg_proactivity_decision_records_no_truncate
    ON public.proactivity_decision_records;
DROP TRIGGER IF EXISTS trg_proactivity_decision_records_immutable
    ON public.proactivity_decision_records;
DROP TRIGGER IF EXISTS trg_proactivity_decision_records_consistent
    ON public.proactivity_decision_records;
DROP TRIGGER IF EXISTS trg_proactivity_decision_batches_no_truncate
    ON public.proactivity_decision_batches;
DROP TRIGGER IF EXISTS trg_proactivity_decision_batches_immutable
    ON public.proactivity_decision_batches;
DROP TRIGGER IF EXISTS trg_proactivity_decision_batches_consistent
    ON public.proactivity_decision_batches;
DROP TRIGGER IF EXISTS trg_proactivity_signal_records_no_truncate
    ON public.proactivity_signal_records;
DROP TRIGGER IF EXISTS trg_proactivity_signal_records_immutable
    ON public.proactivity_signal_records;
DROP TRIGGER IF EXISTS trg_proactivity_signal_records_consistent
    ON public.proactivity_signal_records;
DROP TRIGGER IF EXISTS trg_proactivity_signal_batches_no_truncate
    ON public.proactivity_signal_batches;
DROP TRIGGER IF EXISTS trg_proactivity_signal_batches_immutable
    ON public.proactivity_signal_batches;
DROP TRIGGER IF EXISTS trg_proactivity_signal_batches_consistent
    ON public.proactivity_signal_batches;
DROP TRIGGER IF EXISTS trg_proactivity_policy_records_no_truncate
    ON public.proactivity_policy_records;
DROP TRIGGER IF EXISTS trg_proactivity_policy_records_immutable
    ON public.proactivity_policy_records;
DROP TRIGGER IF EXISTS trg_proactivity_idempotency_no_truncate
    ON public.proactivity_idempotency;
DROP TRIGGER IF EXISTS trg_proactivity_idempotency_immutable
    ON public.proactivity_idempotency;

DROP TABLE IF EXISTS public.proactivity_decision_records;
DROP TABLE IF EXISTS public.proactivity_decision_batches;
DROP TABLE IF EXISTS public.proactivity_signal_records;
DROP TABLE IF EXISTS public.proactivity_signal_batches;
DROP TABLE IF EXISTS public.proactivity_policy_records;
DROP TABLE IF EXISTS public.proactivity_idempotency;

DROP FUNCTION IF EXISTS public.hai_reject_proactivity_mutation();
DROP FUNCTION IF EXISTS public.hai_validate_proactivity_decision_batch();
DROP FUNCTION IF EXISTS public.hai_validate_proactivity_signal_batch();
DROP FUNCTION IF EXISTS public.hai_proactivity_decisions_are_advisory(jsonb);
