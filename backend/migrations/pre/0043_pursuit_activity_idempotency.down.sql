DROP INDEX IF EXISTS public.idx_pursuit_activities_idempotency;

ALTER TABLE public.pursuit_activities
    DROP COLUMN IF EXISTS idempotency_key;
