DROP TRIGGER IF EXISTS trg_controlled_learning_review_decisions_no_truncate
    ON public.controlled_learning_review_decisions;
DROP TRIGGER IF EXISTS trg_controlled_learning_decisions_require_state
    ON public.controlled_learning_review_decisions;
DROP TRIGGER IF EXISTS trg_controlled_learning_review_decisions_immutable
    ON public.controlled_learning_review_decisions;
DROP TRIGGER IF EXISTS trg_controlled_learning_proposal_evidence_no_truncate
    ON public.controlled_learning_proposal_evidence;
DROP TRIGGER IF EXISTS trg_controlled_learning_proposal_evidence_immutable
    ON public.controlled_learning_proposal_evidence;
DROP TRIGGER IF EXISTS trg_controlled_learning_proposals_no_truncate
    ON public.controlled_learning_proposals;
DROP TRIGGER IF EXISTS trg_controlled_learning_proposals_require_decision
    ON public.controlled_learning_proposals;
DROP TRIGGER IF EXISTS trg_controlled_learning_proposals_no_delete
    ON public.controlled_learning_proposals;
DROP TRIGGER IF EXISTS trg_controlled_learning_proposals_validate_insert
    ON public.controlled_learning_proposals;
DROP TRIGGER IF EXISTS trg_controlled_learning_proposals_guard_update
    ON public.controlled_learning_proposals;
DROP TRIGGER IF EXISTS trg_controlled_learning_outcomes_no_truncate
    ON public.controlled_learning_outcomes;
DROP TRIGGER IF EXISTS trg_controlled_learning_outcomes_immutable
    ON public.controlled_learning_outcomes;

DROP FUNCTION IF EXISTS public.hai_require_controlled_learning_review_pair();
DROP FUNCTION IF EXISTS public.hai_validate_controlled_learning_proposal_insert();
DROP FUNCTION IF EXISTS public.hai_guard_controlled_learning_proposal_state();
DROP FUNCTION IF EXISTS public.hai_reject_controlled_learning_mutation();

DROP TABLE IF EXISTS public.controlled_learning_review_decisions;
DROP TABLE IF EXISTS public.controlled_learning_proposal_evidence;
DROP TABLE IF EXISTS public.controlled_learning_proposals;
DROP TABLE IF EXISTS public.controlled_learning_outcomes;
