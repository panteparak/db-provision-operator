-- Create least-privilege admin for DB Provision Operator
-- This role has the minimum privileges needed to manage databases, users, and roles

CREATE ROLE dbprovision_admin WITH
    LOGIN
    CREATEDB
    CREATEROLE
    PASSWORD 'adminpassword123';

-- Grant connection termination capability (required for force-drop)
GRANT pg_signal_backend TO dbprovision_admin;

-- PostgreSQL 14+ has pg_read_all_data for backup operations
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'pg_read_all_data') THEN
        EXECUTE 'GRANT pg_read_all_data TO dbprovision_admin';
    END IF;
END $$;

-- Grant CONNECT on default database
GRANT CONNECT ON DATABASE postgres TO dbprovision_admin;

-- Verify setup
SELECT rolname, rolsuper, rolcreaterole, rolcreatedb, rolcanlogin
FROM pg_roles WHERE rolname = 'dbprovision_admin';
