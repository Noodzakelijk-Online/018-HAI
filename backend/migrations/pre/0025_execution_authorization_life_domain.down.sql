-- Do not silently discard immutable authorization provenance. Rollback is
-- allowed only before a receipt has recorded an explicit life domain.
DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM public.execution_authorization_receipts
        WHERE domain <> ''
        LIMIT 1
    ) THEN
        RAISE EXCEPTION 'refusing to discard execution authorization life domains';
    END IF;
END;
$$;

ALTER TABLE public.execution_authorization_receipts
    DROP CONSTRAINT IF EXISTS chk_execution_authorization_receipt_domain_length;

ALTER TABLE public.execution_authorization_receipts
    DROP COLUMN IF EXISTS domain;
