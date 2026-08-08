ALTER TABLE public.pursuits
    ADD COLUMN success_criteria jsonb NOT NULL DEFAULT '[]'::jsonb,
    ADD COLUMN stop_conditions jsonb NOT NULL DEFAULT '[]'::jsonb,
    ADD COLUMN dependencies jsonb NOT NULL DEFAULT '[]'::jsonb,
    ADD COLUMN resource_limits jsonb NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN target_at timestamptz,
    ADD COLUMN review_cadence_days integer NOT NULL DEFAULT 0;

UPDATE public.pursuits
SET success_criteria = jsonb_build_array(jsonb_build_object(
        'id', 'legacy-completion-definition',
        'description', completion_definition,
        'status', 'pending',
        'evidenceRequired', true
    ))
WHERE btrim(COALESCE(completion_definition, '')) <> '';

UPDATE public.pursuits
SET stop_conditions = jsonb_build_array(jsonb_build_object(
        'id', 'default-safe-stop',
        'description', 'Stop and request review when the pursuit no longer serves its outcome or exceeds an approved boundary.',
        'status', 'monitoring'
    ));

ALTER TABLE public.pursuits
    ADD CONSTRAINT chk_pursuits_success_criteria_array
        CHECK (jsonb_typeof(success_criteria) = 'array'),
    ADD CONSTRAINT chk_pursuits_stop_conditions_array
        CHECK (jsonb_typeof(stop_conditions) = 'array'),
    ADD CONSTRAINT chk_pursuits_dependencies_array
        CHECK (jsonb_typeof(dependencies) = 'array'),
    ADD CONSTRAINT chk_pursuits_resource_limits_object
        CHECK (jsonb_typeof(resource_limits) = 'object'),
    ADD CONSTRAINT chk_pursuits_review_cadence_days
        CHECK (review_cadence_days BETWEEN 0 AND 3650);

CREATE INDEX idx_pursuits_target_at
    ON public.pursuits (target_at)
    WHERE target_at IS NOT NULL AND archived = false;
