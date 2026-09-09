-- kb_chunks.embedding is vector(1536) and the HNSW index spans exactly that
-- width, but kb_collections.dimensions was free text: an embedder returning a
-- different width committed a collection row of its own size, then failed every
-- insert against the fixed column. Nothing repaired that row, so the collection
-- 500'd forever. Fail at collection-creation time instead, before the first
-- write, while the failure is still one loud error.
--
-- Supporting several widths for real means one chunk table (or one vector(N)
-- column) per width -- an architecture change, not a migration.

ALTER TABLE kb_collections
    ADD CONSTRAINT kb_collections_dimensions_supported
        CHECK (dimensions = 1536) NOT VALID;

ALTER TABLE kb_collections VALIDATE CONSTRAINT kb_collections_dimensions_supported;
