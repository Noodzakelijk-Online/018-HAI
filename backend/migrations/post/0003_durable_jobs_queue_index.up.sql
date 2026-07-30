DROP INDEX IF EXISTS public.idx_durable_jobs_claim;

CREATE INDEX idx_durable_jobs_claim
    ON public.durable_jobs USING btree (queue, status, run_at);
