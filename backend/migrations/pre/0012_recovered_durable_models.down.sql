DROP TRIGGER IF EXISTS trg_llm_generation_records_no_truncate
    ON public.llm_generation_records;
DROP TRIGGER IF EXISTS trg_llm_generation_records_immutable
    ON public.llm_generation_records;
DROP TRIGGER IF EXISTS trg_llm_model_maintenances_no_truncate
    ON public.llm_model_maintenances;
DROP TRIGGER IF EXISTS trg_llm_model_maintenances_immutable
    ON public.llm_model_maintenances;
DROP TRIGGER IF EXISTS trg_brain_catalog_repository_reviews_no_truncate
    ON public.brain_catalog_repository_discovery_reviews;
DROP TRIGGER IF EXISTS trg_brain_catalog_repository_reviews_immutable
    ON public.brain_catalog_repository_discovery_reviews;
DROP TRIGGER IF EXISTS trg_brain_catalog_collection_reviews_no_truncate
    ON public.brain_catalog_collection_reviews;
DROP TRIGGER IF EXISTS trg_brain_catalog_collection_reviews_immutable
    ON public.brain_catalog_collection_reviews;
DROP TRIGGER IF EXISTS trg_brain_catalog_upstream_reviews_no_truncate
    ON public.brain_catalog_upstream_reviews;
DROP TRIGGER IF EXISTS trg_brain_catalog_upstream_reviews_immutable
    ON public.brain_catalog_upstream_reviews;

DROP FUNCTION IF EXISTS public.hai_reject_recovered_audit_mutation();

DROP TABLE IF EXISTS public.mini_swe_patch_proposals;
DROP TABLE IF EXISTS public.llm_generation_records;
DROP TABLE IF EXISTS public.llm_model_maintenances;
DROP TABLE IF EXISTS public.brain_catalog_repository_discovery_reviews;
DROP TABLE IF EXISTS public.brain_catalog_collection_reviews;
DROP TABLE IF EXISTS public.brain_catalog_upstream_reviews;
