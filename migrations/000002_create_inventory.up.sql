-- Main inventory table with computed columns
CREATE TABLE IF NOT EXISTS inventory (
    lot_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    invoice_id VARCHAR(50) NOT NULL,
    auction_id INTEGER,
    item_name VARCHAR(255) NOT NULL,
    description TEXT,
    category item_category DEFAULT 'other',
    subcategory VARCHAR(100),
    condition item_condition DEFAULT 'unknown',
    quantity INTEGER DEFAULT 1 CHECK (quantity > 0),
    
    -- Financial fields
    bid_amount DECIMAL(10, 2) NOT NULL CHECK (bid_amount >= 0),
    buyers_premium DECIMAL(10, 2) DEFAULT 0,
    sales_tax DECIMAL(10, 2) DEFAULT 0,
    shipping_cost DECIMAL(10, 2) DEFAULT 0,
    total_cost DECIMAL(10, 2) GENERATED ALWAYS AS (
        COALESCE(bid_amount, 0) + 
        COALESCE(buyers_premium, 0) + 
        COALESCE(sales_tax, 0) +
        COALESCE(shipping_cost, 0)
    ) STORED,
    cost_per_item DECIMAL(10, 2) GENERATED ALWAYS AS (
        (COALESCE(bid_amount, 0) + COALESCE(buyers_premium, 0) + 
         COALESCE(sales_tax, 0) + COALESCE(shipping_cost, 0)) / NULLIF(quantity, 0)
    ) STORED,
    
    -- Dates
    acquisition_date TIMESTAMP WITH TIME ZONE NOT NULL,
    
    -- Storage
    storage_location VARCHAR(100),
    storage_bin VARCHAR(50),
    qr_code VARCHAR(100),
    
    -- Research & Valuation
    estimated_value DECIMAL(10, 2),
    market_demand market_demand_level DEFAULT 'medium',
    seasonality_notes TEXT,
    
    -- Status flags
    needs_repair BOOLEAN DEFAULT FALSE,
    is_consignment BOOLEAN DEFAULT FALSE,
    is_returned BOOLEAN DEFAULT FALSE,
    
    -- Search optimization
    search_vector tsvector GENERATED ALWAYS AS (
        setweight(to_tsvector('english', coalesce(item_name, '')), 'A') ||
        setweight(to_tsvector('english', coalesce(description, '')), 'B') ||
        setweight(to_tsvector('english', coalesce(keywords, '')), 'C')
    ) STORED,
    
    -- Metadata
    keywords TEXT,
    notes TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP WITH TIME ZONE -- Soft delete support
);

-- Optimized indexes
CREATE INDEX idx_inventory_invoice ON inventory(invoice_id);
CREATE INDEX idx_inventory_category ON inventory(category);
CREATE INDEX idx_inventory_condition ON inventory(condition);
CREATE INDEX idx_inventory_acquisition ON inventory(acquisition_date);
CREATE INDEX idx_inventory_storage ON inventory(storage_location, storage_bin);
CREATE INDEX idx_inventory_date_range ON inventory(acquisition_date, created_at);
CREATE INDEX idx_inventory_search ON inventory USING GIN(search_vector);
CREATE INDEX idx_inventory_not_deleted ON inventory(deleted_at) WHERE deleted_at IS NULL;
