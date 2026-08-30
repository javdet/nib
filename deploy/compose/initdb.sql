-- Runs once, on an empty data directory, before the app ever connects.
--
-- The backend registers the pgvector types in pgxpool's AfterConnect hook, which
-- fires on the very first connection — including the one pool.Ping uses, long
-- before migration 000001 gets a chance to CREATE EXTENSION. On a fresh volume
-- that hook fails with "vector type not found in the database" and the app exits.
-- Creating the extension at initdb time breaks that chicken-and-egg.
CREATE EXTENSION IF NOT EXISTS vector;
