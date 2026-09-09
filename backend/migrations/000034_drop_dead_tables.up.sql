-- Dead tables: no Go code in nib or toolchain reads or writes any of these, in
-- any commit. Verified across nib, nib/frontend, nib/deploy, nib/agent-runner,
-- mcp-helm-chart and toolchain.
--
--   projects / environments / clouds / regions (000001-000003)
--     domain.Project, Cloud and Environment are backed by internal/filestore
--     (markdown + frontmatter), not by these tables. domain/location.go says it
--     "replaces the former Region concept", so regions is the fossil of a rename
--     and domain.Location has no table at all.
--
--   knowledge_connections / knowledge_collections (000017)
--     The abandoned DB-backed design of what is now config + kb_*. The live
--     KnowledgeConnection reads knowledge_base.uri from the config file, and
--     KnowledgeCollection is projected from kb_collections. kb_collections and
--     kb_chunks (000005) stay: they are read from three places.
--
-- The 000001-000003 and 000017 migration files are deliberately left in place:
-- 000001 is also where pgcrypto and vector are created, and a fresh database
-- still runs it before this file drops the tables again.

DROP TABLE IF EXISTS knowledge_collections;
DROP TABLE IF EXISTS knowledge_connections;

DROP TABLE IF EXISTS regions;
DROP TABLE IF EXISTS clouds;
DROP TABLE IF EXISTS environments;
DROP TABLE IF EXISTS projects;
