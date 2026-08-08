DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM public.pursuits
        WHERE success_criteria <> '[]'::jsonb
           OR stop_conditions <> '[]'::jsonb
           OR dependencies <> '[]'::jsonb
           OR resource_limits <> '{}'::jsonb
           OR target_at IS NOT NULL
           OR review_cadence_days <> 0
    ) THEN
        RAISE EXCEPTION 'refusing to remove non-empty pursuit goal contracts';
    END IF;
END;
$$;

DROP INDEX public.idx_pursuits_target_at;

ALTER TABLE public.pursuits
    DROP CONSTRAINT chk_pursuits_review_cadence_days,
    DROP CONSTRAINT chk_pursuits_resource_limits_object,
    DROP CONSTRAINT chk_pursuits_dependencies_array,
    DROP CONSTRAINT chk_pursuits_stop_conditions_array,
    DROP CONSTRAINT chk_pursuits_success_criteria_array,
    DROP COLUMN review_cadence_days,
    DROP COLUMN target_at,
    DROP COLUMN resource_limits,
    DROP COLUMN dependencies,
    DROP COLUMN stop_conditions,
    DROP COLUMN success_criteria;
