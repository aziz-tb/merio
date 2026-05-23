-- Temporal needs two databases on the same Postgres instance:
--   * `temporal` — core data store (created by the POSTGRES_DB env var
--     when the container first starts).
--   * `temporal_visibility` — visibility store (workflow execution
--     queries by status, search attributes, etc.).
--
-- This script runs once, on first boot of the temporal-postgresql
-- volume, via docker-entrypoint-initdb.d. The temporalio/auto-setup
-- image then runs schema migrations against both DBs.
CREATE DATABASE temporal_visibility;
