#!/bin/bash

# Resell Inventory Management System - Database Seeder
# Preserves the battle-tested PDF extraction logic from Python
# while integrating with the Go/PostgreSQL architecture

set -euo pipefail

# Color output for better UX
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
INVOICES_DIR="${INVOICES_DIR:-${PROJECT_ROOT}/../../data/invoices}"
AUCTIONS_FILE="${AUCTIONS_FILE:-${PROJECT_ROOT}/../../data/auction_data/auctions_master.xlsx}"
ENV_FILE="${ENV_FILE:-${PROJECT_ROOT}/../.env.example}"
STATE_FILE="${STATE_FILE:-${PROJECT_ROOT}/.seed_state.json}"
LOG_FILE="${LOG_FILE:-${PROJECT_ROOT}/seed.log}"

# Load environment variables
if [ -f "$ENV_FILE" ]; then
    set -a
    source "$ENV_FILE"
    set +a
else
    echo -e "${RED}Error: .env file not found at $ENV_FILE${NC}"
    echo "Please create it from .env.example"
    exit 1
fi

# Functions
log() {
    echo -e "${GREEN}[$(date +'%Y-%m-%d %H:%M:%S')]${NC} $1" | tee -a "$LOG_FILE"
}

error() {
    echo -e "${RED}[ERROR]${NC} $1" | tee -a "$LOG_FILE"
}

warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

check_prerequisites() {
    log "Checking prerequisites..."
    
    # Check for required directories
    if [ ! -d "$INVOICES_DIR" ]; then
        error "Invoice directory not found: $INVOICES_DIR"
        error "Please create it and add PDF invoices"
        exit 1
    fi
    
    # Count PDF files
    PDF_COUNT=$(find "$INVOICES_DIR" -type f -name "*.pdf" 2>/dev/null | wc -l)
    if [ "$PDF_COUNT" -eq 0 ]; then
        error "No PDF files found in $INVOICES_DIR"
        exit 1
    fi
    info "Found $PDF_COUNT PDF invoice(s) to process"
    
    # Check for auctions file (optional but recommended)
    if [ ! -f "$AUCTIONS_FILE" ]; then
        warning "Auctions file not found at $AUCTIONS_FILE"
        warning "Will use default buyer's premium (20%) and tax rates (8%)"
        read -p "Continue without auction metadata? (y/N): " -n 1 -r
        echo
        if [[ ! $REPLY =~ ^[Yy]$ ]]; then
            exit 1
        fi
    else
        info "Found auction metadata file"
    fi
    
    # Check database connection
    log "Testing database connection..."
    PGPASSWORD="${DB_PASSWORD}" psql -h "${DB_HOST}" -p "${DB_PORT}" -U "${DB_USER}" -d "${DB_NAME}" -c "SELECT 1" > /dev/null 2>&1 || {
        error "Failed to connect to database"
        error "Please ensure PostgreSQL is running and credentials are correct"
        exit 1
    }
    
    # Check if Go seeder binary exists, build if not
    SEEDER_BIN="${PROJECT_ROOT}/bin/seeder"
    if [ ! -f "$SEEDER_BIN" ]; then
        log "Building seeder binary..."
        (cd "$PROJECT_ROOT" && go build -o bin/seeder cmd/seeder/main.go) || {
            error "Failed to build seeder"
            error "Please ensure Go 1.22+ is installed and dependencies are available"
            exit 1
        }
    fi
    
    log "Prerequisites check completed successfully"
}

run_migrations() {
    log "Checking database migrations..."
    
    # Check if migrations have been run
    TABLES_EXIST=$(PGPASSWORD="${DB_PASSWORD}" psql -h "${DB_HOST}" -p "${DB_PORT}" -U "${DB_USER}" -d "${DB_NAME}" -t -c "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = 'public' AND table_name IN ('inventory', 'platform_listings', 'activity_logs')" 2>/dev/null | tr -d ' ')
    
    if [ "$TABLES_EXIST" -lt 3 ]; then
        log "Running database migrations..."
        make migrate-up || {
            error "Failed to run migrations"
            exit 1
        }
    else
        info "Database schema is up to date"
    fi
}

backup_existing_data() {
    log "Creating backup of existing data..."
    
    BACKUP_DIR="${PROJECT_ROOT}/backups"
    mkdir -p "$BACKUP_DIR"
    
    TIMESTAMP=$(date +%Y%m%d_%H%M%S)
    BACKUP_FILE="${BACKUP_DIR}/backup_${TIMESTAMP}.sql"
    
    PGPASSWORD="${DB_PASSWORD}" pg_dump -h "${DB_HOST}" -p "${DB_PORT}" -U "${DB_USER}" -d "${DB_NAME}" \
        --data-only \
        --table=inventory \
        --table=platform_listings \
        --table=activity_logs \
        > "$BACKUP_FILE" 2>/dev/null || {
        warning "No existing data to backup or backup failed"
        return
    }
    
    if [ -s "$BACKUP_FILE" ]; then
        info "Backup created: $BACKUP_FILE"
    else
        rm -f "$BACKUP_FILE"
    fi
}

load_state() {
    if [ -f "$STATE_FILE" ]; then
        info "Found previous seed state"
        PROCESSED_COUNT=$(jq -r '.processed_count // 0' "$STATE_FILE" 2>/dev/null || echo "0")
        info "Previously processed: $PROCESSED_COUNT invoices"
    else
        echo '{"processed_invoices": [], "processed_count": 0}' > "$STATE_FILE"
    fi
}

save_state() {
    local invoice_id=$1
    local status=$2
    
    # Update state file
    jq --arg id "$invoice_id" --arg status "$status" \
        '.processed_invoices += [{"invoice_id": $id, "status": $status, "timestamp": now | strftime("%Y-%m-%dT%H:%M:%SZ")}] | 
         .processed_count = (.processed_invoices | length) |
         .last_update = (now | strftime("%Y-%m-%dT%H:%M:%SZ"))' \
        "$STATE_FILE" > "${STATE_FILE}.tmp" && mv "${STATE_FILE}.tmp" "$STATE_FILE"
}

run_seeder() {
    log "Starting seed process..."
    
    # Prepare arguments for Go seeder
    SEEDER_ARGS=(
        --invoices="$INVOICES_DIR"
        --state="$STATE_FILE"
        --log-level="${LOG_LEVEL:-info}"
    )
    
    if [ -f "$AUCTIONS_FILE" ]; then
        SEEDER_ARGS+=(--auctions="$AUCTIONS_FILE")
    fi
    
    # Add flags based on user input
    if [ "${FORCE_RESEED:-false}" == "true" ]; then
        SEEDER_ARGS+=(--force)
        warning "Force mode enabled - will reprocess all invoices"
    fi
    
    if [ "${DRY_RUN:-false}" == "true" ]; then
        SEEDER_ARGS+=(--dry-run)
        info "Dry run mode - no changes will be made"
    fi
    
    # Run the Go seeder
    log "Executing seeder with arguments: ${SEEDER_ARGS[*]}"
    
    "${PROJECT_ROOT}/bin/seeder" "${SEEDER_ARGS[@]}" | while IFS= read -r line; do
        # Parse seeder output for progress
        if [[ "$line" == *"PROGRESS:"* ]]; then
            echo -ne "\r${BLUE}[PROGRESS]${NC} ${line#*PROGRESS: }"
        elif [[ "$line" == *"SUCCESS:"* ]]; then
            echo -e "\n${GREEN}✓${NC} ${line#*SUCCESS: }"
            # Extract invoice ID and update state
            if [[ "$line" =~ invoice_id:([^ ]+) ]]; then
                save_state "${BASH_REMATCH[1]}" "success"
            fi
        elif [[ "$line" == *"ERROR:"* ]]; then
            echo -e "\n${RED}✗${NC} ${line#*ERROR: }"
            # Extract invoice ID and update state
            if [[ "$line" =~ invoice_id:([^ ]+) ]]; then
                save_state "${BASH_REMATCH[1]}" "error"
            fi
        else
            echo "$line"
        fi
    done
    
    SEEDER_EXIT_CODE=${PIPESTATUS[0]}
    
    if [ $SEEDER_EXIT_CODE -ne 0 ]; then
        error "Seeder failed with exit code $SEEDER_EXIT_CODE"
        exit $SEEDER_EXIT_CODE
    fi
}

generate_summary() {
    log "Generating summary report..."
    
    # Query database for statistics
    STATS=$(PGPASSWORD="${DB_PASSWORD}" psql -h "${DB_HOST}" -p "${DB_PORT}" -U "${DB_USER}" -d "${DB_NAME}" -t -A -F'|' <<EOF
SELECT 
    COUNT(DISTINCT invoice_id) as invoice_count,
    COUNT(*) as total_items,
    COALESCE(SUM(total_cost), 0)::numeric(10,2) as total_invested,
    COUNT(DISTINCT category) as category_count,
    COALESCE(AVG(total_cost), 0)::numeric(10,2) as avg_item_cost
FROM inventory
WHERE deleted_at IS NULL;
EOF
)
    
    IFS='|' read -r INVOICE_COUNT TOTAL_ITEMS TOTAL_INVESTED CATEGORY_COUNT AVG_COST <<< "$STATS"
    
    # Display summary
    echo
    echo -e "${GREEN}═══════════════════════════════════════════════════════${NC}"
    echo -e "${GREEN}         📊 SEED OPERATION COMPLETED SUCCESSFULLY        ${NC}"
    echo -e "${GREEN}═══════════════════════════════════════════════════════${NC}"
    echo
    echo -e "${BLUE}📋 Database Statistics:${NC}"
    echo -e "  • Invoices Processed: ${YELLOW}$INVOICE_COUNT${NC}"
    echo -e "  • Total Items:        ${YELLOW}$TOTAL_ITEMS${NC}"
    echo -e "  • Total Investment:   ${YELLOW}\$$(printf "%'.2f" $TOTAL_INVESTED)${NC}"
    echo -e "  • Categories Used:    ${YELLOW}$CATEGORY_COUNT${NC}"
    echo -e "  • Avg Item Cost:      ${YELLOW}\$$(printf "%'.2f" $AVG_COST)${NC}"
    echo
    
    # Category breakdown
    echo -e "${BLUE}📦 Top Categories:${NC}"
    PGPASSWORD="${DB_PASSWORD}" psql -h "${DB_HOST}" -p "${DB_PORT}" -U "${DB_USER}" -d "${DB_NAME}" -t -A -F'|' <<EOF | head -5 | while IFS='|' read -r category count value; do
SELECT 
    category::text,
    COUNT(*)::text,
    COALESCE(SUM(total_cost), 0)::numeric(10,2)::text
FROM inventory
WHERE deleted_at IS NULL
GROUP BY category
ORDER BY COUNT(*) DESC;
EOF
        printf "  • %-15s %5s items  \$%'.2f\n" "$category" "$count" "$value"
    done
    
    echo
    echo -e "${GREEN}═══════════════════════════════════════════════════════${NC}"
    echo
    
    # Provide next steps
    echo -e "${BLUE}🚀 Next Steps:${NC}"
    echo "  1. Start the API server:     ${YELLOW}make run${NC}"
    echo "  2. View the dashboard:       ${YELLOW}http://localhost:8080${NC}"
    echo "  3. Import new invoices:      ${YELLOW}Place PDFs in $INVOICES_DIR and run: make seed-db${NC}"
    echo "  4. Export to Excel:          ${YELLOW}curl http://localhost:8080/api/v1/export/excel > inventory.xlsx${NC}"
    echo
    
    log "Seed operation completed successfully!"
}

cleanup() {
    info "Cleaning up temporary files..."
    rm -f "${STATE_FILE}.tmp" 2>/dev/null || true
}

# Trap for cleanup on exit
trap cleanup EXIT

# Main execution
main() {
    echo
    echo -e "${BLUE}╔══════════════════════════════════════════════════════╗${NC}"
    echo -e "${BLUE}║     Resell Inventory Management System Seeder       ║${NC}"
    echo -e "${BLUE}╚══════════════════════════════════════════════════════╝${NC}"
    echo
    
    # Parse arguments
    while [[ $# -gt 0 ]]; do
        case $1 in
            --force)
                export FORCE_RESEED=true
                shift
                ;;
            --dry-run)
                export DRY_RUN=true
                shift
                ;;
            --invoices)
                INVOICES_DIR="$2"
                shift 2
                ;;
            --help)
                echo "Usage: $0 [OPTIONS]"
                echo
                echo "Options:"
                echo "  --force       Reprocess all invoices (ignore state)"
                echo "  --dry-run     Preview changes without modifying database"
                echo "  --invoices    Path to invoices directory (default: ./invoices)"
                echo "  --help        Show this help message"
                echo
                exit 0
                ;;
            *)
                error "Unknown option: $1"
                echo "Use --help for usage information"
                exit 1
                ;;
        esac
    done
    
    # Execute seed process
    check_prerequisites
    run_migrations
    backup_existing_data
    load_state
    run_seeder
    generate_summary
}

# Run main function
main "$@"