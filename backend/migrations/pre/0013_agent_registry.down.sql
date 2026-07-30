DROP TRIGGER IF EXISTS trg_agent_registry_outcomes_no_truncate
    ON public.agent_registry_assignment_outcomes;
DROP TRIGGER IF EXISTS trg_agent_registry_outcomes_immutable
    ON public.agent_registry_assignment_outcomes;
DROP TRIGGER IF EXISTS trg_agent_registry_assignments_no_truncate
    ON public.agent_registry_assignments;
DROP TRIGGER IF EXISTS trg_agent_registry_assignments_immutable
    ON public.agent_registry_assignments;
DROP TRIGGER IF EXISTS trg_agent_registry_transitions_no_truncate
    ON public.agent_registry_transitions;
DROP TRIGGER IF EXISTS trg_agent_registry_transitions_immutable
    ON public.agent_registry_transitions;
DROP TRIGGER IF EXISTS trg_agent_registry_revisions_no_truncate
    ON public.agent_registry_revisions;
DROP TRIGGER IF EXISTS trg_agent_registry_revisions_immutable
    ON public.agent_registry_revisions;
DROP TRIGGER IF EXISTS trg_agent_registry_agent_revision
    ON public.agent_registry_agents;

DROP FUNCTION IF EXISTS public.hai_reject_agent_registry_audit_mutation();
DROP FUNCTION IF EXISTS public.hai_enforce_agent_registry_revision();

DROP TABLE IF EXISTS public.agent_registry_assignment_outcomes;
DROP TABLE IF EXISTS public.agent_registry_assignments;
DROP TABLE IF EXISTS public.agent_registry_transitions;
DROP TABLE IF EXISTS public.agent_registry_revisions;
DROP TABLE IF EXISTS public.agent_registry_agents;
