DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM public.proactivity_feedback_records LIMIT 1) THEN
        RAISE EXCEPTION 'refusing to roll back non-empty proactivity attention feedback ledger';
    END IF;
END;
$$;

DROP TRIGGER IF EXISTS trg_proactivity_feedback_no_truncate ON public.proactivity_feedback_records;
DROP TRIGGER IF EXISTS trg_proactivity_feedback_immutable ON public.proactivity_feedback_records;
DROP TRIGGER IF EXISTS trg_proactivity_feedback_validate_insert ON public.proactivity_feedback_records;
DROP TABLE IF EXISTS public.proactivity_feedback_records;
DROP FUNCTION IF EXISTS public.hai_validate_proactivity_feedback_insert();
