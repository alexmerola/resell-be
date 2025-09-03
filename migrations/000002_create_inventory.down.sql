-- Drop indexes (optional; DROP TABLE also removes them)
DROP INDEX IF EXISTS idx_inventory_not_deleted;
DROP INDEX IF EXISTS idx_inventory_search;
DROP INDEX IF EXISTS idx_inventory_date_range;
DROP INDEX IF EXISTS idx_inventory_storage;
DROP INDEX IF EXISTS idx_inventory_acquisition;
DROP INDEX IF EXISTS idx_inventory_condition;
DROP INDEX IF EXISTS idx_inventory_category;
DROP INDEX IF EXISTS idx_inventory_invoice;

-- Drop table
DROP TABLE IF EXISTS inventory;
