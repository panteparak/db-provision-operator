-- Create least-privilege admin for DB Provision Operator
-- CockroachDB uses PostgreSQL wire protocol but has different privilege model:
-- - No SUPERUSER role
-- - Uses CREATEROLE and CREATEDB privileges
-- - Role-based access control similar to PostgreSQL
--
-- NOTE: Running in insecure mode - passwords not supported.
-- In production, use --certs-dir with TLS certificates.

-- Create the admin user (no password in insecure mode)
CREATE USER IF NOT EXISTS dbprovision_admin;

-- Grant role creation and database creation privileges
-- In CockroachDB, we use CREATEROLE option
ALTER USER dbprovision_admin CREATEROLE;

-- Grant admin role for full database management capabilities
-- CockroachDB has an 'admin' role similar to PostgreSQL's superuser
GRANT admin TO dbprovision_admin;

-- Verify setup
SELECT username, "isRole" FROM system.users WHERE username = 'dbprovision_admin';
