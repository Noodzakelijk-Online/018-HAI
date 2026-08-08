ALTER TABLE public.workflow_items
    ADD COLUMN mandate_id uuid;

ALTER TABLE public.workflow_items
    ADD CONSTRAINT fk_workflow_items_owner_mandate
    FOREIGN KEY (owner_identity, mandate_id)
    REFERENCES public.standing_mandates (owner_identity, id)
    ON UPDATE RESTRICT
    ON DELETE RESTRICT;

CREATE INDEX idx_workflow_items_owner_mandate
    ON public.workflow_items (owner_identity, mandate_id)
    WHERE mandate_id IS NOT NULL;

ALTER TABLE public.pursuits
    ADD COLUMN mandate_id uuid;

ALTER TABLE public.pursuits
    ADD CONSTRAINT fk_pursuits_owner_mandate
    FOREIGN KEY (owner_identity, mandate_id)
    REFERENCES public.standing_mandates (owner_identity, id)
    ON UPDATE RESTRICT
    ON DELETE RESTRICT;

CREATE INDEX idx_pursuits_owner_mandate
    ON public.pursuits (owner_identity, mandate_id)
    WHERE mandate_id IS NOT NULL;
