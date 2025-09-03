-- Platform listings with unique constraints
CREATE TABLE IF NOT EXISTS platform_listings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    lot_id UUID NOT NULL REFERENCES inventory(lot_id) ON DELETE CASCADE,
    platform platform_type NOT NULL,
    status listing_status DEFAULT 'not_listed',
    
    -- Listing details
    list_price DECIMAL(10, 2) CHECK (list_price > 0),
    listing_url TEXT,
    listing_title VARCHAR(255),
    listing_description TEXT,
    
    -- Sale details
    sold_price DECIMAL(10, 2),
    platform_fees DECIMAL(10, 2) DEFAULT 0,
    shipping_paid DECIMAL(10, 2) DEFAULT 0,
    actual_shipping DECIMAL(10, 2) DEFAULT 0,
    buyer_username VARCHAR(100),
    
    -- Dates
    listed_date TIMESTAMP WITH TIME ZONE,
    scheduled_date TIMESTAMP WITH TIME ZONE,
    sold_date TIMESTAMP WITH TIME ZONE,
    expired_date TIMESTAMP WITH TIME ZONE,
    
    -- Engagement metrics
    views INTEGER DEFAULT 0,
    watchers INTEGER DEFAULT 0,
    
    -- Platform-specific data
    metadata JSONB,
    
    -- Timestamps
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    
    -- Ensure only one active listing per platform
    CONSTRAINT unique_active_listing UNIQUE(lot_id, platform)
);

-- Composite indexes for common queries
CREATE INDEX idx_platform_listings_lot ON platform_listings(lot_id);
CREATE INDEX idx_platform_listings_platform_status ON platform_listings(platform, status);
CREATE INDEX idx_platform_listings_dates ON platform_listings(listed_date, sold_date);
