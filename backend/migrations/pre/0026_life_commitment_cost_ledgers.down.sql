DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM public.life_ledger_commitment_revisions LIMIT 1)
        OR EXISTS (SELECT 1 FROM public.life_ledger_cost_entries LIMIT 1) THEN
        RAISE EXCEPTION 'refusing to discard immutable life commitment or cost history';
    END IF;
END;
$$;

DROP TRIGGER IF EXISTS trg_life_ledger_costs_no_truncate ON public.life_ledger_cost_entries;
DROP TRIGGER IF EXISTS trg_life_ledger_costs_immutable ON public.life_ledger_cost_entries;
DROP TRIGGER IF EXISTS trg_life_ledger_commitments_no_truncate ON public.life_ledger_commitment_revisions;
DROP TRIGGER IF EXISTS trg_life_ledger_commitments_immutable ON public.life_ledger_commitment_revisions;
DROP TABLE IF EXISTS public.life_ledger_cost_entries;
DROP TABLE IF EXISTS public.life_ledger_commitment_revisions;
DROP FUNCTION IF EXISTS public.hai_reject_life_ledger_mutation();
