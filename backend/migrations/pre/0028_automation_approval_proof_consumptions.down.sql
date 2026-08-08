DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.tables
        WHERE table_schema = 'public'
          AND table_name = 'automation_approval_proof_consumptions'
    ) AND EXISTS (
        SELECT 1 FROM public.automation_approval_proof_consumptions
    ) THEN
        RAISE EXCEPTION 'refusing to remove non-empty immutable approval proof consumption ledger'
            USING ERRCODE = 'object_not_in_prerequisite_state';
    END IF;
END;
$$;

DROP TRIGGER IF EXISTS trg_automation_approval_proof_consumptions_no_truncate
    ON public.automation_approval_proof_consumptions;
DROP TRIGGER IF EXISTS trg_automation_approval_proof_consumptions_immutable
    ON public.automation_approval_proof_consumptions;
DROP TABLE IF EXISTS public.automation_approval_proof_consumptions;
