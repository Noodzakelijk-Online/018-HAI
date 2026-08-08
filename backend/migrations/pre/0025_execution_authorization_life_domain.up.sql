-- Preserve the life-domain routing input on the immutable authorization
-- receipt. Existing receipts remain valid and explicitly carry no domain.
ALTER TABLE public.execution_authorization_receipts
    ADD COLUMN IF NOT EXISTS domain character varying(64) NOT NULL DEFAULT '';

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'chk_execution_authorization_receipt_domain_length'
          AND conrelid = 'public.execution_authorization_receipts'::regclass
    ) THEN
        ALTER TABLE public.execution_authorization_receipts
            ADD CONSTRAINT chk_execution_authorization_receipt_domain_length
            CHECK (char_length(domain) <= 64);
    END IF;
END;
$$;
