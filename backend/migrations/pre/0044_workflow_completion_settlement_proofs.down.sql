DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM public.pursuit_portfolio_workflow_settlement_proofs
        LIMIT 1
    ) OR EXISTS (
        SELECT 1
        FROM public.workflow_completion_attestations
        LIMIT 1
    ) THEN
        RAISE EXCEPTION
            'refusing to remove non-empty workflow completion and portfolio settlement proof ledgers';
    END IF;
END
$$;

DROP TRIGGER IF EXISTS pursuit_portfolio_workflow_settlement_proofs_reject_truncate
    ON public.pursuit_portfolio_workflow_settlement_proofs;
DROP TRIGGER IF EXISTS pursuit_portfolio_workflow_settlement_proofs_reject_delete
    ON public.pursuit_portfolio_workflow_settlement_proofs;
DROP TRIGGER IF EXISTS pursuit_portfolio_workflow_settlement_proofs_reject_update
    ON public.pursuit_portfolio_workflow_settlement_proofs;
DROP TRIGGER IF EXISTS pursuit_portfolio_workflow_settlement_proofs_validate_insert
    ON public.pursuit_portfolio_workflow_settlement_proofs;
DROP FUNCTION IF EXISTS public.validate_portfolio_workflow_settlement_proof_insert();
DROP TABLE IF EXISTS public.pursuit_portfolio_workflow_settlement_proofs;

DROP TRIGGER IF EXISTS workflow_completion_attestations_reject_truncate
    ON public.workflow_completion_attestations;
DROP TRIGGER IF EXISTS workflow_completion_attestations_reject_delete
    ON public.workflow_completion_attestations;
DROP TRIGGER IF EXISTS workflow_completion_attestations_reject_update
    ON public.workflow_completion_attestations;
DROP TRIGGER IF EXISTS workflow_completion_attestations_validate_insert
    ON public.workflow_completion_attestations;
DROP FUNCTION IF EXISTS public.reject_workflow_completion_settlement_proof_mutation();
DROP FUNCTION IF EXISTS public.validate_workflow_completion_attestation_insert();
DROP TABLE IF EXISTS public.workflow_completion_attestations;
