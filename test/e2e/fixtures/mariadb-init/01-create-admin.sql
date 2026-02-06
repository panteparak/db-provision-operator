-- Create least-privilege admin for DB Provision Operator
-- This user has the minimum privileges needed to manage databases, users, and grants
-- Note: MariaDB uses slightly different syntax than MySQL for some operations

CREATE USER 'dbprovision_admin'@'%' IDENTIFIED BY 'adminpassword123';

-- Database operations
GRANT CREATE, DROP, ALTER ON *.* TO 'dbprovision_admin'@'%';

-- User management with grant option for role delegation
GRANT CREATE USER ON *.* TO 'dbprovision_admin'@'%' WITH GRANT OPTION;

-- System table access for metadata queries
GRANT SELECT ON mysql.user TO 'dbprovision_admin'@'%';
GRANT SELECT ON mysql.db TO 'dbprovision_admin'@'%';
GRANT SELECT ON mysql.tables_priv TO 'dbprovision_admin'@'%';
GRANT SELECT ON mysql.columns_priv TO 'dbprovision_admin'@'%';
GRANT SELECT ON mysql.procs_priv TO 'dbprovision_admin'@'%';

-- Administrative operations
GRANT RELOAD ON *.* TO 'dbprovision_admin'@'%';
GRANT PROCESS ON *.* TO 'dbprovision_admin'@'%';

-- Connection management (MariaDB 10.5.2+ uses CONNECTION ADMIN with space)
GRANT CONNECTION ADMIN ON *.* TO 'dbprovision_admin'@'%';

-- Data operations for managed databases and backup operations
-- INSERT, UPDATE, DELETE are needed to grant these privileges to application users
GRANT SELECT, INSERT, UPDATE, DELETE, SHOW VIEW, TRIGGER, LOCK TABLES ON *.* TO 'dbprovision_admin'@'%';
-- Note: information_schema access is automatic

FLUSH PRIVILEGES;

-- Verify setup
SHOW GRANTS FOR 'dbprovision_admin'@'%';
