DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM public.model_run_telemetries
        WHERE validation_status <> 'unvalidated'
           OR validation_method <> ''
           OR estimated_cost_eur <> 0
           OR fallback_depth <> 0
    ) THEN
        RAISE EXCEPTION 'refusing to remove non-empty model outcome calibration data';
    END IF;
END;
$$;

DROP INDEX public.idx_model_run_telemetries_lane_calibration;
DROP INDEX public.idx_model_run_telemetries_validation_status;

ALTER TABLE public.model_run_telemetries
    DROP CONSTRAINT chk_model_run_fallback_depth,
    DROP CONSTRAINT chk_model_run_estimated_cost,
    DROP CONSTRAINT chk_model_run_validation_method,
    DROP CONSTRAINT chk_model_run_validation_status,
    DROP COLUMN fallback_depth,
    DROP COLUMN estimated_cost_eur,
    DROP COLUMN validation_method,
    DROP COLUMN validation_status;
