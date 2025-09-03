-- Drop indexes (optional; DROP TABLE also removes them)
DROP INDEX IF EXISTS idx_platform_listings_dates;
DROP INDEX IF EXISTS idx_platform_listings_platform_status;
DROP INDEX IF EXISTS idx_platform_listings_lot;

-- Drop table
DROP TABLE IF EXISTS platform_listings;
