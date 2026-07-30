ALTER TABLE public.durable_jobs
    ADD COLUMN IF NOT EXISTS lease_generation bigint DEFAULT 0 NOT NULL;

CREATE INDEX IF NOT EXISTS idx_durable_jobs_owned_lease
    ON public.durable_jobs USING btree (id, status, locked_by, lease_generation);
