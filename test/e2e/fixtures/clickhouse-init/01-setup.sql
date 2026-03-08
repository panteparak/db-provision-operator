-- Create admin user for operator with full access management privileges
CREATE USER IF NOT EXISTS admin IDENTIFIED BY 'admin_password' HOST ANY;
GRANT ALL ON *.* TO admin WITH GRANT OPTION;
