ALTER TABLE public.operations
    ADD CONSTRAINT uq_operations_owner_workspace_id
    UNIQUE (owner_user_id, workspace_id, id);

CREATE TABLE public.evidence_packs (
    id uuid NOT NULL,
    owner_identity text NOT NULL,
    workspace_id text NOT NULL,
    operation_id uuid NOT NULL,
    title text NOT NULL,
    markdown text NOT NULL,
    source_type text NOT NULL,
    source_id uuid,
    source_uri text NOT NULL DEFAULT '',
    source_received_at timestamp with time zone,
    source_revision_hash text NOT NULL DEFAULT '',
    dedupe_key text NOT NULL,
    content_digest character varying(71) NOT NULL,
    generated_at timestamp with time zone NOT NULL,
    CONSTRAINT evidence_packs_pkey PRIMARY KEY (id),
    CONSTRAINT uq_evidence_packs_owner_workspace_id
        UNIQUE (owner_identity, workspace_id, id),
    CONSTRAINT fk_evidence_packs_operation_owner_workspace
        FOREIGN KEY (owner_identity, workspace_id, operation_id)
        REFERENCES public.operations (owner_user_id, workspace_id, id)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT chk_evidence_packs_scope CHECK (
        char_length(btrim(owner_identity)) BETWEEN 1 AND 256
        AND char_length(btrim(workspace_id)) BETWEEN 1 AND 256
    ),
    CONSTRAINT chk_evidence_packs_content CHECK (
        char_length(btrim(title)) BETWEEN 1 AND 4096
        AND octet_length(markdown) BETWEEN 1 AND 2097152
        AND char_length(btrim(source_type)) BETWEEN 1 AND 256
        AND char_length(btrim(dedupe_key)) BETWEEN 1 AND 4096
    ),
    CONSTRAINT chk_evidence_packs_provenance CHECK (
        char_length(source_uri) <= 16384
        AND char_length(source_revision_hash) <= 4096
    ),
    CONSTRAINT chk_evidence_packs_digest CHECK (
        content_digest ~ '^sha256:[0-9a-f]{64}$'
    )
);

CREATE INDEX idx_evidence_packs_owner_workspace_generated
    ON public.evidence_packs
    (owner_identity, workspace_id, generated_at DESC, id DESC);

CREATE INDEX idx_evidence_packs_owner_workspace_operation
    ON public.evidence_packs
    (owner_identity, workspace_id, operation_id, generated_at DESC);

CREATE INDEX idx_evidence_packs_source_revision
    ON public.evidence_packs
    (source_revision_hash)
    WHERE source_revision_hash <> '';

CREATE OR REPLACE FUNCTION public.hai_reject_evidence_pack_mutation()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE EXCEPTION 'evidence packs are immutable'
        USING ERRCODE = 'integrity_constraint_violation';
END;
$$;

CREATE TRIGGER trg_evidence_packs_immutable
    BEFORE UPDATE OR DELETE ON public.evidence_packs
    FOR EACH ROW
    EXECUTE FUNCTION public.hai_reject_evidence_pack_mutation();

CREATE TRIGGER trg_evidence_packs_no_truncate
    BEFORE TRUNCATE ON public.evidence_packs
    FOR EACH STATEMENT
    EXECUTE FUNCTION public.hai_reject_evidence_pack_mutation();
