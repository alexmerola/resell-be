-- Materialized view for Excel export with performance optimization
CREATE MATERIALIZED VIEW inventory_excel_export_mat AS
SELECT 
    i.lot_id,
    i.invoice_id,
    i.auction_id,
    i.item_name,
    i.description,
    i.category::text,
    i.subcategory,
    i.condition::text,
    i.quantity,
    i.bid_amount,
    i.buyers_premium,
    i.sales_tax,
    i.shipping_cost,
    i.total_cost,
    i.cost_per_item,
    i.acquisition_date,
    i.storage_location,
    i.storage_bin,
    i.estimated_value,
    i.market_demand::text,
    i.seasonality_notes,
    i.needs_repair,
    i.is_consignment,
    i.is_returned,
    i.keywords,
    i.notes,

    -- eBay
    BOOL_OR(pl.status = 'active')          FILTER (WHERE pl.platform = 'ebay')     AS ebay_listed,
    MAX(pl.list_price)                     FILTER (WHERE pl.platform = 'ebay')     AS ebay_price,
    MAX(pl.listing_url)                    FILTER (WHERE pl.platform = 'ebay')     AS ebay_url,
    BOOL_OR(pl.status = 'sold')            FILTER (WHERE pl.platform = 'ebay')     AS ebay_sold,

    -- Etsy
    BOOL_OR(pl.status = 'active')          FILTER (WHERE pl.platform = 'etsy')     AS etsy_listed,
    MAX(pl.list_price)                     FILTER (WHERE pl.platform = 'etsy')     AS etsy_price,
    MAX(pl.listing_url)                    FILTER (WHERE pl.platform = 'etsy')     AS etsy_url,
    BOOL_OR(pl.status = 'sold')            FILTER (WHERE pl.platform = 'etsy')     AS etsy_sold,

    -- Facebook
    BOOL_OR(pl.status = 'active')          FILTER (WHERE pl.platform = 'facebook') AS facebook_listed,
    MAX(pl.list_price)                     FILTER (WHERE pl.platform = 'facebook') AS facebook_price,
    MAX(pl.listing_url)                    FILTER (WHERE pl.platform = 'facebook') AS facebook_url,
    BOOL_OR(pl.status = 'sold')            FILTER (WHERE pl.platform = 'facebook') AS facebook_sold,

    -- Chairish 
    BOOL_OR(pl.status = 'active')          FILTER (WHERE pl.platform = 'chairish') AS chairish_listed,
    MAX(pl.list_price)                     FILTER (WHERE pl.platform = 'chairish') AS chairish_price,
    MAX(pl.listing_url)                    FILTER (WHERE pl.platform = 'chairish') AS chairish_url,
    BOOL_OR(pl.status = 'sold')            FILTER (WHERE pl.platform = 'chairish') AS chairish_sold,

    -- WorthPoint
    BOOL_OR(pl.status = 'active')          FILTER (WHERE pl.platform = 'worthpoint') AS worthpoint_listed,
    MAX(pl.list_price)                     FILTER (WHERE pl.platform = 'worthpoint') AS worthpoint_price,
    MAX(pl.listing_url)                    FILTER (WHERE pl.platform = 'worthpoint') AS worthpoint_url,
    BOOL_OR(pl.status = 'sold')            FILTER (WHERE pl.platform = 'worthpoint') AS worthpoint_sold,

    -- Local
    BOOL_OR(pl.status = 'active')          FILTER (WHERE pl.platform = 'local')    AS local_listed,
    MAX(pl.list_price)                     FILTER (WHERE pl.platform = 'local')    AS local_price,
    MAX(pl.listing_url)                    FILTER (WHERE pl.platform = 'local')    AS local_url,
    BOOL_OR(pl.status = 'sold')            FILTER (WHERE pl.platform = 'local')    AS local_sold,

    -- Calculated fields across all platforms
    MAX(pl.sold_price)                     AS sale_price,
    MAX(pl.sold_date)                      AS sale_date,
    SUM(pl.platform_fees)                  AS total_platform_fees,
    (MAX(pl.sold_price) - i.total_cost - COALESCE(SUM(pl.platform_fees), 0))       AS net_profit,
    CASE 
        WHEN i.total_cost > 0 AND MAX(pl.sold_price) IS NOT NULL THEN 
            ((MAX(pl.sold_price) - i.total_cost - COALESCE(SUM(pl.platform_fees), 0)) / i.total_cost * 100.0)
        ELSE NULL 
    END                                                                            AS roi_percent,
    CASE 
        WHEN MAX(pl.sold_date) IS NOT NULL THEN 
            EXTRACT(DAY FROM (MAX(pl.sold_date) - i.acquisition_date))
        ELSE NULL 
    END                                                                            AS days_to_sell,

    i.created_at,
    i.updated_at
FROM inventory i
LEFT JOIN platform_listings pl ON i.lot_id = pl.lot_id
WHERE i.deleted_at IS NULL
GROUP BY i.lot_id, i.invoice_id, i.auction_id, i.item_name, i.description, i.category, i.subcategory,
         i.condition, i.quantity, i.bid_amount, i.buyers_premium, i.sales_tax, i.shipping_cost,
         i.total_cost, i.cost_per_item, i.acquisition_date, i.storage_location, i.storage_bin,
         i.estimated_value, i.market_demand, i.seasonality_notes, i.needs_repair, i.is_consignment,
         i.is_returned, i.keywords, i.notes, i.created_at, i.updated_at;

-- Unique index required for CONCURRENTLY refreshes
CREATE UNIQUE INDEX IF NOT EXISTS idx_excel_export_lot ON inventory_excel_export_mat(lot_id);

-- Refresh strategy for materialized view
CREATE OR REPLACE FUNCTION refresh_excel_export_mat()
RETURNS void AS $$
BEGIN
    -- requires the unique index above
    REFRESH MATERIALIZED VIEW CONCURRENTLY inventory_excel_export_mat;
END;
$$ LANGUAGE plpgsql;