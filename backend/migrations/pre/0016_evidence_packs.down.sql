DROP TRIGGER IF EXISTS trg_evidence_packs_no_truncate
    ON public.evidence_packs;
DROP TRIGGER IF EXISTS trg_evidence_packs_immutable
    ON public.evidence_packs;
DROP FUNCTION IF EXISTS public.hai_reject_evidence_pack_mutation();

DROP TABLE IF EXISTS public.evidence_packs;

ALTER TABLE public.operations
    DROP CONSTRAINT IF EXISTS uq_operations_owner_workspace_id;
