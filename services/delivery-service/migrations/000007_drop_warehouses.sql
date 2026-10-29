-- Drop warehouses table from delivery service as warehouse management is moved to inventory service
DROP TABLE IF EXISTS warehouses CASCADE;
