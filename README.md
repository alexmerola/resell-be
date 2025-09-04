Of course. I have thoroughly reviewed your codebase and your existing `DESIGN.MD` document.

You've built a robust and well-architected system. The original design document was an excellent blueprint. As you suspected, some parts had become slightly out of sync with the implementation, and some aspirational sections could be refined to reflect the current state of the codebase.

I have updated the document to serve as an accurate, comprehensive, and forward-looking guide to your project as it stands today. The new version synchronizes the database schema with your migrations, aligns the API documentation with your implemented handlers, and updates the technical stack and development instructions to match the provided code. It retains the excellent structure and clarity of your original while ensuring it is a perfect reflection of your work.

Here is the newly updated `DESIGN.MD` document.

***

# Resell Inventory Management System v2.0

## 🚀 Executive Summary

A professional-grade inventory management system for reselling businesses, featuring **asynchronous data ingestion**, **advanced inventory tracking**, and **real-time analytics dashboards**. Built with a modern Go backend (**Go 1.22+**) leveraging enhanced standard library routing, a high-performance **PostgreSQL 16** database, structured logging with `slog`, and fully containerized for seamless local development and cloud-native deployment.

---

## 📋 Table of Contents

- [✨ Features](#-features)
- [🏗 Architecture](#-architecture)
- [🛠 Tech Stack](#-tech-stack)
- [📊 Database Schema](#-database-schema)
- [📡 API Documentation](#-api-documentation)
- [📁 Project Structure](#-project-structure)
- [🚀 Installation & Setup](#-installation--setup)
- [💻 Development](#-development)
- [🧪 Testing](#-testing)
- [📦 Deployment](#-deployment)
- [📊 Monitoring](#-monitoring)
- [🔒 Security](#-security)
- [🤝 Contributing](#-contributing)
- [📝 License](#-license)
- [🚀 Roadmap](#-roadmap)

---

## ✨ Features

### Core Functionality

- 📑 **Asynchronous Batch Processing**: Robust, queue-based ingestion of PDF invoices and Excel files using Asynq for non-blocking, reliable data extraction.
- 📦 **Comprehensive Inventory Tracking**: Detailed records of each item, including financial data, acquisition history, and physical storage location.
- 📊 **Real-time Dashboard**: Key metrics and analytics served via a high-performance API, with caching for speed.
- 🧠 **Heuristic-Based Classification**: An intelligent seeder classifies items by category and condition based on description keywords.
- 🔍 **Advanced Search**: Full-text search capabilities powered by PostgreSQL's `tsvector` and GIN indexes.
- 💰 **Financial Analytics**: Automated calculation of total cost, cost-per-item, and profit margins.
- 🗄 **Storage Management**: Track the physical location of inventory with fields ready for QR code integration.
- ➗ **Automated Calculations**: Database-level generated columns ensure financial data consistency and accuracy.
- 📤 **Bulk Data Export**: Streamlined exporting to standardized JSON or Excel formats directly from the API.

### Business Intelligence

- 🚀 **Performance-Optimized Reporting**: A dedicated materialized view (`inventory_excel_export_mat`) pre-aggregates data for lightning-fast exports and analytics.
- 📊 **Category Performance Analysis**: Grouping and aggregation capabilities to analyze profitability and sales velocity by category.
- 🔄 **Platform Profitability Comparison**: Schema designed to track listings, sales, and fees across multiple platforms like eBay, Etsy, and more.
- ⏳ **Aging Inventory Insights**: Track acquisition and sale dates to identify slow-moving stock.

### Technical Excellence

- 🌐 **Modern RESTful API**: Built with Go 1.22's enhanced `http.ServeMux` for method-specific routing and path wildcards.
- 📝 **Structured Logging**: Comprehensive, context-rich logging using the standard library's `slog` for superior observability.
- 🗄 **Optimized Database**: A normalized PostgreSQL schema featuring generated columns, GIN indexes, and materialized views for read performance.
- 🔄 **Async Job Processing**: Redis-backed Asynq task queue for handling computationally expensive tasks like file parsing without blocking the API.
- 🏭 **Repository Pattern**: Clean separation of concerns between business logic and data access layers.
- 🐳 **Containerized Environment**: A complete, cross-platform local development environment managed with Docker Compose.
- 📡 **Observability**: Built-in health check endpoints providing deep insights into the status of the application and its dependencies.
- 🔒 **Secure by Design**: Middleware-driven security including rate limiting, CORS policies, secure headers, and panic recovery.
- ⚡ **High Performance**: Utilizes the `pgx/v5` driver, connection pooling, and Redis caching for critical endpoints.

---

## 🏗 Architecture

### High-Level Architecture

```mermaid
graph TB
    subgraph "Input Sources"
        PDF[PDF Invoices]
        EXCEL[Excel Files]
        WEB[Web Dashboard]
        API[3rd Party API]
    end
    
    subgraph "Application Layer (Go Backend)"
        ROUTER[Go 1.22 Router + Middleware]
        HANDLERS[API Handlers]
        SERVICES[Business Logic Services]
        REPOS[Data Repositories]
    end
    
    subgraph "Asynchronous Processing"
        ASYNQ_CLIENT[Asynq Client]
        ASYNQ_QUEUE[(Redis Queue)]
        WORKERS[Asynq Workers]
    end
    
    subgraph "Data & Storage Layer"
        POSTGRES[(PostgreSQL 16)]
        REDIS_CACHE[(Redis 7.2 Cache)]
        S3[S3-Compatible Storage (MinIO)]
    end
    
    subgraph "Output & Consumption"
        JSON_API[JSON API]
        EXCEL_EXPORT[Excel Export API]
        DASHBOARD[Monitoring & Dashboards]
    end
    
    %% Connections
    PDF & EXCEL -- "Upload via API" --> HANDLERS
    WEB & API -- "HTTP Requests" --> ROUTER
    ROUTER --> HANDLERS
    
    HANDLERS -- "Enqueues Jobs" --> ASYNQ_CLIENT
    ASYNQ_CLIENT --> ASYNQ_QUEUE
    
    HANDLERS -- "Calls" --> SERVICES
    SERVICES -- "Uses" --> REPOS
    REPOS -- "Queries" --> POSTGRES
    SERVICES -- "Caches Data" --> REDIS_CACHE
    
    WORKERS -- "Pulls Jobs from" --> ASYNQ_QUEUE
    WORKERS -- "Processes Jobs using" --> SERVICES
    
    POSTGRES --> JSON_API
    POSTGRES --> EXCEL_EXPORT
    ASYNQ_QUEUE --> DASHBOARD

```

### Hexagonal Architecture

The project adheres to the principles of Hexagonal (Ports and Adapters) Architecture. This clean separation ensures the core business logic is independent of external technologies.

```
┌─────────────────────────────────────────────────────┐
│                 Presentation Layer                   │
│      (Primary Adapters: HTTP Handlers, Workers)      │
├─────────────────────────────────────────────────────┤
│                 Application Layer                    │
│          (Ports: Service Interfaces)                 │
│         (Use Cases & Business Services)              │
├─────────────────────────────────────────────────────┤
│                   Domain Layer                       │
│      (Core Entities, Value Objects, Domain Logic)    │
├─────────────────────────────────────────────────────┤
│                Infrastructure Layer                  │
│   (Secondary Adapters: PostgreSQL, Redis, S3, Asynq) │
└─────────────────────────────────────────────────────┘
```

---

## 🛠 Tech Stack

### Core Technologies

| Component | Technology | Version | Justification |
|-----------|------------|---------|---------------|
| **Language** | Go | 1.22+ | Excellent performance, built-in concurrency, and modern features like `slog` and enhanced routing. |
| **Database** | PostgreSQL | 16 | Advanced features like `tsvector`, generated columns, and materialized views are perfect for this use case. |
| **DB Driver** | pgx | v5 | The premier, high-performance PostgreSQL driver for Go. |
| **Cache / Queue**| Redis | 7.2 | Industry standard for high-speed caching and as a reliable message broker for Asynq. |
| **Job Queue** | Asynq | latest | A simple, efficient, and reliable distributed task queue for Go. |
| **Container** | Docker | 24.0+ | Ensures a consistent and reproducible development environment. |

### Key Go Dependencies

| Library | Purpose |
|---|---|
| `github.com/jackc/pgx/v5` | High-performance PostgreSQL driver and toolkit. |
| `github.com/hibiken/asynq` | Distributed task queue for background job processing. |
| `github.com/redis/go-redis/v9` | The primary Redis client for caching. |
| `github.com/Masterminds/squirrel` | Fluent SQL query builder for Go. |
| `github.com/golang-migrate/migrate/v4` | Handles database schema migrations. |
| `github.com/ledongthuc/pdf` | Core library for PDF text extraction in the seeder. |
| `github.com/tealeg/xlsx/v3` | Used for generating and streaming Excel files. |
| `github.com/google/uuid` | UUID generation. |
| `github.com/shopspring/decimal`| Arbitrary-precision fixed-point decimal numbers for financial calculations. |
| `github.com/joho/godotenv` | Manages environment variables from `.env` files for local development. |
| `log/slog` | (Standard Library) Structured, context-aware logging. |

### Development & Operations Tools

| Tool | Purpose |
|------|---------|
| **Docker Compose** | Orchestrates the multi-container local development environment. |
| **golangci-lint** | Fast, configurable Go linter aggregator. |
| **migrate CLI** | Command-line tool for managing database migrations. |
| **AsynqMon** | Web UI for monitoring and managing Asynq tasks and queues. |
| **MinIO** | S3-compatible object storage for local development. |
| **MailHog** | Local SMTP server for testing email notifications. |
| **pgAdmin** | Web UI for managing the PostgreSQL database. |

---

## 📊 Database Schema

The database schema is the foundation of the application, designed for data integrity, performance, and scalability. It is managed via versioned SQL migration files.

*   **Source of Truth**: The `migrations/*.sql` files.
*   **Key Features**:
    *   **Generated Columns**: `total_cost`, `cost_per_item`, and `search_vector` are calculated automatically by the database, ensuring consistency.
    *   **Full-Text Search**: A `tsvector` column is indexed with GIN for fast and advanced searching on item names and descriptions.
    *   **Soft Deletes**: The `deleted_at` timestamp allows for data to be archived without permanent loss.
    *   **Materialized View**: `inventory_excel_export_mat` is used to pre-calculate complex joins and aggregations, making data exports incredibly fast.

```sql
-- Enumerations (from 000001_create_enums.up.sql)
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


-- Main Inventory Table (from 000002_create_inventory.up.sql)
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
    
    -- Storage & Research
    storage_location VARCHAR(100),
    storage_bin VARCHAR(50),
    qr_code VARCHAR(100),
    estimated_value DECIMAL(10, 2),
    market_demand market_demand_level DEFAULT 'medium',
    seasonality_notes TEXT,
    
    -- Status flags & Metadata
    needs_repair BOOLEAN DEFAULT FALSE,
    is_consignment BOOLEAN DEFAULT FALSE,
    is_returned BOOLEAN DEFAULT FALSE,
    search_vector tsvector GENERATED ALWAYS AS (
        setweight(to_tsvector('english', coalesce(item_name, '')), 'A') ||
        setweight(to_tsvector('english', coalesce(description, '')), 'B') ||
        setweight(to_tsvector('english', coalesce(keywords, '')), 'C')
    ) STORED,
    keywords TEXT,
    notes TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP WITH TIME ZONE
);

-- Indexes for inventory table
CREATE INDEX idx_inventory_invoice ON inventory(invoice_id);
CREATE INDEX idx_inventory_category ON inventory(category);
CREATE INDEX idx_inventory_storage ON inventory(storage_location, storage_bin);
CREATE INDEX idx_inventory_search ON inventory USING GIN(search_vector);
CREATE INDEX idx_inventory_not_deleted ON inventory(deleted_at) WHERE deleted_at IS NULL;


-- Platform Listings Table (from 000003_create_platform_listings.up.sql)
CREATE TABLE IF NOT EXISTS platform_listings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    lot_id UUID NOT NULL REFERENCES inventory(lot_id) ON DELETE CASCADE,
    platform platform_type NOT NULL,
    status listing_status DEFAULT 'not_listed',
    list_price DECIMAL(10, 2) CHECK (list_price > 0),
    listing_url TEXT,
    listing_title VARCHAR(255),
    sold_price DECIMAL(10, 2),
    platform_fees DECIMAL(10, 2) DEFAULT 0,
    listed_date TIMESTAMP WITH TIME ZONE,
    sold_date TIMESTAMP WITH TIME ZONE,
    metadata JSONB,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT unique_active_listing UNIQUE(lot_id, platform)
);

-- Indexes for platform_listings table
CREATE INDEX idx_platform_listings_lot ON platform_listings(lot_id);
CREATE INDEX idx_platform_listings_platform_status ON platform_listings(platform, status);


-- Supporting Tables (from 000004_create_supporting_tables.up.sql)
CREATE TABLE IF NOT EXISTS activity_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    lot_id UUID REFERENCES inventory(lot_id) ON DELETE CASCADE,
    action_type VARCHAR(50) NOT NULL,
    old_values JSONB,
    new_values JSONB,
    user_id VARCHAR(100),
    ip_address INET,
    user_agent TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS async_jobs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    job_type VARCHAR(50) NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    payload JSONB,
    result JSONB,
    error TEXT,
    attempts INTEGER DEFAULT 0,
    started_at TIMESTAMP WITH TIME ZONE,
    completed_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);


-- Materialized View for Reporting (from 000005_create_materialized_views.up.sql)
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

    -- Platform-specific columns (flattened using modern FILTER clause)
    BOOL_OR(pl.status = 'active') FILTER (WHERE pl.platform = 'ebay') AS ebay_listed,
    MAX(pl.list_price) FILTER (WHERE pl.platform = 'ebay') AS ebay_price,
    MAX(pl.listing_url) FILTER (WHERE pl.platform = 'ebay') AS ebay_url,
    BOOL_OR(pl.status = 'sold') FILTER (WHERE pl.platform = 'ebay') AS ebay_sold,
    
    BOOL_OR(pl.status = 'active') FILTER (WHERE pl.platform = 'etsy') AS etsy_listed,
    MAX(pl.list_price) FILTER (WHERE pl.platform = 'etsy') AS etsy_price,
    MAX(pl.listing_url) FILTER (WHERE pl.platform = 'etsy') AS etsy_url,
    BOOL_OR(pl.status = 'sold') FILTER (WHERE pl.platform = 'etsy') AS etsy_sold,
    
    -- ... other platforms like facebook, chairish, etc. ...

    -- Calculated fields across all platforms
    MAX(pl.sold_price) AS sale_price,
    MAX(pl.sold_date) AS sale_date,
    SUM(pl.platform_fees) AS total_platform_fees,
    (MAX(pl.sold_price) - i.total_cost - COALESCE(SUM(pl.platform_fees), 0)) AS net_profit,
    CASE 
        WHEN i.total_cost > 0 AND MAX(pl.sold_price) IS NOT NULL THEN 
            ((MAX(pl.sold_price) - i.total_cost - COALESCE(SUM(pl.platform_fees), 0)) / i.total_cost * 100.0)
        ELSE NULL 
    END AS roi_percent,
    CASE 
        WHEN MAX(pl.sold_date) IS NOT NULL THEN 
            EXTRACT(DAY FROM (MAX(pl.sold_date) - i.acquisition_date))
        ELSE NULL 
    END AS days_to_sell,

    i.created_at,
    i.updated_at
FROM inventory i
LEFT JOIN platform_listings pl ON i.lot_id = pl.lot_id
WHERE i.deleted_at IS NULL
GROUP BY i.lot_id;

CREATE UNIQUE INDEX IF NOT EXISTS idx_excel_export_lot ON inventory_excel_export_mat(lot_id);

-- Helper function to refresh the view concurrently
CREATE OR REPLACE FUNCTION refresh_excel_export_mat()
RETURNS void AS $$
BEGIN
    REFRESH MATERIALIZED VIEW CONCURRENTLY inventory_excel_export_mat;
END;
$$ LANGUAGE plpgsql;
```

---

## 📡 API Documentation

The API follows RESTful principles and uses Go 1.22's method-specific routing with path variables.

**Base URL**: `/api/v1`

### Core Endpoints

#### Asynchronous Import

```yaml
POST /import/pdf:
  description: Upload a single PDF invoice to be queued for async processing.
  content-type: multipart/form-data
  body:
    file: binary (PDF file)
    invoice_id: string (required)
    auction_id: integer (optional)
  response: 202 Accepted
    job_id: string
    status: "queued"
    message: string

POST /import/excel:
  description: Upload a single Excel file to be queued for async processing.
  content-type: multipart/form-data
  body:
    file: binary (XLSX file)
  response: 202 Accepted
    job_id: string
    status: "queued"
    message: string

POST /import/batch:
  description: Upload multiple files of the same type for batch processing.
  content-type: multipart/form-data
  body:
    files: array[binary]
    type: string (pdf|excel)
  response: 202 Accepted
    batch_id: string
    job_ids: array[string]

GET /import/status/{job_id}:
  description: Check the status of an asynchronous import job.
  response: 200 OK
    job_id: string
    status: string (pending|processing|completed|failed)
    progress: integer
    result: object
```

#### Inventory Management

```yaml
GET /inventory:
  description: List inventory items with filtering, sorting, and pagination.
  parameters:
    page: integer (default: 1)
    limit: integer (default: 50, max: 100)
    search: string (full-text search on name, description, keywords)
    category: string
    condition: string
    storage_location: string
    invoice_id: string
    needs_repair: boolean
    sort: string (e.g., acquisition_date, value, name)
    order: string (asc|desc)
  response: 200 OK
    items: array
    page: integer
    page_size: integer
    total_count: integer
    total_pages: integer

GET /inventory/{id}:
  description: Retrieve a single inventory item by its Lot ID (UUID).
  response: 200 OK
    (InventoryItem object)

POST /inventory:
  description: Create a new inventory item.
  body: (CreateInventoryRequest object)
  response: 201 Created
    (InventoryItem object)

PUT /inventory/{id}:
  description: Update an existing inventory item.
  body: (UpdateInventoryRequest object)
  response: 200 OK
    (InventoryItem object)

DELETE /inventory/{id}:
  description: Soft delete an inventory item. Use ?permanent=true for a hard delete.
  response: 200 OK
    message: "Inventory item deleted successfully"
```

#### Export & Reports

```yaml
GET /export/excel:
  description: Stream an Excel file of inventory data. Supports filtering.
  parameters: (Similar to GET /inventory)
  response: 200 OK
    content-type: application/vnd.openxmlformats-officedocument.spreadsheetml.sheet

GET /export/json:
  description: Export inventory data as a structured JSON object with metadata.
  parameters: (Similar to GET /inventory)
  response: 200 OK
    inventory: array
    metadata: object

GET /export/pdf:
  description: Generate and stream a PDF report of inventory data.
  response: 200 OK
    content-type: application/pdf
```

#### Dashboard & Health

```yaml
GET /dashboard:
  description: Retrieve aggregated data for the main dashboard.
  response: 200 OK
    (DashboardData object)

GET /dashboard/analytics:
  description: Retrieve time-series analytics data.
  parameters:
    period: string (e.g., 7d, 30d, 90d)
  response: 200 OK
    (AnalyticsData object)

GET /health:
  description: Comprehensive health check of the API and its dependencies (DB, Redis, Asynq).
  response: 200 OK or 503 Service Unavailable

GET /ready:
  description: Kubernetes readiness probe to check if the service can accept traffic.
  response: 200 OK or 503 Service Unavailable
```

---

## 📁 Project Structure

The project follows the standard Go project layout, promoting a clean separation of concerns.

```
resell-be/
├── build_support/
│   └── scripts/
│       ├── migrate.sh          # Migration runner script
│       └── seed.sh             # Battle-tested database seeder script
├── cmd/
│   ├── api/
│   │   └── main.go             # API server entry point
│   ├── seeder/
│   │   └── main.go             # Go-based seeder program
│   └── worker/
│       └── main.go             # Asynq worker entry point
├── internal/
│   ├── adapters/               # Implementations of external interfaces (DB, Redis)
│   │   ├── db/
│   │   ├── redis_adapter/
│   │   └── storage/
│   ├── core/                   # Core business logic, domain models, and service interfaces
│   │   ├── domain/
│   │   ├── ports/
│   │   └── services/
│   ├── handlers/               # HTTP handlers and API-specific logic
│   │   └── middleware/
│   ├── pkg/                    # Shared utility packages (config, logger)
│   │   ├── config/
│   │   └── logger/
│   └── workers/                # Asynq task processor implementations
├── migrations/                 # SQL database migration files
│   ├── 000001_create_enums.up.sql
│   └── ...
├── test/
│   └── helpers/                # Test helpers and utilities
├── .gitignore
├── DESIGN.md                   # This document
├── SETUP.md                    # Detailed setup and testing guide
├── docker-compose.yml          # Docker Compose for local development
└── go.mod                      # Go module definition
```

---

## 🚀 Installation & Setup

A complete local development environment is managed via Docker Compose. See `SETUP.MD` for detailed, step-by-step instructions.

### Quick Start

1.  **Prerequisites**:
    *   Go 1.22+
    *   Docker & Docker Compose
    *   `make` (optional, for convenience)
    *   `git`

2.  **Clone the Repository**:
    ```bash
    git clone https://github.com/ammerola/resell-be.git
    cd resell-be
    ```

3.  **Configure Environment**:
    ```bash
    cp .env.example .env
    # No edits are required to run locally with Docker
    ```

4.  **Start Services**:
    ```bash
    docker-compose up -d --build
    ```
    This will start PostgreSQL, Redis, MinIO, and other development services.

5.  **Run Database Migrations**:
    ```bash
    # Wait a few seconds for the database to initialize...
    sleep 5
    make migrate-up
    ```

6.  **Seed the Database**:
    *   Place your PDF invoices in a directory (e.g., `./data/invoices`).
    *   Place your `auctions_master.xlsx` file in a directory (e.g., `./data/auction_data`).
    *   Run the provided `seed.sh` script, which orchestrates the Go seeder:
    ```bash
    # Update INVOICES_DIR and AUCTIONS_FILE in the script or pass as environment variables
    INVOICES_DIR=./data/invoices AUCTIONS_FILE=./data/auction_data/auctions_master.xlsx ./build_support/scripts/seed.sh
    ```

7.  **Run the Application**:
    ```bash
    go run ./cmd/api/main.go
    ```
    In a separate terminal, run the worker:
    ```bash
    go run ./cmd/worker/main.go
    ```

8.  **Verify**:
    *   API Health: `curl http://localhost:8080/health`
    *   AsynqMon UI: [http://localhost:8081](http://localhost:8081)
    *   pgAdmin: [http://localhost:5050](http://localhost:5050)

---

## 💻 Development

### Running with Hot Reload

For an efficient development workflow, use `air` for live reloading:

```bash
# Install air
go install github.com/air-verse/air@latest

# Run with hot reload
air
```

### Code Quality

-   **Formatting**: Code should be formatted with `gofmt`.
-   **Linting**: Use `golangci-lint` to check for style issues and potential bugs. Run `make lint`.

### Database Migrations

Migrations are managed using `golang-migrate`.

```bash
# Create a new migration
migrate create -ext sql -dir migrations -seq add_new_feature

# Run migrations up
make migrate-up

# Roll back the last migration
make migrate-down
```

---

## 🧪 Testing

The project emphasizes a comprehensive testing strategy, including unit, integration, and end-to-end tests. Please see `SETUP.MD` for a detailed guide on the testing structure and how to write effective tests for this project.

-   **Run all tests**: `go test ./...`
-   **Run tests with coverage**: `go test -cover ./...`

---

## 📦 Deployment

The application is designed for containerized deployment on cloud platforms like AWS ECS, Fargate, or Kubernetes.

### Production Docker Image

A multi-stage `Dockerfile.prod` (located in `build/docker/`) creates a minimal, optimized production image.

### Environment Configuration

In a production environment, all configuration should be managed through environment variables or a secrets management service (e.g., AWS Secrets Manager). The `.env` file should **not** be used in production.

### AWS Deployment Architecture (Example)

```yaml
AWS Infrastructure:
  VPC: With public and private subnets for security.
  Application:
    - ECS Fargate: For running the `api` and `worker` containers without managing servers.
    - Application Load Balancer (ALB): To distribute traffic to the API service.
    - Auto-scaling: To handle variable loads.
  Database:
    - RDS PostgreSQL: A managed, multi-AZ database for high availability.
    - Read Replicas: For offloading read-heavy analytics queries.
  Caching & Queuing:
    - ElastiCache for Redis: A managed Redis service for both caching and the Asynq job queue.
  Storage:
    - S3: For persistent storage of uploaded PDFs, Excel files, and generated reports.
  Security:
    - IAM Roles: For secure, password-less access between services.
    - Secrets Manager: To store database credentials and other secrets.
    - WAF: Web Application Firewall to protect the API from common exploits.
```

---

## 📊 Monitoring

### Health Endpoints

-   `/health`: A comprehensive check of all service dependencies. Returns `200 OK` if all services are healthy and `503 Service Unavailable` if any are degraded.
-   `/ready`: A simple check for readiness probes, ensuring the service is ready to accept traffic.

### Metrics

The application is instrumented to expose Prometheus metrics (when enabled via config). Key metrics to monitor include:
-   **API**: Request latency (p95, p99), request rate, error rate (4xx/5xx).
-   **Asynq**: Queue depth, job processing times, failure/retry rates.
-   **Database**: Connection pool utilization, query latency, CPU/memory usage.
-   **Go Runtime**: Goroutine count, memory allocation, GC pause times.

### Logging

Structured `slog` logs in JSON format are written to `stdout`. In a cloud environment, these logs should be collected by a service like AWS CloudWatch, Datadog, or an ELK stack for aggregation, searching, and alerting.

---

## 🔒 Security

-   **Rate Limiting**: IP-based rate limiting is implemented as middleware to prevent abuse.
-   **CORS**: Configurable Cross-Origin Resource Sharing policy to restrict access to trusted domains.
-   **Secure Headers**: Security-focused HTTP headers (`X-Content-Type-Options`, `X-Frame-Options`, `CSP`, etc.) are applied via middleware.
-   **Input Validation**: All incoming API requests are strictly validated to prevent malformed data from entering the system.
-   **SQL Injection**: The use of `pgx` with parameterized queries prevents SQL injection vulnerabilities.
-   **Secrets Management**: Configuration is loaded from the environment, allowing for secure injection of secrets in production environments.

---

## 🤝 Contributing

Please follow standard Gitflow or feature-branch workflow. All contributions should be submitted via pull requests and must pass all automated checks (linting, testing).

---

## 📝 License

This project is licensed under the MIT License. See the `LICENSE` file for details.

---

## 🚀 Roadmap

### Phase 1: Core Functionality (Complete)
-   ✅ Async PDF and Excel invoice processing.
-   ✅ Core inventory CRUD and listing capabilities.
-   ✅ Foundational analytics and dashboard API.
-   ✅ Robust, containerized development environment.

### Phase 2: Enhanced Analytics & UI (Q1 2026)
-   📈 Develop a web-based front-end dashboard.
-   📊 Implement advanced analytics: demand forecasting, price optimization suggestions.
-   📸 Integrate image hosting (S3) and management for inventory items.

### Phase 3: Automation & Integration (Q2 2026)
-   🔗 Direct API integrations with eBay, Etsy, etc., for automated listing.
-   🤖 Implement a dynamic repricing engine based on market data.
-   🔔 Set up customizable email/SMS notifications for key events (e.g., sales, low stock).

### Phase 4: Scale & Intelligence (Q3 2026)
-   🚀 Refactor for multi-tenant SaaS architecture.
-   🧠 Introduce ML models for more accurate price prediction and categorization.
-   📱 Develop a companion mobile application for on-the-go inventory management.

---

**Built with precision for professional resellers** 🎯