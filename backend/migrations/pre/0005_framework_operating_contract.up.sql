ALTER TABLE public.framework_selection_records
    ADD COLUMN IF NOT EXISTS life_domains_json jsonb DEFAULT '[]'::jsonb NOT NULL,
    ADD COLUMN IF NOT EXISTS needs_state_json jsonb DEFAULT '[]'::jsonb NOT NULL,
    ADD COLUMN IF NOT EXISTS capacity_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    ADD COLUMN IF NOT EXISTS agent_cards_json jsonb DEFAULT '[]'::jsonb NOT NULL,
    ADD COLUMN IF NOT EXISTS delegations_json jsonb DEFAULT '[]'::jsonb NOT NULL,
    ADD COLUMN IF NOT EXISTS communication_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    ADD COLUMN IF NOT EXISTS coordination_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    ADD COLUMN IF NOT EXISTS action_autonomy_json jsonb DEFAULT '[]'::jsonb NOT NULL,
    ADD COLUMN IF NOT EXISTS stop_conditions_json jsonb DEFAULT '[]'::jsonb NOT NULL,
    ADD COLUMN IF NOT EXISTS outcome_monitoring_json jsonb DEFAULT '[]'::jsonb NOT NULL,
    ADD COLUMN IF NOT EXISTS chief_of_staff_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    ADD COLUMN IF NOT EXISTS operating_contract_digest character(64)
        DEFAULT '0000000000000000000000000000000000000000000000000000000000000000'::bpchar
        NOT NULL;

ALTER TABLE public.framework_selection_records
    DROP CONSTRAINT IF EXISTS chk_framework_selection_records_life_domains_array,
    ADD CONSTRAINT chk_framework_selection_records_life_domains_array
        CHECK (jsonb_typeof(life_domains_json) = 'array'),
    DROP CONSTRAINT IF EXISTS chk_framework_selection_records_needs_state_array,
    ADD CONSTRAINT chk_framework_selection_records_needs_state_array
        CHECK (jsonb_typeof(needs_state_json) = 'array'),
    DROP CONSTRAINT IF EXISTS chk_framework_selection_records_capacity_object,
    ADD CONSTRAINT chk_framework_selection_records_capacity_object
        CHECK (jsonb_typeof(capacity_json) = 'object'),
    DROP CONSTRAINT IF EXISTS chk_framework_selection_records_agent_cards_array,
    ADD CONSTRAINT chk_framework_selection_records_agent_cards_array
        CHECK (jsonb_typeof(agent_cards_json) = 'array'),
    DROP CONSTRAINT IF EXISTS chk_framework_selection_records_delegations_array,
    ADD CONSTRAINT chk_framework_selection_records_delegations_array
        CHECK (jsonb_typeof(delegations_json) = 'array'),
    DROP CONSTRAINT IF EXISTS chk_framework_selection_records_communication_object,
    ADD CONSTRAINT chk_framework_selection_records_communication_object
        CHECK (jsonb_typeof(communication_json) = 'object'),
    DROP CONSTRAINT IF EXISTS chk_framework_selection_records_coordination_object,
    ADD CONSTRAINT chk_framework_selection_records_coordination_object
        CHECK (jsonb_typeof(coordination_json) = 'object'),
    DROP CONSTRAINT IF EXISTS chk_framework_selection_records_action_autonomy_array,
    ADD CONSTRAINT chk_framework_selection_records_action_autonomy_array
        CHECK (jsonb_typeof(action_autonomy_json) = 'array'),
    DROP CONSTRAINT IF EXISTS chk_framework_selection_records_stop_conditions_array,
    ADD CONSTRAINT chk_framework_selection_records_stop_conditions_array
        CHECK (jsonb_typeof(stop_conditions_json) = 'array'),
    DROP CONSTRAINT IF EXISTS chk_framework_selection_records_outcome_monitoring_array,
    ADD CONSTRAINT chk_framework_selection_records_outcome_monitoring_array
        CHECK (jsonb_typeof(outcome_monitoring_json) = 'array'),
    DROP CONSTRAINT IF EXISTS chk_framework_selection_records_chief_of_staff_object,
    ADD CONSTRAINT chk_framework_selection_records_chief_of_staff_object
        CHECK (jsonb_typeof(chief_of_staff_json) = 'object'),
    DROP CONSTRAINT IF EXISTS chk_framework_selection_records_operating_digest,
    ADD CONSTRAINT chk_framework_selection_records_operating_digest
        CHECK (
            operating_contract_digest ~ '^[0-9a-f]{64}$'
            AND (
                selector_algorithm_version <> 'selector-v4'
                OR operating_contract_digest <>
                    '0000000000000000000000000000000000000000000000000000000000000000'
            )
        );

CREATE INDEX IF NOT EXISTS idx_framework_selection_records_operating_digest
    ON public.framework_selection_records USING btree (operating_contract_digest);
