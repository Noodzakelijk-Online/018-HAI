DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM public.agent_team_consensus_outcomes LIMIT 1)
        OR EXISTS (SELECT 1 FROM public.agent_team_coordination_messages LIMIT 1)
        OR EXISTS (SELECT 1 FROM public.agent_team_lifecycle_events LIMIT 1)
        OR EXISTS (SELECT 1 FROM public.agent_team_contracts LIMIT 1)
        OR EXISTS (SELECT 1 FROM public.agent_teams LIMIT 1) THEN
        RAISE EXCEPTION 'refusing to roll back non-empty agent team lifecycle tables'
            USING ERRCODE = 'object_not_in_prerequisite_state';
    END IF;
END;
$$;

DROP TRIGGER IF EXISTS trg_agent_team_consensus_outcomes_no_truncate
    ON public.agent_team_consensus_outcomes;
DROP TRIGGER IF EXISTS trg_agent_team_consensus_outcomes_immutable
    ON public.agent_team_consensus_outcomes;
DROP TRIGGER IF EXISTS trg_agent_team_consensus_outcomes_revision
    ON public.agent_team_consensus_outcomes;
DROP TRIGGER IF EXISTS trg_agent_team_coordination_messages_no_truncate
    ON public.agent_team_coordination_messages;
DROP TRIGGER IF EXISTS trg_agent_team_coordination_messages_immutable
    ON public.agent_team_coordination_messages;
DROP TRIGGER IF EXISTS trg_agent_team_lifecycle_events_no_truncate
    ON public.agent_team_lifecycle_events;
DROP TRIGGER IF EXISTS trg_agent_team_lifecycle_events_immutable
    ON public.agent_team_lifecycle_events;
DROP TRIGGER IF EXISTS trg_agent_team_lifecycle_events_chain
    ON public.agent_team_lifecycle_events;
DROP TRIGGER IF EXISTS trg_agent_team_contracts_no_truncate
    ON public.agent_team_contracts;
DROP TRIGGER IF EXISTS trg_agent_team_contracts_require_event
    ON public.agent_team_contracts;
DROP TRIGGER IF EXISTS trg_agent_team_contracts_no_delete
    ON public.agent_team_contracts;
DROP TRIGGER IF EXISTS trg_agent_team_contracts_revision
    ON public.agent_team_contracts;
DROP TRIGGER IF EXISTS trg_agent_teams_no_truncate
    ON public.agent_teams;
DROP TRIGGER IF EXISTS trg_agent_teams_immutable
    ON public.agent_teams;

DROP FUNCTION IF EXISTS public.hai_validate_agent_team_consensus_revision();
DROP FUNCTION IF EXISTS public.hai_validate_agent_team_event_chain();
DROP FUNCTION IF EXISTS public.hai_require_agent_team_revision_event();
DROP FUNCTION IF EXISTS public.hai_guard_agent_team_contract_revision();
DROP FUNCTION IF EXISTS public.hai_reject_agent_team_append_only_mutation();

DROP TABLE IF EXISTS public.agent_team_consensus_outcomes;
DROP TABLE IF EXISTS public.agent_team_coordination_messages;
DROP TABLE IF EXISTS public.agent_team_lifecycle_events;
DROP TABLE IF EXISTS public.agent_team_contracts;
DROP TABLE IF EXISTS public.agent_teams;
