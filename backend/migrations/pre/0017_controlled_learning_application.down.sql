DROP TRIGGER IF EXISTS trg_controlled_learning_applications_no_truncate
    ON public.controlled_learning_applications;
DROP TRIGGER IF EXISTS trg_controlled_learning_applications_no_delete
    ON public.controlled_learning_applications;
DROP TRIGGER IF EXISTS trg_controlled_learning_applications_guard_update
    ON public.controlled_learning_applications;
DROP TRIGGER IF EXISTS trg_controlled_learning_application_events_no_truncate
    ON public.controlled_learning_application_events;
DROP TRIGGER IF EXISTS trg_controlled_learning_application_events_immutable
    ON public.controlled_learning_application_events;
DROP TRIGGER IF EXISTS trg_controlled_learning_proposals_require_application
    ON public.controlled_learning_proposals;

DROP FUNCTION IF EXISTS public.hai_guard_controlled_learning_application_definition();
DROP FUNCTION IF EXISTS public.hai_require_controlled_learning_application();

ALTER TABLE public.controlled_learning_review_decisions
    DROP CONSTRAINT IF EXISTS chk_controlled_learning_review_decision_application;
ALTER TABLE public.controlled_learning_review_decisions
    DROP CONSTRAINT IF EXISTS fk_controlled_learning_review_decision_application;
ALTER TABLE public.controlled_learning_review_decisions
    DROP COLUMN IF EXISTS application_id;

DROP TABLE IF EXISTS public.controlled_learning_application_events;
DROP TABLE IF EXISTS public.controlled_learning_applications;
