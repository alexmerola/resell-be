-- Drop enum types (should be run after dependent tables are gone)
DROP TYPE IF EXISTS market_demand_level;
DROP TYPE IF EXISTS platform_type;
DROP TYPE IF EXISTS listing_status;
DROP TYPE IF EXISTS item_condition;
DROP TYPE IF EXISTS item_category;
