ALTER TABLE public.pursuit_activities
    ADD COLUMN idempotency_key varchar(160);

CREATE UNIQUE INDEX idx_pursuit_activities_idempotency
    ON public.pursuit_activities (pursuit_id, idempotency_key)
    WHERE idempotency_key IS NOT NULL
      AND btrim(idempotency_key) <> '';
