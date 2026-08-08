DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM public.life_ontology_merge_proposals LIMIT 1)
        OR EXISTS (SELECT 1 FROM public.life_ontology_relations LIMIT 1)
        OR EXISTS (SELECT 1 FROM public.life_ontology_entities LIMIT 1) THEN
        RAISE EXCEPTION 'refusing to roll back non-empty life ontology tables'
            USING ERRCODE = 'object_not_in_prerequisite_state';
    END IF;
END;
$$;

DROP TRIGGER IF EXISTS trg_life_ontology_merge_proposals_no_truncate
    ON public.life_ontology_merge_proposals;
DROP TRIGGER IF EXISTS trg_life_ontology_merge_proposals_immutable
    ON public.life_ontology_merge_proposals;
DROP TRIGGER IF EXISTS trg_life_ontology_relations_no_truncate
    ON public.life_ontology_relations;
DROP TRIGGER IF EXISTS trg_life_ontology_relations_immutable
    ON public.life_ontology_relations;
DROP TRIGGER IF EXISTS trg_life_ontology_entities_no_truncate
    ON public.life_ontology_entities;
DROP TRIGGER IF EXISTS trg_life_ontology_entities_immutable
    ON public.life_ontology_entities;

DROP FUNCTION IF EXISTS public.hai_reject_life_ontology_mutation();

DROP TABLE IF EXISTS public.life_ontology_merge_proposals;
DROP TABLE IF EXISTS public.life_ontology_relations;
DROP TABLE IF EXISTS public.life_ontology_entities;
