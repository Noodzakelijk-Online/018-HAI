-- uuid_generate_v4() backs the default for UUID primary keys, so the extension
-- must exist before any table is created (AutoMigrate or baseline migration).
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
