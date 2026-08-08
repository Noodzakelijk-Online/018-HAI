DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM public.pursuit_portfolio_dispatch_item_results LIMIT 1)
       OR EXISTS (SELECT 1 FROM public.pursuit_portfolio_dispatch_runs LIMIT 1) THEN
        RAISE EXCEPTION 'refusing to remove non-empty portfolio dispatch coordination ledgers';
    END IF;
END
$$;

DROP TRIGGER IF EXISTS pursuit_portfolio_dispatch_results_reject_truncate
    ON public.pursuit_portfolio_dispatch_item_results;
DROP TRIGGER IF EXISTS pursuit_portfolio_dispatch_results_reject_delete
    ON public.pursuit_portfolio_dispatch_item_results;
DROP TRIGGER IF EXISTS pursuit_portfolio_dispatch_results_reject_update
    ON public.pursuit_portfolio_dispatch_item_results;
DROP TRIGGER IF EXISTS pursuit_portfolio_dispatch_runs_reject_truncate
    ON public.pursuit_portfolio_dispatch_runs;
DROP TRIGGER IF EXISTS pursuit_portfolio_dispatch_runs_reject_delete
    ON public.pursuit_portfolio_dispatch_runs;
DROP TRIGGER IF EXISTS pursuit_portfolio_dispatch_runs_reject_update
    ON public.pursuit_portfolio_dispatch_runs;
DROP TRIGGER IF EXISTS pursuit_portfolio_dispatch_results_validate_insert
    ON public.pursuit_portfolio_dispatch_item_results;
DROP TRIGGER IF EXISTS pursuit_portfolio_dispatch_runs_validate_insert
    ON public.pursuit_portfolio_dispatch_runs;

DROP FUNCTION IF EXISTS public.reject_pursuit_portfolio_dispatch_mutation();
DROP FUNCTION IF EXISTS public.validate_pursuit_portfolio_dispatch_result_insert();
DROP FUNCTION IF EXISTS public.validate_pursuit_portfolio_dispatch_run_insert();
DROP TABLE IF EXISTS public.pursuit_portfolio_dispatch_item_results;
DROP TABLE IF EXISTS public.pursuit_portfolio_dispatch_runs;
