DROP TABLE IF EXISTS public.task_review_decisions;
DROP TABLE IF EXISTS public.task_review_items;
DROP TABLE IF EXISTS public.task_completion_plan_logs;

DROP FUNCTION IF EXISTS public.hai_enforce_task_review_decision_binding();
DROP FUNCTION IF EXISTS public.hai_require_task_review_resolution_state();
DROP FUNCTION IF EXISTS public.hai_enforce_task_review_item_transition();
DROP FUNCTION IF EXISTS public.hai_enforce_task_review_item_insert();
DROP FUNCTION IF EXISTS public.hai_enforce_task_review_item_provenance();
DROP FUNCTION IF EXISTS public.hai_reject_task_audit_truncate();
DROP FUNCTION IF EXISTS public.hai_reject_task_audit_mutation();
