DROP TRIGGER IF EXISTS trg_life_capacity_snapshots_immutable ON public.life_capacity_snapshots;
DROP TRIGGER IF EXISTS trg_life_priority_assessments_immutable ON public.life_priority_assessments;
DROP TRIGGER IF EXISTS trg_life_need_observations_immutable ON public.life_need_observations;
DROP FUNCTION IF EXISTS public.hai_reject_life_observation_mutation();
DROP TABLE IF EXISTS public.life_priority_assessments;
DROP TABLE IF EXISTS public.life_goal_nodes;
DROP TABLE IF EXISTS public.life_capacity_snapshots;
DROP TABLE IF EXISTS public.life_need_observations;
DROP TABLE IF EXISTS public.life_entity_domain_links;
