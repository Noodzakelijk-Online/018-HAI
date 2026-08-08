DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM information_schema.tables
        WHERE table_schema = 'public'
          AND table_name = 'framework_evidence_preflights'
    ) AND EXISTS (
        SELECT 1 FROM public.framework_evidence_preflights
    ) THEN
        RAISE EXCEPTION 'refusing to remove non-empty immutable framework evidence preflight ledger'
            USING ERRCODE = 'object_not_in_prerequisite_state';
    END IF;
END;
$$;

DROP TRIGGER IF EXISTS trg_framework_evidence_preflights_no_truncate
    ON public.framework_evidence_preflights;
DROP TRIGGER IF EXISTS trg_framework_evidence_preflights_immutable
    ON public.framework_evidence_preflights;
DROP TABLE IF EXISTS public.framework_evidence_preflights;
DROP FUNCTION IF EXISTS public.hai_framework_evidence_json_bytes_valid(bytea);
DROP FUNCTION IF EXISTS public.hai_reject_framework_evidence_preflight_mutation();
