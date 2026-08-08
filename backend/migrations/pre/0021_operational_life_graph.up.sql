ALTER TABLE public.life_ontology_entities
    DROP CONSTRAINT chk_life_ontology_entity_type,
    ADD CONSTRAINT chk_life_ontology_entity_type CHECK (
        entity_type IN (
            'person', 'need', 'goal', 'asset', 'obligation',
            'project', 'case', 'opportunity', 'risk', 'source',
            'document', 'pursuit', 'workflow', 'task', 'memory',
            'commitment', 'cost', 'outcome'
        )
    );

ALTER TABLE public.life_ontology_relations
    DROP CONSTRAINT chk_life_ontology_relation_type,
    ADD CONSTRAINT chk_life_ontology_relation_type CHECK (
        relation_type IN (
            'has_need', 'pursues_goal', 'owns_asset', 'owes_obligation',
            'advances', 'belongs_to_project', 'related_to_case',
            'creates_opportunity', 'threatens', 'mitigates',
            'depends_on', 'supports', 'conflicts_with', 'derived_from',
            'documents', 'produces', 'fulfills', 'assigned_to', 'requires',
            'incurs_cost', 'belongs_to_pursuit', 'belongs_to_workflow'
        )
    );
