DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM public.framework_selection_records
        WHERE selector_algorithm_version = 'selector-v5'
    ) THEN
        RAISE EXCEPTION 'cannot roll back selector-v5 risk contract while selector-v5 records exist';
    END IF;
END
$$;

ALTER TABLE public.framework_selection_records
    DROP CONSTRAINT IF EXISTS chk_framework_selection_records_risk_ceiling_rank,
    DROP CONSTRAINT IF EXISTS chk_framework_selection_records_v5_risk_contract,
    DROP CONSTRAINT IF EXISTS chk_framework_selection_records_effective_risk_ceiling,
    DROP CONSTRAINT IF EXISTS chk_framework_selection_records_task_risk_level,
    DROP CONSTRAINT IF EXISTS chk_framework_selection_records_operating_digest,
    ADD CONSTRAINT chk_framework_selection_records_operating_digest
        CHECK (
            operating_contract_digest ~ '^[0-9a-f]{64}$'
            AND (
                selector_algorithm_version <> 'selector-v4'
                OR operating_contract_digest <>
                    '0000000000000000000000000000000000000000000000000000000000000000'
            )
        ),
    DROP COLUMN IF EXISTS effective_risk_ceiling,
    DROP COLUMN IF EXISTS task_risk_level;
