-- Knowledge base: pgvector collections and document chunks for RAG.
-- Embedding column is vector(1536): matches common OpenAI / OpenRouter embedding models.
-- Collections store logical dimensions/metadata; ingest must match this physical dimension for HNSW indexing.
-- Idempotent: safe when extension/tables/indexes already exist (e.g. from toolchain or prior apply).

CREATE EXTENSION IF NOT EXISTS vector;

CREATE TABLE IF NOT EXISTS kb_collections (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name text NOT NULL UNIQUE,
    dimensions integer NOT NULL,
    embedding_model text NOT NULL,
    metric text NOT NULL DEFAULT 'cosine',
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS kb_chunks (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    collection_id uuid NOT NULL REFERENCES kb_collections (id) ON DELETE CASCADE,
    source_uri text NOT NULL DEFAULT '',
    chunk_index integer NOT NULL DEFAULT 0,
    content text NOT NULL,
    metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
    embedding vector(1536) NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT kb_chunks_collection_source_chunk_unique UNIQUE (collection_id, source_uri, chunk_index)
);

CREATE INDEX IF NOT EXISTS idx_kb_chunks_collection_id ON kb_chunks (collection_id);

CREATE INDEX IF NOT EXISTS idx_kb_chunks_embedding_hnsw ON kb_chunks
    USING hnsw (embedding vector_cosine_ops);
