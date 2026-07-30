CREATE TABLE IF NOT EXISTS public.domain_pack_preferences (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    owner_identity character varying(255) NOT NULL,
    pack_id character varying(120) NOT NULL,
    catalog_version character varying(32) NOT NULL,
    revision bigint DEFAULT 1 NOT NULL,
    status character varying(32) DEFAULT 'active'::character varying NOT NULL,
    enabled boolean,
    classification_boost smallint DEFAULT 0 NOT NULL,
    force_local_only boolean DEFAULT false NOT NULL,
    adaptations_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL,
    CONSTRAINT domain_pack_preferences_pkey PRIMARY KEY (id),
    CONSTRAINT uq_domain_pack_preferences_owner_pack UNIQUE (owner_identity, pack_id),
    CONSTRAINT chk_domain_pack_preferences_owner CHECK (length(btrim(owner_identity)) > 0),
    CONSTRAINT chk_domain_pack_preferences_pack CHECK (length(btrim(pack_id)) > 0),
    CONSTRAINT chk_domain_pack_preferences_catalog CHECK (length(btrim(catalog_version)) > 0),
    CONSTRAINT chk_domain_pack_preferences_revision CHECK (revision > 0),
    CONSTRAINT chk_domain_pack_preferences_status
        CHECK (status::text = ANY (ARRAY['draft', 'active', 'archived']::text[])),
    CONSTRAINT chk_domain_pack_preferences_boost CHECK (classification_boost BETWEEN -25 AND 25),
    CONSTRAINT chk_domain_pack_preferences_adaptations CHECK (jsonb_typeof(adaptations_json) = 'object'),
    CONSTRAINT chk_domain_pack_preferences_time CHECK (updated_at >= created_at)
);

CREATE INDEX IF NOT EXISTS idx_domain_pack_preferences_owner_status
    ON public.domain_pack_preferences USING btree (owner_identity, status, pack_id);
