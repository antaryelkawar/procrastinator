-- Runs as the `pgadmin` bootstrap superuser, and only on FIRST initialization
-- of an empty data volume (postgres image initdb convention — it never
-- re-runs on existing volumes).
--
-- Volumes that predate this script need a one-time dev reset:
--   docker compose down -v && docker compose up -d   (from procrastinator-backend/)
-- This wipes the dev data volume; dev data is disposable.
--
-- App DSNs are unchanged: the role keeps its name/password
-- (`procrastinator:procrastinator`); it is simply no longer a superuser.
CREATE ROLE procrastinator LOGIN PASSWORD 'procrastinator' NOBYPASSRLS;
ALTER DATABASE procrastinator OWNER TO procrastinator;
CREATE DATABASE procrastinator_test OWNER procrastinator;
