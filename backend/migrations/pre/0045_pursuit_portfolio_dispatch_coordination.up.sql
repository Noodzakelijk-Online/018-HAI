CREATE TABLE public.pursuit_portfolio_dispatch_runs (
    id uuid PRIMARY KEY DEFAULT public.uuid_generate_v4(),
    proposal_id uuid NOT NULL,
    owner_identity character varying(255) NOT NULL,
    proposal_digest character(64) NOT NULL,
    selected_item_ids jsonb NOT NULL DEFAULT '[]'::jsonb,
    selected_items_digest character(64) NOT NULL,
    request_digest character(64) NOT NULL,
    actor character varying(255) NOT NULL,
    confirmation character varying(255) NOT NULL,
    record_digest character(64) NOT NULL,
    requested_at timestamp with time zone NOT NULL,
    CONSTRAINT uq_pursuit_portfolio_dispatch_owner_request
        UNIQUE (owner_identity, request_digest),
    CONSTRAINT fk_pursuit_portfolio_dispatch_proposal
        FOREIGN KEY (proposal_id)
        REFERENCES public.pursuit_portfolio_execution_proposals (id)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT chk_pursuit_portfolio_dispatch_identity CHECK (
        char_length(btrim(owner_identity)) BETWEEN 1 AND 255
        AND actor = owner_identity
        AND confirmation = 'DISPATCH APPROVED PORTFOLIO WORKFLOWS'
    ),
    CONSTRAINT chk_pursuit_portfolio_dispatch_digests CHECK (
        proposal_digest ~ '^[0-9a-f]{64}$'
        AND selected_items_digest ~ '^[0-9a-f]{64}$'
        AND request_digest ~ '^[0-9a-f]{64}$'
        AND record_digest ~ '^[0-9a-f]{64}$'
    ),
    CONSTRAINT chk_pursuit_portfolio_dispatch_selection CHECK (
        jsonb_typeof(selected_item_ids) = 'array'
        AND jsonb_array_length(selected_item_ids) BETWEEN 1 AND 20
    )
);

CREATE INDEX idx_pursuit_portfolio_dispatch_runs_owner_proposal_time
    ON public.pursuit_portfolio_dispatch_runs
    (owner_identity, proposal_id, requested_at DESC, id DESC);

CREATE TABLE public.pursuit_portfolio_dispatch_item_results (
    id uuid PRIMARY KEY DEFAULT public.uuid_generate_v4(),
    dispatch_run_id uuid NOT NULL,
    proposal_id uuid NOT NULL,
    proposal_item_id uuid NOT NULL,
    owner_identity character varying(255) NOT NULL,
    attempt_number integer NOT NULL,
    proposal_item_digest character(64) NOT NULL,
    approval_decision_id uuid,
    approval_decision_digest character(64),
    outcome character varying(40) NOT NULL,
    message text NOT NULL,
    authorization_receipt_id uuid,
    workflow_id uuid,
    workflow_state character varying(80),
    replayed boolean NOT NULL DEFAULT false,
    record_digest character(64) NOT NULL,
    attempted_at timestamp with time zone NOT NULL,
    CONSTRAINT uq_pursuit_portfolio_dispatch_item_attempt
        UNIQUE (dispatch_run_id, proposal_item_id, attempt_number),
    CONSTRAINT fk_pursuit_portfolio_dispatch_item_run
        FOREIGN KEY (dispatch_run_id)
        REFERENCES public.pursuit_portfolio_dispatch_runs (id)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT fk_pursuit_portfolio_dispatch_item_proposal
        FOREIGN KEY (proposal_id)
        REFERENCES public.pursuit_portfolio_execution_proposals (id)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT fk_pursuit_portfolio_dispatch_item
        FOREIGN KEY (proposal_item_id)
        REFERENCES public.pursuit_portfolio_execution_proposal_items (id)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT fk_pursuit_portfolio_dispatch_item_decision
        FOREIGN KEY (approval_decision_id)
        REFERENCES public.pursuit_portfolio_execution_proposal_decisions (id)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT fk_pursuit_portfolio_dispatch_item_receipt
        FOREIGN KEY (authorization_receipt_id)
        REFERENCES public.execution_authorization_receipts (id)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT fk_pursuit_portfolio_dispatch_item_workflow
        FOREIGN KEY (workflow_id)
        REFERENCES public.workflow_items (id)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT chk_pursuit_portfolio_dispatch_item_attempt CHECK (
        attempt_number BETWEEN 1 AND 1000
        AND char_length(btrim(owner_identity)) BETWEEN 1 AND 255
        AND char_length(btrim(message)) BETWEEN 1 AND 4000
    ),
    CONSTRAINT chk_pursuit_portfolio_dispatch_item_outcome CHECK (
        outcome IN (
            'workflow_created', 'replayed', 'needs_approval', 'blocked',
            'stale', 'failed', 'cancelled'
        )
    ),
    CONSTRAINT chk_pursuit_portfolio_dispatch_item_digests CHECK (
        proposal_item_digest ~ '^[0-9a-f]{64}$'
        AND (
            (approval_decision_id IS NULL AND COALESCE(approval_decision_digest, '') = '')
            OR (
                approval_decision_id IS NOT NULL
                AND approval_decision_digest IS NOT NULL
                AND approval_decision_digest ~ '^[0-9a-f]{64}$'
            )
        )
        AND record_digest ~ '^[0-9a-f]{64}$'
    ),
    CONSTRAINT chk_pursuit_portfolio_dispatch_item_success CHECK (
        (authorization_receipt_id IS NULL OR approval_decision_id IS NOT NULL)
        AND (workflow_id IS NULL OR authorization_receipt_id IS NOT NULL)
        AND (
            (
                outcome IN ('workflow_created', 'replayed')
                AND approval_decision_id IS NOT NULL
                AND authorization_receipt_id IS NOT NULL
                AND workflow_id IS NOT NULL
                AND char_length(btrim(workflow_state)) BETWEEN 1 AND 80
            )
            OR (
                outcome NOT IN ('workflow_created', 'replayed')
                AND workflow_id IS NULL
                AND COALESCE(workflow_state, '') = ''
            )
        )
    )
);

CREATE INDEX idx_pursuit_portfolio_dispatch_results_owner_proposal_time
    ON public.pursuit_portfolio_dispatch_item_results
    (owner_identity, proposal_id, attempted_at DESC, id DESC);

CREATE INDEX idx_pursuit_portfolio_dispatch_results_run_item
    ON public.pursuit_portfolio_dispatch_item_results
    (dispatch_run_id, proposal_item_id, attempt_number DESC);

CREATE OR REPLACE FUNCTION public.validate_pursuit_portfolio_dispatch_run_insert()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    proposal_record public.pursuit_portfolio_execution_proposals%ROWTYPE;
    selected_id_text text;
    selected_count integer := 0;
BEGIN
    SELECT * INTO proposal_record
    FROM public.pursuit_portfolio_execution_proposals
    WHERE id = NEW.proposal_id
    FOR KEY SHARE;

    IF NOT FOUND
       OR proposal_record.owner_identity <> NEW.owner_identity
       OR proposal_record.record_digest <> NEW.proposal_digest THEN
        RAISE EXCEPTION 'portfolio dispatch does not match its immutable proposal';
    END IF;

    FOR selected_id_text IN SELECT jsonb_array_elements_text(NEW.selected_item_ids)
    LOOP
        selected_count := selected_count + 1;
        IF selected_id_text !~* '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$'
           OR NOT EXISTS (
                SELECT 1
                FROM public.pursuit_portfolio_execution_proposal_items
                WHERE id = selected_id_text::uuid
                  AND proposal_id = NEW.proposal_id
                  AND owner_identity = NEW.owner_identity
           ) THEN
            RAISE EXCEPTION 'portfolio dispatch contains an unavailable proposal item';
        END IF;
    END LOOP;

    IF selected_count <> (
        SELECT count(DISTINCT value)
        FROM jsonb_array_elements_text(NEW.selected_item_ids) AS selected(value)
    ) THEN
        RAISE EXCEPTION 'portfolio dispatch contains duplicate proposal items';
    END IF;

    RETURN NEW;
END;
$$;

CREATE OR REPLACE FUNCTION public.validate_pursuit_portfolio_dispatch_result_insert()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    run_record public.pursuit_portfolio_dispatch_runs%ROWTYPE;
    item_record public.pursuit_portfolio_execution_proposal_items%ROWTYPE;
    decision_record public.pursuit_portfolio_execution_proposal_decisions%ROWTYPE;
    receipt_record public.execution_authorization_receipts%ROWTYPE;
    workflow_record public.workflow_items%ROWTYPE;
BEGIN
    SELECT * INTO run_record
    FROM public.pursuit_portfolio_dispatch_runs
    WHERE id = NEW.dispatch_run_id
    FOR KEY SHARE;

    IF NOT FOUND
       OR run_record.proposal_id <> NEW.proposal_id
       OR run_record.owner_identity <> NEW.owner_identity
       OR NOT (run_record.selected_item_ids ? NEW.proposal_item_id::text) THEN
        RAISE EXCEPTION 'portfolio dispatch result does not match its immutable run';
    END IF;

    SELECT * INTO item_record
    FROM public.pursuit_portfolio_execution_proposal_items
    WHERE id = NEW.proposal_item_id
    FOR KEY SHARE;

    IF NOT FOUND
       OR item_record.proposal_id <> NEW.proposal_id
       OR item_record.owner_identity <> NEW.owner_identity
       OR item_record.record_digest <> NEW.proposal_item_digest THEN
        RAISE EXCEPTION 'portfolio dispatch result does not match its immutable proposal item';
    END IF;

    IF NEW.approval_decision_id IS NOT NULL THEN
        SELECT * INTO decision_record
        FROM public.pursuit_portfolio_execution_proposal_decisions
        WHERE id = NEW.approval_decision_id
        FOR KEY SHARE;
        IF NOT FOUND
           OR decision_record.proposal_item_id <> NEW.proposal_item_id
           OR decision_record.owner_identity <> NEW.owner_identity
           OR decision_record.record_digest <> NEW.approval_decision_digest
           OR decision_record.decision <> 'approved' THEN
            RAISE EXCEPTION 'portfolio dispatch result does not match an approved decision';
        END IF;
    END IF;

    IF NEW.authorization_receipt_id IS NOT NULL THEN
        SELECT * INTO receipt_record
        FROM public.execution_authorization_receipts
        WHERE id = NEW.authorization_receipt_id
        FOR KEY SHARE;
        IF NOT FOUND
           OR receipt_record.owner_identity <> NEW.owner_identity
           OR receipt_record.portfolio_proposal_decision_id IS DISTINCT FROM NEW.approval_decision_id
           OR receipt_record.outcome <> 'authorized' THEN
            RAISE EXCEPTION 'portfolio dispatch result does not match its authorization receipt';
        END IF;
    END IF;

    IF NEW.workflow_id IS NOT NULL THEN
        SELECT * INTO workflow_record
        FROM public.workflow_items
        WHERE id = NEW.workflow_id
        FOR KEY SHARE;
        IF NOT FOUND
           OR COALESCE(NULLIF(workflow_record.owner_identity, ''), 'system') <> NEW.owner_identity
           OR workflow_record.source_type <> 'portfolio_workflow_effect'
           OR workflow_record.source_id <> NEW.authorization_receipt_id::text
           OR workflow_record.current_state <> NEW.workflow_state THEN
            RAISE EXCEPTION 'portfolio dispatch result does not match its receipt-bound workflow';
        END IF;
    END IF;

    RETURN NEW;
END;
$$;

CREATE OR REPLACE FUNCTION public.reject_pursuit_portfolio_dispatch_mutation()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE EXCEPTION 'portfolio dispatch coordination records are append-only'
        USING ERRCODE = '55000';
END;
$$;

CREATE TRIGGER pursuit_portfolio_dispatch_runs_validate_insert
    BEFORE INSERT ON public.pursuit_portfolio_dispatch_runs
    FOR EACH ROW EXECUTE FUNCTION public.validate_pursuit_portfolio_dispatch_run_insert();
CREATE TRIGGER pursuit_portfolio_dispatch_results_validate_insert
    BEFORE INSERT ON public.pursuit_portfolio_dispatch_item_results
    FOR EACH ROW EXECUTE FUNCTION public.validate_pursuit_portfolio_dispatch_result_insert();

CREATE TRIGGER pursuit_portfolio_dispatch_runs_reject_update
    BEFORE UPDATE ON public.pursuit_portfolio_dispatch_runs
    FOR EACH ROW EXECUTE FUNCTION public.reject_pursuit_portfolio_dispatch_mutation();
CREATE TRIGGER pursuit_portfolio_dispatch_runs_reject_delete
    BEFORE DELETE ON public.pursuit_portfolio_dispatch_runs
    FOR EACH ROW EXECUTE FUNCTION public.reject_pursuit_portfolio_dispatch_mutation();
CREATE TRIGGER pursuit_portfolio_dispatch_runs_reject_truncate
    BEFORE TRUNCATE ON public.pursuit_portfolio_dispatch_runs
    FOR EACH STATEMENT EXECUTE FUNCTION public.reject_pursuit_portfolio_dispatch_mutation();

CREATE TRIGGER pursuit_portfolio_dispatch_results_reject_update
    BEFORE UPDATE ON public.pursuit_portfolio_dispatch_item_results
    FOR EACH ROW EXECUTE FUNCTION public.reject_pursuit_portfolio_dispatch_mutation();
CREATE TRIGGER pursuit_portfolio_dispatch_results_reject_delete
    BEFORE DELETE ON public.pursuit_portfolio_dispatch_item_results
    FOR EACH ROW EXECUTE FUNCTION public.reject_pursuit_portfolio_dispatch_mutation();
CREATE TRIGGER pursuit_portfolio_dispatch_results_reject_truncate
    BEFORE TRUNCATE ON public.pursuit_portfolio_dispatch_item_results
    FOR EACH STATEMENT EXECUTE FUNCTION public.reject_pursuit_portfolio_dispatch_mutation();
