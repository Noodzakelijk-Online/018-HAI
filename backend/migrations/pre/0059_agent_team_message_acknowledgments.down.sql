DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM public.agent_team_message_acknowledgments LIMIT 1) THEN
        RAISE EXCEPTION 'refusing to remove non-empty agent team message acknowledgment ledger';
    END IF;
END
$$;

DROP TRIGGER IF EXISTS agent_team_message_acknowledgments_reject_truncate ON public.agent_team_message_acknowledgments;
DROP TRIGGER IF EXISTS agent_team_message_acknowledgments_reject_delete ON public.agent_team_message_acknowledgments;
DROP TRIGGER IF EXISTS agent_team_message_acknowledgments_reject_update ON public.agent_team_message_acknowledgments;
DROP TRIGGER IF EXISTS agent_team_message_acknowledgments_validate_insert ON public.agent_team_message_acknowledgments;
DROP FUNCTION IF EXISTS public.reject_agent_team_message_acknowledgment_mutation();
DROP FUNCTION IF EXISTS public.validate_agent_team_message_acknowledgment_insert();
DROP TABLE IF EXISTS public.agent_team_message_acknowledgments;
