-- Remove helper function first (it references the mat view)
DROP FUNCTION IF EXISTS refresh_excel_export_mat();

-- Drop index on the materialized view (optional; DROP MATERIALIZED VIEW also removes it)
DROP INDEX IF EXISTS idx_excel_export_lot;

-- Drop the materialized view
DROP MATERIALIZED VIEW IF EXISTS inventory_excel_export_mat;
