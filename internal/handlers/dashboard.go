package handlers

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/ammerola/resell-be/internal/adapters/db"
	redis_a "github.com/ammerola/resell-be/internal/adapters/redis_adapter"
	"github.com/ammerola/resell-be/internal/core/ports"
	"github.com/shopspring/decimal"
)

// DashboardHandler handles dashboard operations
type DashboardHandler struct {
	db     *db.Database
	cache  ports.CacheRepository
	logger *slog.Logger
}

// NewDashboardHandler creates a new dashboard handler
func NewDashboardHandler(db *db.Database, cache ports.CacheRepository, logger *slog.Logger) *DashboardHandler {
	return &DashboardHandler{
		db:     db,
		cache:  cache,
		logger: logger.With(slog.String("handler", "dashboard")),
	}
}

// GetDashboard handles GET /api/v1/dashboard
func (h *DashboardHandler) GetDashboard(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Try cache first
	cacheKey := redis_a.BuildKey(redis_a.PrefixDashboard, "main")
	var dashboard DashboardData

	err := h.cache.GetOrSet(ctx, cacheKey, &dashboard, func() (interface{}, error) {
		return h.loadDashboardData(ctx)
	}, 5*time.Minute)

	if err != nil {
		h.logger.ErrorContext(ctx, "failed to load dashboard", err)
		h.respondError(w, http.StatusInternalServerError, "Failed to load dashboard")
		return
	}

	h.respondJSON(w, http.StatusOK, dashboard)
}

// GetAnalytics handles GET /api/v1/dashboard/analytics
func (h *DashboardHandler) GetAnalytics(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Parse time range
	period := r.URL.Query().Get("period")
	if period == "" {
		period = "30d"
	}

	cacheKey := redis_a.BuildKey(redis_a.PrefixAnalytics, period)
	var analytics AnalyticsData

	err := h.cache.GetOrSet(ctx, cacheKey, &analytics, func() (interface{}, error) {
		return h.loadAnalyticsData(ctx, period)
	}, 15*time.Minute)

	if err != nil {
		h.logger.ErrorContext(ctx, "failed to load analytics", err)
		h.respondError(w, http.StatusInternalServerError, "Failed to load analytics")
		return
	}

	h.respondJSON(w, http.StatusOK, analytics)
}

func (h *DashboardHandler) loadDashboardData(ctx context.Context) (*DashboardData, error) {
	dashboard := &DashboardData{
		Timestamp: time.Now(),
	}

	// Load summary statistics
	summaryQuery := `
		SELECT
			COUNT(*) as total_items,
			SUM(total_cost) as total_invested,
			COUNT(CASE WHEN EXISTS (
				SELECT 1 FROM platform_listings pl 
				WHERE pl.lot_id = i.lot_id AND pl.status = 'active'
			) THEN 1 END) as total_listed,
			COUNT(CASE WHEN EXISTS (
				SELECT 1 FROM platform_listings pl 
				WHERE pl.lot_id = i.lot_id AND pl.status = 'sold'
			) THEN 1 END) as total_sold,
			SUM(CASE WHEN EXISTS (
				SELECT 1 FROM platform_listings pl 
				WHERE pl.lot_id = i.lot_id AND pl.sold_price IS NOT NULL
			) THEN pl.sold_price ELSE 0 END) as total_revenue
		FROM inventory i
		WHERE i.deleted_at IS NULL
	`

	err := h.db.QueryRow(ctx, summaryQuery).Scan(
		&dashboard.Summary.TotalItems,
		&dashboard.Summary.TotalInvested,
		&dashboard.Summary.TotalListed,
		&dashboard.Summary.TotalSold,
		&dashboard.Summary.TotalRevenue,
	)
	if err != nil {
		return nil, err
	}

	// Calculate derived metrics
	if dashboard.Summary.TotalInvested.GreaterThan(decimal.Zero) {
		dashboard.Summary.TotalProfit = dashboard.Summary.TotalRevenue.Sub(dashboard.Summary.TotalInvested)
		dashboard.Summary.AverageROI = dashboard.Summary.TotalProfit.Div(dashboard.Summary.TotalInvested).Mul(decimal.NewFromInt(100))
	}

	// Load category breakdown
	categoryQuery := `
		SELECT 
			category,
			COUNT(*) as count,
			SUM(total_cost) as value,
			COUNT(CASE WHEN EXISTS (
				SELECT 1 FROM platform_listings pl 
				WHERE pl.lot_id = i.lot_id AND pl.status = 'sold'
			) THEN 1 END) as sold_count
		FROM inventory i
		WHERE deleted_at IS NULL
		GROUP BY category
		ORDER BY count DESC
		LIMIT 10
	`

	rows, err := h.db.Query(ctx, categoryQuery)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var cat CategoryBreakdown
		if err := rows.Scan(&cat.Category, &cat.Count, &cat.Value, &cat.SoldCount); err != nil {
			continue
		}
		dashboard.CategoryBreakdown = append(dashboard.CategoryBreakdown, cat)
	}

	// Load recent activity
	activityQuery := `
		SELECT 
			action_type as action,
			lot_id,
			created_at as timestamp,
			new_values as details
		FROM activity_logs
		ORDER BY created_at DESC
		LIMIT 20
	`

	actRows, err := h.db.Query(ctx, activityQuery)
	if err == nil {
		defer actRows.Close()
		for actRows.Next() {
			var activity RecentActivity
			if err := actRows.Scan(&activity.Action, &activity.LotID, &activity.Timestamp, &activity.Details); err == nil {
				dashboard.RecentActivity = append(dashboard.RecentActivity, activity)
			}
		}
	}

	return dashboard, nil
}

func (h *DashboardHandler) loadAnalyticsData(ctx context.Context, period string) (*AnalyticsData, error) {
	// Load analytics based on period
	// This is a simplified version
	return &AnalyticsData{
		Period: period,
		// ... load actual analytics
	}, nil
}

// Type definitions

type DashboardData struct {
	Summary           DashboardSummary    `json:"summary"`
	CategoryBreakdown []CategoryBreakdown `json:"category_breakdown"`
	PlatformMetrics   []PlatformMetric    `json:"platform_metrics"`
	AgingInventory    []AgingInventory    `json:"aging_inventory"`
	RecentActivity    []RecentActivity    `json:"recent_activity"`
	Timestamp         time.Time           `json:"timestamp"`
}

type DashboardSummary struct {
	TotalItems        int64           `json:"total_items"`
	TotalInvested     decimal.Decimal `json:"total_invested"`
	TotalListed       int64           `json:"total_listed"`
	TotalSold         int64           `json:"total_sold"`
	TotalRevenue      decimal.Decimal `json:"total_revenue"`
	TotalProfit       decimal.Decimal `json:"total_profit"`
	AverageROI        decimal.Decimal `json:"average_roi"`
	AverageDaysToSell float64         `json:"average_days_to_sell"`
}

type CategoryBreakdown struct {
	Category  string          `json:"category"`
	Count     int             `json:"count"`
	Value     decimal.Decimal `json:"value"`
	SoldCount int             `json:"sold_count"`
	AvgROI    decimal.Decimal `json:"avg_roi,omitempty"`
}

type PlatformMetric struct {
	Platform       string          `json:"platform"`
	ListedCount    int             `json:"listed_count"`
	SoldCount      int             `json:"sold_count"`
	Revenue        decimal.Decimal `json:"revenue"`
	AvgSalePrice   decimal.Decimal `json:"avg_sale_price"`
	ConversionRate float64         `json:"conversion_rate"`
}

type AgingInventory struct {
	Range      string                   `json:"range"`
	Count      int                      `json:"count"`
	TotalValue decimal.Decimal          `json:"total_value"`
	Items      []map[string]interface{} `json:"items,omitempty"`
}

type RecentActivity struct {
	Action    string                 `json:"action"`
	LotID     string                 `json:"lot_id"`
	Timestamp time.Time              `json:"timestamp"`
	Details   map[string]interface{} `json:"details,omitempty"`
}

type AnalyticsData struct {
	Period string `json:"period"`
	// ... analytics fields
}

// Helper methods

func (h *DashboardHandler) respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func (h *DashboardHandler) respondError(w http.ResponseWriter, status int, message string) {
	h.respondJSON(w, status, map[string]string{"error": message})
}
