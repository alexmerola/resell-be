-- Enumerations
CREATE TYPE item_category AS ENUM (
    'antiques', 'art', 'books', 'ceramics', 'china', 'clothing',
    'coins', 'collectibles', 'electronics', 'furniture', 'glass',
    'jewelry', 'linens', 'memorabilia', 'musical', 'pottery',
    'silver', 'stamps', 'tools', 'toys', 'vintage', 'other'
);

CREATE TYPE item_condition AS ENUM (
    'mint', 'excellent', 'very_good', 'good', 'fair', 
    'poor', 'restoration', 'parts', 'unknown'
);

CREATE TYPE listing_status AS ENUM (
    'not_listed', 'draft', 'active', 'scheduled', 
    'sold', 'expired', 'cancelled', 'pending'
);

CREATE TYPE platform_type AS ENUM (
    'ebay', 'etsy', 'facebook', 'chairish', 
    'worthpoint', 'local', 'other'
);

CREATE TYPE market_demand_level AS ENUM ('low', 'medium', 'high');