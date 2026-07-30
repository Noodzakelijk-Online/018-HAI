DROP TABLE IF EXISTS public.robert_constitution_versions;
DROP FUNCTION IF EXISTS public.hai_enforce_constitution_lifecycle();
DROP FUNCTION IF EXISTS public.hai_require_active_constitution_after_history();

DROP TABLE IF EXISTS public.framework_selection_records;
DROP FUNCTION IF EXISTS public.hai_reject_framework_selection_mutation();
DROP FUNCTION IF EXISTS public.hai_reject_framework_registry_truncate();

DROP TABLE IF EXISTS public.framework_preferences;
