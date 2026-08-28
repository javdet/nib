CREATE TABLE IF NOT EXISTS knowledge_connections (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    type VARCHAR(50) NOT NULL DEFAULT 'pgvector',
    connection_uri TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS knowledge_collections (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    connection_id UUID NOT NULL REFERENCES knowledge_connections(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    dimensions INTEGER NOT NULL,
    metric VARCHAR(50) NOT NULL DEFAULT 'cosine',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(connection_id, name)
);
