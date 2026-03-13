-- Create admin user for operator E2E tests
CREATE USER IF NOT EXISTS dbprovision_admin IDENTIFIED BY 'adminpassword123';
GRANT ALL ON *.* TO dbprovision_admin WITH GRANT OPTION;
