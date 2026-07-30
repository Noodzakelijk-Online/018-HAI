DROP TRIGGER IF EXISTS trg_standing_mandate_decisions_no_truncate
    ON public.standing_mandate_authorization_decisions;
DROP TRIGGER IF EXISTS trg_standing_mandate_decisions_immutable
    ON public.standing_mandate_authorization_decisions;
DROP TRIGGER IF EXISTS trg_standing_mandates_no_delete
    ON public.standing_mandates;
DROP TRIGGER IF EXISTS trg_standing_mandates_lifecycle
    ON public.standing_mandates;

DROP FUNCTION IF EXISTS public.hai_reject_standing_mandate_mutation();
DROP FUNCTION IF EXISTS public.hai_reject_standing_mandate_delete();
DROP FUNCTION IF EXISTS public.hai_enforce_standing_mandate_lifecycle();

DROP TABLE IF EXISTS public.standing_mandate_authorization_decisions;
DROP TABLE IF EXISTS public.standing_mandates;
