DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM public.task_operations LIMIT 1) THEN
        RAISE EXCEPTION 'refusing to remove non-empty task operation audit state';
    END IF;
END;
$$;

DROP TRIGGER IF EXISTS task_operations_reject_truncate ON public.task_operations;
DROP TRIGGER IF EXISTS task_operations_reject_delete ON public.task_operations;
DROP TRIGGER IF EXISTS task_operations_guard_identity ON public.task_operations;
DROP FUNCTION IF EXISTS public.task_operations_reject_removal();
DROP FUNCTION IF EXISTS public.task_operations_guard_identity();
DROP TABLE IF EXISTS public.task_operations;
