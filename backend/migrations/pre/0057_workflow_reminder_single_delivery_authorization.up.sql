ALTER TABLE public.workflow_reminder_delivery_authorizations
    ADD CONSTRAINT uq_workflow_reminder_delivery_single_authorization
    UNIQUE (owner_identity, activation_request_id, activation_decision_id, channel);
