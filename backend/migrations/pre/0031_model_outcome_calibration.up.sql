ALTER TABLE public.model_run_telemetries
    ADD COLUMN validation_status character varying(32) NOT NULL DEFAULT 'unvalidated',
    ADD COLUMN validation_method character varying(120) NOT NULL DEFAULT '',
    ADD COLUMN estimated_cost_eur numeric(12,6) NOT NULL DEFAULT 0,
    ADD COLUMN fallback_depth integer NOT NULL DEFAULT 0;

ALTER TABLE public.model_run_telemetries
    ADD CONSTRAINT chk_model_run_validation_status CHECK (
        validation_status IN (
            'unvalidated',
            'schema_validated',
            'source_supported',
            'test_passed',
            'human_approved',
            'verified',
            'failed',
            'needs_review'
        )
    ),
    ADD CONSTRAINT chk_model_run_validation_method CHECK (
        length(validation_method) <= 120
        AND validation_method !~ E'[\r\n]'
    ),
    ADD CONSTRAINT chk_model_run_estimated_cost CHECK (
        estimated_cost_eur >= 0
        AND estimated_cost_eur <= 100000
    ),
    ADD CONSTRAINT chk_model_run_fallback_depth CHECK (
        fallback_depth BETWEEN 0 AND 32
    );

CREATE INDEX idx_model_run_telemetries_validation_status
    ON public.model_run_telemetries (validation_status, created_at DESC);

CREATE INDEX idx_model_run_telemetries_lane_calibration
    ON public.model_run_telemetries (
        lane,
        provider_id,
        model_id,
        validation_status,
        created_at DESC
    );
