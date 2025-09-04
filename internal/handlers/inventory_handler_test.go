// internal/handlers/inventory_handler_test.go
package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/ammerola/resell-be/internal/core/domain"
	"github.com/ammerola/resell-be/internal/core/services"
	"github.com/ammerola/resell-be/internal/handlers"
	"github.com/ammerola/resell-be/test/helpers"
	"github.com/ammerola/resell-be/test/mocks"
)

func TestInventoryHandler_GetInventory(t *testing.T) {
	testItem := helpers.CreateTestInventoryItem()

	tests := []struct {
		name           string
		lotID          string
		setupMocks     func(*mocks.MockInventoryService)
		expectedStatus int
		validateBody   func(*testing.T, []byte)
	}{
		{
			name:  "successfully_retrieves_inventory_item",
			lotID: testItem.LotID.String(),
			setupMocks: func(m *mocks.MockInventoryService) {
				m.EXPECT().
					GetByID(gomock.Any(), testItem.LotID).
					Return(testItem, nil)
			},
			expectedStatus: http.StatusOK,
			validateBody: func(t *testing.T, body []byte) {
				var response domain.InventoryItem
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)
				assert.Equal(t, testItem.LotID, response.LotID)
				assert.Equal(t, testItem.ItemName, response.ItemName)
			},
		},
		{
			name:           "invalid_uuid_format",
			lotID:          "not-a-uuid",
			setupMocks:     func(m *mocks.MockInventoryService) {},
			expectedStatus: http.StatusBadRequest,
			validateBody: func(t *testing.T, body []byte) {
				var response map[string]string
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)
				assert.Equal(t, "Invalid inventory ID format", response["error"])
			},
		},
		{
			name:  "item_not_found",
			lotID: uuid.New().String(),
			setupMocks: func(m *mocks.MockInventoryService) {
				m.EXPECT().
					GetByID(gomock.Any(), gomock.Any()).
					Return(nil, fmt.Errorf("inventory item not found: %s", uuid.New()))
			},
			expectedStatus: http.StatusNotFound,
			validateBody: func(t *testing.T, body []byte) {
				var response map[string]string
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)
				assert.Equal(t, "Inventory item not found", response["error"])
			},
		},
		{
			name:  "service_error",
			lotID: testItem.LotID.String(),
			setupMocks: func(m *mocks.MockInventoryService) {
				m.EXPECT().
					GetByID(gomock.Any(), testItem.LotID).
					Return(nil, errors.New("database connection failed"))
			},
			expectedStatus: http.StatusInternalServerError,
			validateBody: func(t *testing.T, body []byte) {
				var response map[string]string
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)
				assert.Equal(t, "Failed to retrieve inventory item", response["error"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockService := mocks.NewMockInventoryService(ctrl)
			logger := helpers.TestLogger()
			handler := handlers.NewInventoryHandler(mockService, logger)

			// Setup mocks
			tt.setupMocks(mockService)

			// Create request
			req := httptest.NewRequest("GET", "/api/v1/inventory/"+tt.lotID, nil)
			req.SetPathValue("id", tt.lotID)
			w := httptest.NewRecorder()

			// Execute
			handler.GetInventory(w, req)

			// Assert
			resp := w.Result()
			assert.Equal(t, tt.expectedStatus, resp.StatusCode)

			if tt.validateBody != nil {
				tt.validateBody(t, w.Body.Bytes())
			}
		})
	}
}

func TestInventoryHandler_ListInventory(t *testing.T) {
	tests := []struct {
		name           string
		queryParams    map[string]string
		setupMocks     func(*mocks.MockInventoryService)
		expectedStatus int
		validateBody   func(*testing.T, []byte)
	}{
		{
			name: "successfully_lists_inventory_with_pagination",
			queryParams: map[string]string{
				"page":  "1",
				"limit": "10",
			},
			setupMocks: func(m *mocks.MockInventoryService) {
				m.EXPECT().
					List(gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, params services.ListParams) (*services.ListResult, error) {
						assert.Equal(t, 1, params.Page)
						assert.Equal(t, 10, params.PageSize)
						return &services.ListResult{
							Items:      []*domain.InventoryItem{helpers.CreateTestInventoryItem()},
							Page:       1,
							PageSize:   10,
							TotalCount: 1,
							TotalPages: 1,
						}, nil
					})
			},
			expectedStatus: http.StatusOK,
			validateBody: func(t *testing.T, body []byte) {
				var response services.ListResult
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)
				assert.Equal(t, 1, len(response.Items))
				assert.Equal(t, int64(1), response.TotalCount)
			},
		},
		{
			name: "filters_by_category",
			queryParams: map[string]string{
				"category": "antiques",
			},
			setupMocks: func(m *mocks.MockInventoryService) {
				m.EXPECT().
					List(gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, params services.ListParams) (*services.ListResult, error) {
						assert.Equal(t, "antiques", params.Category)
						return &services.ListResult{
							Items:      []*domain.InventoryItem{},
							Page:       1,
							PageSize:   50,
							TotalCount: 0,
							TotalPages: 0,
						}, nil
					})
			},
			expectedStatus: http.StatusOK,
		},
		{
			name: "filters_by_search_term",
			queryParams: map[string]string{
				"search": "victorian",
			},
			setupMocks: func(m *mocks.MockInventoryService) {
				m.EXPECT().
					List(gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, params services.ListParams) (*services.ListResult, error) {
						assert.Equal(t, "victorian", params.Search)
						return &services.ListResult{
							Items:      []*domain.InventoryItem{},
							Page:       1,
							PageSize:   50,
							TotalCount: 0,
							TotalPages: 0,
						}, nil
					})
			},
			expectedStatus: http.StatusOK,
		},
		{
			name: "handles_needs_repair_filter",
			queryParams: map[string]string{
				"needs_repair": "true",
			},
			setupMocks: func(m *mocks.MockInventoryService) {
				m.EXPECT().
					List(gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, params services.ListParams) (*services.ListResult, error) {
						require.NotNil(t, params.NeedsRepair)
						assert.True(t, *params.NeedsRepair)
						return &services.ListResult{
							Items:      []*domain.InventoryItem{},
							Page:       1,
							PageSize:   50,
							TotalCount: 0,
							TotalPages: 0,
						}, nil
					})
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:        "service_error",
			queryParams: map[string]string{},
			setupMocks: func(m *mocks.MockInventoryService) {
				m.EXPECT().
					List(gomock.Any(), gomock.Any()).
					Return(nil, errors.New("database error"))
			},
			expectedStatus: http.StatusInternalServerError,
		},
		{
			name: "validates_page_limit",
			queryParams: map[string]string{
				"page":  "0",
				"limit": "200",
			},
			setupMocks: func(m *mocks.MockInventoryService) {
				m.EXPECT().
					List(gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, params services.ListParams) (*services.ListResult, error) {
						assert.Equal(t, 1, params.Page)      // Defaults to 1
						assert.Equal(t, 50, params.PageSize) // Defaults to 50 (max is 100)
						return &services.ListResult{
							Items:      []*domain.InventoryItem{},
							Page:       1,
							PageSize:   50,
							TotalCount: 0,
							TotalPages: 0,
						}, nil
					})
			},
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockService := mocks.NewMockInventoryService(ctrl)
			logger := helpers.TestLogger()
			handler := handlers.NewInventoryHandler(mockService, logger)

			// Setup mocks
			tt.setupMocks(mockService)

			// Create request with query parameters
			req := httptest.NewRequest("GET", "/api/v1/inventory", nil)
			q := req.URL.Query()
			for k, v := range tt.queryParams {
				q.Add(k, v)
			}
			req.URL.RawQuery = q.Encode()

			w := httptest.NewRecorder()

			// Execute
			handler.ListInventory(w, req)

			// Assert
			resp := w.Result()
			assert.Equal(t, tt.expectedStatus, resp.StatusCode)

			if tt.validateBody != nil {
				tt.validateBody(t, w.Body.Bytes())
			}
		})
	}
}

func TestInventoryHandler_CreateInventory(t *testing.T) {
	tests := []struct {
		name           string
		requestBody    interface{}
		setupMocks     func(*mocks.MockInventoryService)
		expectedStatus int
		validateBody   func(*testing.T, []byte)
	}{
		{
			name: "successfully_creates_inventory_item",
			requestBody: handlers.CreateInventoryRequest{
				InvoiceID:   "INV-001",
				AuctionID:   12345,
				ItemName:    "Victorian Tea Set",
				Description: "Antique tea set",
				Category:    "antiques",
				Condition:   "excellent",
				Quantity:    1,
				BidAmount:   decimal.NewFromFloat(150.00),
			},
			setupMocks: func(m *mocks.MockInventoryService) {
				m.EXPECT().
					SaveItem(gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, item *domain.InventoryItem) error {
						assert.Equal(t, "INV-001", item.InvoiceID)
						assert.Equal(t, "Victorian Tea Set", item.ItemName)
						return nil
					})
			},
			expectedStatus: http.StatusCreated,
			validateBody: func(t *testing.T, body []byte) {
				var response domain.InventoryItem
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)
				assert.Equal(t, "INV-001", response.InvoiceID)
			},
		},
		{
			name:           "invalid_json_body",
			requestBody:    "not json",
			setupMocks:     func(m *mocks.MockInventoryService) {},
			expectedStatus: http.StatusBadRequest,
			validateBody: func(t *testing.T, body []byte) {
				var response map[string]string
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)
				assert.Equal(t, "Invalid request body", response["error"])
			},
		},
		{
			name: "missing_required_fields",
			requestBody: handlers.CreateInventoryRequest{
				ItemName: "Test Item",
				// Missing InvoiceID
			},
			setupMocks:     func(m *mocks.MockInventoryService) {},
			expectedStatus: http.StatusBadRequest,
			validateBody: func(t *testing.T, body []byte) {
				var response map[string]string
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)
				assert.Equal(t, "invoice_id is required", response["error"])
			},
		},
		{
			name: "negative_bid_amount",
			requestBody: handlers.CreateInventoryRequest{
				InvoiceID: "INV-001",
				ItemName:  "Test Item",
				BidAmount: decimal.NewFromFloat(-100.00),
			},
			setupMocks:     func(m *mocks.MockInventoryService) {},
			expectedStatus: http.StatusBadRequest,
			validateBody: func(t *testing.T, body []byte) {
				var response map[string]string
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)
				assert.Equal(t, "bid_amount cannot be negative", response["error"])
			},
		},
		{
			name: "service_error",
			requestBody: handlers.CreateInventoryRequest{
				InvoiceID: "INV-001",
				ItemName:  "Test Item",
				BidAmount: decimal.NewFromFloat(100.00),
			},
			setupMocks: func(m *mocks.MockInventoryService) {
				m.EXPECT().
					SaveItem(gomock.Any(), gomock.Any()).
					Return(errors.New("database error"))
			},
			expectedStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockService := mocks.NewMockInventoryService(ctrl)
			logger := helpers.TestLogger()
			handler := handlers.NewInventoryHandler(mockService, logger)

			// Setup mocks
			tt.setupMocks(mockService)

			// Create request
			body, _ := json.Marshal(tt.requestBody)
			req := httptest.NewRequest("POST", "/api/v1/inventory", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			// Execute
			handler.CreateInventory(w, req)

			// Assert
			resp := w.Result()
			assert.Equal(t, tt.expectedStatus, resp.StatusCode)

			if tt.validateBody != nil {
				tt.validateBody(t, w.Body.Bytes())
			}
		})
	}
}

func TestInventoryHandler_UpdateInventory(t *testing.T) {
	testLotID := uuid.New()

	tests := []struct {
		name           string
		lotID          string
		requestBody    interface{}
		setupMocks     func(*mocks.MockInventoryService)
		expectedStatus int
		validateBody   func(*testing.T, []byte)
	}{
		{
			name:  "successfully_updates_inventory_item",
			lotID: testLotID.String(),
			requestBody: handlers.UpdateInventoryRequest{
				InvoiceID: "INV-002",
				ItemName:  "Updated Tea Set",
				BidAmount: decimal.NewFromFloat(200.00),
				Quantity:  1,
			},
			setupMocks: func(m *mocks.MockInventoryService) {
				m.EXPECT().
					UpdateItem(gomock.Any(), testLotID, gomock.Any()).
					Return(nil)
				m.EXPECT().
					GetByID(gomock.Any(), testLotID).
					Return(helpers.CreateTestInventoryItem(func(i *domain.InventoryItem) {
						i.LotID = testLotID
						i.ItemName = "Updated Tea Set"
					}), nil)
			},
			expectedStatus: http.StatusOK,
			validateBody: func(t *testing.T, body []byte) {
				var response domain.InventoryItem
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)
				assert.Equal(t, "Updated Tea Set", response.ItemName)
			},
		},
		{
			name:           "invalid_uuid",
			lotID:          "not-a-uuid",
			requestBody:    handlers.UpdateInventoryRequest{},
			setupMocks:     func(m *mocks.MockInventoryService) {},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:  "validation_error",
			lotID: testLotID.String(),
			requestBody: handlers.UpdateInventoryRequest{
				InvoiceID: "", // Required field
				ItemName:  "Test",
				BidAmount: decimal.NewFromFloat(100.00),
				Quantity:  1,
			},
			setupMocks:     func(m *mocks.MockInventoryService) {},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:  "item_not_found",
			lotID: testLotID.String(),
			requestBody: handlers.UpdateInventoryRequest{
				InvoiceID: "INV-002",
				ItemName:  "Test",
				BidAmount: decimal.NewFromFloat(100.00),
				Quantity:  1,
			},
			setupMocks: func(m *mocks.MockInventoryService) {
				m.EXPECT().
					UpdateItem(gomock.Any(), testLotID, gomock.Any()).
					Return(fmt.Errorf("inventory item not found: %s", testLotID))
			},
			expectedStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockService := mocks.NewMockInventoryService(ctrl)
			logger := helpers.TestLogger()
			handler := handlers.NewInventoryHandler(mockService, logger)

			// Setup mocks
			tt.setupMocks(mockService)

			// Create request
			body, _ := json.Marshal(tt.requestBody)
			req := httptest.NewRequest("PUT", "/api/v1/inventory/"+tt.lotID, bytes.NewReader(body))
			req.SetPathValue("id", tt.lotID)
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			// Execute
			handler.UpdateInventory(w, req)

			// Assert
			resp := w.Result()
			assert.Equal(t, tt.expectedStatus, resp.StatusCode)

			if tt.validateBody != nil {
				tt.validateBody(t, w.Body.Bytes())
			}
		})
	}
}

func TestInventoryHandler_DeleteInventory(t *testing.T) {
	testLotID := uuid.New()

	tests := []struct {
		name           string
		lotID          string
		permanent      bool
		setupMocks     func(*mocks.MockInventoryService)
		expectedStatus int
		validateBody   func(*testing.T, []byte)
	}{
		{
			name:      "successfully_soft_deletes_item",
			lotID:     testLotID.String(),
			permanent: false,
			setupMocks: func(m *mocks.MockInventoryService) {
				m.EXPECT().
					DeleteItem(gomock.Any(), testLotID, false).
					Return(nil)
			},
			expectedStatus: http.StatusOK,
			validateBody: func(t *testing.T, body []byte) {
				var response map[string]interface{}
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)
				assert.Equal(t, "Inventory item deleted successfully", response["message"])
				assert.False(t, response["permanent"].(bool))
			},
		},
		{
			name:      "successfully_permanently_deletes_item",
			lotID:     testLotID.String(),
			permanent: true,
			setupMocks: func(m *mocks.MockInventoryService) {
				m.EXPECT().
					DeleteItem(gomock.Any(), testLotID, true).
					Return(nil)
			},
			expectedStatus: http.StatusOK,
			validateBody: func(t *testing.T, body []byte) {
				var response map[string]interface{}
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)
				assert.True(t, response["permanent"].(bool))
			},
		},
		{
			name:           "invalid_uuid",
			lotID:          "not-a-uuid",
			permanent:      false,
			setupMocks:     func(m *mocks.MockInventoryService) {},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:      "item_not_found",
			lotID:     testLotID.String(),
			permanent: false,
			setupMocks: func(m *mocks.MockInventoryService) {
				m.EXPECT().
					DeleteItem(gomock.Any(), testLotID, false).
					Return(fmt.Errorf("inventory item not found: %s", testLotID))
			},
			expectedStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockService := mocks.NewMockInventoryService(ctrl)
			logger := helpers.TestLogger()
			handler := handlers.NewInventoryHandler(mockService, logger)

			// Setup mocks
			tt.setupMocks(mockService)

			// Create request
			url := "/api/v1/inventory/" + tt.lotID
			if tt.permanent {
				url += "?permanent=true"
			}
			req := httptest.NewRequest("DELETE", url, nil)
			req.SetPathValue("id", tt.lotID)
			w := httptest.NewRecorder()

			// Execute
			handler.DeleteInventory(w, req)

			// Assert
			resp := w.Result()
			assert.Equal(t, tt.expectedStatus, resp.StatusCode)

			if tt.validateBody != nil {
				tt.validateBody(t, w.Body.Bytes())
			}
		})
	}
}
