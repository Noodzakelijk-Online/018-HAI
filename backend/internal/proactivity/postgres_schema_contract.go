package proactivity

// PostgresSchemaContract is executable documentation for the future migration.
// Production startup must fail until these relations exist; the repository
// never creates, repairs, or falls back around missing durable storage.
const PostgresSchemaContract = `
CREATE TABLE public.proactivity_idempotency (
    owner_identity text NOT NULL CHECK (length(owner_identity) BETWEEN 1 AND 320),
    idempotency_key text NOT NULL CHECK (length(idempotency_key) BETWEEN 1 AND 200),
    record_kind text NOT NULL CHECK (record_kind IN ('policy', 'signals', 'decisions')),
    payload_digest char(64) NOT NULL CHECK (payload_digest ~ '^[a-f0-9]{64}$'),
    recorded_at timestamptz NOT NULL,
    PRIMARY KEY (owner_identity, idempotency_key),
    UNIQUE (owner_identity, idempotency_key, record_kind, payload_digest)
);

CREATE TABLE public.proactivity_policy_records (
    owner_identity text NOT NULL,
    idempotency_key text NOT NULL,
    record_kind text NOT NULL DEFAULT 'policy' CHECK (record_kind = 'policy'),
    payload_digest char(64) NOT NULL,
    recorded_at timestamptz NOT NULL,
    payload jsonb NOT NULL CHECK (jsonb_typeof(payload) = 'object'),
    PRIMARY KEY (owner_identity, idempotency_key),
    FOREIGN KEY (owner_identity, idempotency_key, record_kind, payload_digest)
        REFERENCES public.proactivity_idempotency(owner_identity, idempotency_key, record_kind, payload_digest)
        ON DELETE RESTRICT,
    CHECK (payload_digest ~ '^[a-f0-9]{64}$')
);
CREATE INDEX proactivity_policy_owner_recorded_idx
    ON public.proactivity_policy_records(owner_identity, recorded_at DESC, idempotency_key DESC);

CREATE TABLE public.proactivity_signal_batches (
    owner_identity text NOT NULL,
    idempotency_key text NOT NULL,
    record_kind text NOT NULL DEFAULT 'signals' CHECK (record_kind = 'signals'),
    payload_digest char(64) NOT NULL,
    signal_count integer NOT NULL CHECK (signal_count BETWEEN 1 AND 256),
    recorded_at timestamptz NOT NULL,
    payload jsonb NOT NULL CHECK (jsonb_typeof(payload) = 'array'),
    PRIMARY KEY (owner_identity, idempotency_key),
    FOREIGN KEY (owner_identity, idempotency_key, record_kind, payload_digest)
        REFERENCES public.proactivity_idempotency(owner_identity, idempotency_key, record_kind, payload_digest)
        ON DELETE RESTRICT,
    CHECK (payload_digest ~ '^[a-f0-9]{64}$')
);
CREATE TABLE public.proactivity_signal_records (
    owner_identity text NOT NULL,
    batch_idempotency_key text NOT NULL,
    ordinal integer NOT NULL CHECK (ordinal BETWEEN 0 AND 255),
    signal_id text NOT NULL CHECK (length(signal_id) BETWEEN 1 AND 200),
    recorded_at timestamptz NOT NULL,
    payload jsonb NOT NULL CHECK (jsonb_typeof(payload) = 'object'),
    PRIMARY KEY (owner_identity, batch_idempotency_key, ordinal),
    FOREIGN KEY (owner_identity, batch_idempotency_key)
        REFERENCES public.proactivity_signal_batches(owner_identity, idempotency_key)
        ON DELETE RESTRICT
);
CREATE INDEX proactivity_signal_owner_history_idx
    ON public.proactivity_signal_records(owner_identity, recorded_at DESC, batch_idempotency_key DESC, ordinal DESC);
CREATE INDEX proactivity_signal_owner_latest_idx
    ON public.proactivity_signal_records(owner_identity, signal_id, recorded_at DESC, batch_idempotency_key DESC, ordinal DESC);

CREATE TABLE public.proactivity_decision_batches (
    owner_identity text NOT NULL,
    idempotency_key text NOT NULL,
    record_kind text NOT NULL DEFAULT 'decisions' CHECK (record_kind = 'decisions'),
    payload_digest char(64) NOT NULL,
    decision_count integer NOT NULL CHECK (decision_count BETWEEN 0 AND 256),
    recorded_at timestamptz NOT NULL,
    payload jsonb NOT NULL CHECK (jsonb_typeof(payload) = 'object'),
    PRIMARY KEY (owner_identity, idempotency_key),
    FOREIGN KEY (owner_identity, idempotency_key, record_kind, payload_digest)
        REFERENCES public.proactivity_idempotency(owner_identity, idempotency_key, record_kind, payload_digest)
        ON DELETE RESTRICT,
    CHECK (payload_digest ~ '^[a-f0-9]{64}$')
);
CREATE TABLE public.proactivity_decision_records (
    owner_identity text NOT NULL,
    batch_idempotency_key text NOT NULL,
    ordinal integer NOT NULL CHECK (ordinal BETWEEN 0 AND 255),
    signal_id text NOT NULL CHECK (length(signal_id) BETWEEN 1 AND 200),
    open_loop_key text NOT NULL CHECK (length(open_loop_key) BETWEEN 1 AND 200),
    outcome text NOT NULL CHECK (outcome IN ('suppress', 'ambient', 'daily_brief', 'notify', 'require_review')),
    recorded_at timestamptz NOT NULL,
    payload jsonb NOT NULL CHECK (jsonb_typeof(payload) = 'object'),
    PRIMARY KEY (owner_identity, batch_idempotency_key, ordinal),
    FOREIGN KEY (owner_identity, batch_idempotency_key)
        REFERENCES public.proactivity_decision_batches(owner_identity, idempotency_key)
        ON DELETE RESTRICT
);
CREATE INDEX proactivity_decision_owner_history_idx
    ON public.proactivity_decision_records(owner_identity, recorded_at DESC, batch_idempotency_key DESC, ordinal DESC);

CREATE TABLE public.proactivity_feedback_records (
    owner_identity text NOT NULL,
    feedback_id uuid NOT NULL,
    idempotency_key text NOT NULL,
    request_digest char(64) NOT NULL,
    signal_id text NOT NULL,
    open_loop_key text NOT NULL,
    signal_digest char(64) NOT NULL,
    source_outcome text NOT NULL,
    source_decision_at timestamptz NOT NULL,
    action text NOT NULL,
    snoozed_until timestamptz,
    previous_record_digest char(64),
    record_digest char(64) NOT NULL,
    recorded_at timestamptz NOT NULL,
    authority text NOT NULL CHECK (authority = 'attention_feedback_only'),
    can_execute boolean NOT NULL CHECK (can_execute = false),
    delivery_authorized boolean NOT NULL CHECK (delivery_authorized = false),
    execution_authorized boolean NOT NULL CHECK (execution_authorized = false),
    payload jsonb NOT NULL,
    PRIMARY KEY (owner_identity, feedback_id),
    UNIQUE (owner_identity, idempotency_key)
);
`
