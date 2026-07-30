DROP INDEX IF EXISTS public.idx_durable_jobs_owned_lease;

ALTER TABLE public.durable_jobs
    DROP COLUMN IF EXISTS lease_generation;
