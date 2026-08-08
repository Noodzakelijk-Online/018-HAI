ALTER TABLE public.framework_selection_records
    ADD COLUMN IF NOT EXISTS task_risk_level varchar(16),
    ADD COLUMN IF NOT EXISTS effective_risk_ceiling varchar(16),
    DROP CONSTRAINT IF EXISTS chk_framework_selection_records_operating_digest,
    ADD CONSTRAINT chk_framework_selection_records_operating_digest
        CHECK (
            operating_contract_digest ~ '^[0-9a-f]{64}$'
            AND (
                selector_algorithm_version NOT IN ('selector-v4', 'selector-v5')
                OR operating_contract_digest <>
                    '0000000000000000000000000000000000000000000000000000000000000000'
            )
        );

ALTER TABLE public.framework_selection_records
    DROP CONSTRAINT IF EXISTS chk_framework_selection_records_task_risk_level,
    ADD CONSTRAINT chk_framework_selection_records_task_risk_level
        CHECK (task_risk_level IS NULL OR task_risk_level IN ('low', 'medium', 'high')),
    DROP CONSTRAINT IF EXISTS chk_framework_selection_records_effective_risk_ceiling,
    ADD CONSTRAINT chk_framework_selection_records_effective_risk_ceiling
        CHECK (effective_risk_ceiling IS NULL OR effective_risk_ceiling IN ('low', 'medium', 'high')),
    DROP CONSTRAINT IF EXISTS chk_framework_selection_records_v5_risk_contract,
    ADD CONSTRAINT chk_framework_selection_records_v5_risk_contract
        CHECK (
            selector_algorithm_version <> 'selector-v5'
            OR (task_risk_level IS NOT NULL AND effective_risk_ceiling IS NOT NULL)
        ),
    DROP CONSTRAINT IF EXISTS chk_framework_selection_records_risk_ceiling_rank,
    ADD CONSTRAINT chk_framework_selection_records_risk_ceiling_rank
        CHECK (
            task_risk_level IS NULL
            OR effective_risk_ceiling IS NULL
            OR CASE task_risk_level
                WHEN 'low' THEN 1
                WHEN 'medium' THEN 2
                WHEN 'high' THEN 3
               END
               <= CASE effective_risk_ceiling
                WHEN 'low' THEN 1
                WHEN 'medium' THEN 2
                WHEN 'high' THEN 3
               END
        );
