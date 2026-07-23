-- Indexes for the durable worker.
-- Claim query: WHERE status = 'pending' AND run_at <= now() ORDER BY run_at
CREATE INDEX IF NOT EXISTS idx_durable_jobs_claim ON durable_jobs (status, run_at);
-- Lease reaping: WHERE status = 'running' AND locked_at < cutoff
CREATE INDEX IF NOT EXISTS idx_durable_jobs_lease ON durable_jobs (status, locked_at);
