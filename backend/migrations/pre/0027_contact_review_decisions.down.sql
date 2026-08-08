DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.tables
        WHERE table_schema = 'public'
          AND table_name = 'life_ontology_contact_review_decisions'
    ) AND EXISTS (
        SELECT 1 FROM public.life_ontology_contact_review_decisions
    ) THEN
        RAISE EXCEPTION 'refusing to remove non-empty immutable contact review ledger'
            USING ERRCODE = 'object_not_in_prerequisite_state';
    END IF;
END;
$$;

DROP TRIGGER IF EXISTS trg_life_ontology_contact_review_validate
    ON public.life_ontology_contact_review_decisions;
DROP TRIGGER IF EXISTS trg_life_ontology_contact_review_no_truncate
    ON public.life_ontology_contact_review_decisions;
DROP TRIGGER IF EXISTS trg_life_ontology_contact_review_immutable
    ON public.life_ontology_contact_review_decisions;
DROP TABLE IF EXISTS public.life_ontology_contact_review_decisions;
DROP FUNCTION IF EXISTS public.hai_validate_contact_review_decision();
