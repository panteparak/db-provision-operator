-- Create least-privilege admin for DB Provision Operator
-- This user has the minimum privileges needed to manage databases, users, and grants

CREATE USER 'dbprovision_admin'@'%' IDENTIFIED BY 'adminpassword123';

-- Database operations
GRANT CREATE, DROP, ALTER ON *.* TO 'dbprovision_admin'@'%';

-- User management
GRANT CREATE USER ON *.* TO 'dbprovision_admin'@'%';
GRANT GRANT OPTION ON *.* TO 'dbprovision_admin'@'%';

-- System table access for metadata queries
GRANT SELECT ON mysql.user TO 'dbprovision_admin'@'%';
GRANT SELECT ON mysql.db TO 'dbprovision_admin'@'%';
GRANT SELECT ON mysql.tables_priv TO 'dbprovision_admin'@'%';
GRANT SELECT ON mysql.columns_priv TO 'dbprovision_admin'@'%';
GRANT SELECT ON mysql.procs_priv TO 'dbprovision_admin'@'%';
GRANT SELECT ON mysql.global_grants TO 'dbprovision_admin'@'%';

-- Administrative operations
GRANT RELOAD ON *.* TO 'dbprovision_admin'@'%';
GRANT PROCESS ON *.* TO 'dbprovision_admin'@'%';

-- Connection management (for force-drop)
GRANT CONNECTION_ADMIN ON *.* TO 'dbprovision_admin'@'%';

-- Role management (MySQL 8.0+)
GRANT ROLE_ADMIN ON *.* TO 'dbprovision_admin'@'%';

-- Data operations for managed databases and backup operations
-- INSERT, UPDATE, DELETE are needed to grant these privileges to application users
GRANT SELECT, INSERT, UPDATE, DELETE, SHOW VIEW, TRIGGER, LOCK TABLES ON *.* TO 'dbprovision_admin'@'%';
-- Note: information_schema and performance_schema access is automatic

FLUSH PRIVILEGES;

-- Verify setup
SHOW GRANTS FOR 'dbprovision_admin'@'%';
