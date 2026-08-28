-- Add vector embeddings to MCP tool catalog for hybrid semantic search.

ALTER TABLE mcp_tools ADD COLUMN IF NOT EXISTS embedding vector(1536);

CREATE INDEX IF NOT EXISTS idx_mcp_tools_embedding_hnsw ON mcp_tools
    USING hnsw (embedding vector_cosine_ops);
