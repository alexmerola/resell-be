// internal/core/services/inventory_service_test.go
package services_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/ammerola/resell-be/internal/core/domain"
	"github.com/ammerola/resell-be/internal/core/ports"
	"github.com/ammerola/resell-be/internal/core/services"
	"github.com/ammerola/resell-be/test/helpers"
	"github.com/ammerola/resell-be/test/mocks"
)

func TestInventoryService_SaveItem(t *testing.T) {
	tests := []struct {
		name          string
		item          *domain.InventoryItem
		setupMocks    func(*mocks.MockInventoryRepository)
		expectedError bool
		errorContains string
	}{
		{
			name: "successful_save_with_valid_item",
			item: helpers.CreateTestInventoryItem(),
			setupMocks: func(m *mocks.MockInventoryRepository) {
				m.EXPECT().
					Save(gomock.Any(), gomock.Any()).
					Return(nil)
			},
			expectedError: false,
		},
		{
			name: "validation_fails_for_missing_invoice_id",
			item: helpers.CreateTestInventoryItem(func(i *domain.InventoryItem) {
				i.InvoiceID = ""
			}),
			setupMocks:    func(m *mocks.MockInventoryRepository) {},
			expectedError: true,
			errorContains: "invoice_id is required",
		},
		{
			name: "validation_fails_for_missing_item_name",
			item: helpers.CreateTestInventoryItem(func(i *domain.InventoryItem) {
				i.ItemName = ""
			}),
			setupMocks:    func(m *mocks.MockInventoryRepository) {},
			expectedError: true,
			errorContains: "item_name is required",
		},
		{
			name: "validation_fails_for_negative_bid_amount",
			item: helpers.CreateTestInventoryItem(func(i *domain.InventoryItem) {
				i.BidAmount = decimal.NewFromFloat(-100.00)
			}),
			setupMocks:    func(m *mocks.MockInventoryRepository) {},
			expectedError: true,
			errorContains: "bid_amount cannot be negative",
		},
		{
			name: "validation_fails_for_zero_quantity",
			item: helpers.CreateTestInventoryItem(func(i *domain.InventoryItem) {
				i.Quantity = 0
			}),
			setupMocks:    func(m *mocks.MockInventoryRepository) {},
			expectedError: true,
			errorContains: "quantity must be positive",
		},
		{
			name: "repository_save_error",
			item: helpers.CreateTestInventoryItem(),
			setupMocks: func(m *mocks.MockInventoryRepository) {
				m.EXPECT().
					Save(gomock.Any(), gomock.Any()).
					Return(errors.New("database connection failed"))
			},
			expectedError: true,
			errorContains: "database connection failed",
		},
		{
			name: "sets_default_category_when_empty",
			item: helpers.CreateTestInventoryItem(func(i *domain.InventoryItem) {
				i.Category = ""
			}),
			setupMocks: func(m *mocks.MockInventoryRepository) {
				m.EXPECT().
					Save(gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, item *domain.InventoryItem) error {
						assert.Equal(t, domain.CategoryOther, item.Category)
						return nil
					})
			},
			expectedError: false,
		},
		{
			name: "calculates_total_cost_correctly",
			item: helpers.CreateTestInventoryItem(func(i *domain.InventoryItem) {
				i.BidAmount = decimal.NewFromFloat(100.00)
				i.BuyersPremium = decimal.NewFromFloat(18.00)
				i.SalesTax = decimal.NewFromFloat(10.18)
				i.ShippingCost = decimal.NewFromFloat(15.00)
			}),
			setupMocks: func(m *mocks.MockInventoryRepository) {
				m.EXPECT().
					Save(gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, item *domain.InventoryItem) error {
						expectedTotal := decimal.NewFromFloat(143.18)
						assert.True(t, item.TotalCost.Equal(expectedTotal),
							"Expected total: %s, Got: %s", expectedTotal, item.TotalCost)
						assert.True(t, item.CostPerItem.Equal(expectedTotal))
						return nil
					})
			},
			expectedError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockRepo := mocks.NewMockInventoryRepository(ctrl)
			mockDB := mocks.NewMockPgxPool(ctrl)
			logger := helpers.TestLogger()

			service := services.NewInventoryService(mockRepo, mockDB, logger)

			// Setup mocks
			tt.setupMocks(mockRepo)

			// Execute
			err := service.SaveItem(context.Background(), tt.item)

			// Assert
			if tt.expectedError {
				require.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				require.NoError(t, err)
				assert.NotEqual(t, uuid.Nil, tt.item.LotID)
			}
		})
	}
}

func TestInventoryService_SaveItems(t *testing.T) {
	tests := []struct {
		name          string
		items         []domain.InventoryItem
		setupMocks    func(*mocks.MockInventoryRepository)
		expectedError bool
		errorContains string
	}{
		{
			name:  "successfully_saves_multiple_items",
			items: helpers.CreateTestInventoryItems(3),
			setupMocks: func(m *mocks.MockInventoryRepository) {
				m.EXPECT().
					SaveBatch(gomock.Any(), gomock.Any()).
					Return(nil)
			},
			expectedError: false,
		},
		{
			name:          "returns_nil_for_empty_items",
			items:         []domain.InventoryItem{},
			setupMocks:    func(m *mocks.MockInventoryRepository) {},
			expectedError: false,
		},
		{
			name: "validation_fails_for_invalid_item",
			items: []domain.InventoryItem{
				*helpers.CreateTestInventoryItem(),
				*helpers.CreateTestInventoryItem(func(i *domain.InventoryItem) {
					i.InvoiceID = ""
				}),
			},
			setupMocks:    func(m *mocks.MockInventoryRepository) {},
			expectedError: true,
			errorContains: "validation failed",
		},
		{
			name:  "repository_batch_save_error",
			items: helpers.CreateTestInventoryItems(2),
			setupMocks: func(m *mocks.MockInventoryRepository) {
				m.EXPECT().
					SaveBatch(gomock.Any(), gomock.Any()).
					Return(errors.New("batch insert failed"))
			},
			expectedError: true,
			errorContains: "batch insert failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockRepo := mocks.NewMockInventoryRepository(ctrl)
			mockDB := mocks.NewMockPgxPool(ctrl)
			logger := helpers.TestLogger()

			service := services.NewInventoryService(mockRepo, mockDB, logger)

			// Setup mocks
			tt.setupMocks(mockRepo)

			// Execute
			err := service.SaveItems(context.Background(), tt.items)

			// Assert
			if tt.expectedError {
				require.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestInventoryService_GetByID(t *testing.T) {
	testItem := helpers.CreateTestInventoryItem()

	tests := []struct {
		name          string
		lotID         uuid.UUID
		setupMocks    func(*mocks.MockInventoryRepository)
		expectedItem  *domain.InventoryItem
		expectedError bool
		errorContains string
	}{
		{
			name:  "successfully_retrieves_item",
			lotID: testItem.LotID,
			setupMocks: func(m *mocks.MockInventoryRepository) {
				m.EXPECT().
					FindByID(gomock.Any(), testItem.LotID).
					Return(testItem, nil)
			},
			expectedItem:  testItem,
			expectedError: false,
		},
		{
			name:  "item_not_found",
			lotID: uuid.New(),
			setupMocks: func(m *mocks.MockInventoryRepository) {
				m.EXPECT().
					FindByID(gomock.Any(), gomock.Any()).
					Return(nil, nil)
			},
			expectedItem:  nil,
			expectedError: true,
			errorContains: "inventory item not found",
		},
		{
			name:  "repository_error",
			lotID: testItem.LotID,
			setupMocks: func(m *mocks.MockInventoryRepository) {
				m.EXPECT().
					FindByID(gomock.Any(), testItem.LotID).
					Return(nil, errors.New("database error"))
			},
			expectedItem:  nil,
			expectedError: true,
			errorContains: "failed to get inventory item",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockRepo := mocks.NewMockInventoryRepository(ctrl)
			mockDB := mocks.NewMockPgxPool(ctrl)
			logger := helpers.TestLogger()

			service := services.NewInventoryService(mockRepo, mockDB, logger)

			// Setup mocks
			tt.setupMocks(mockRepo)

			// Execute
			result, err := service.GetByID(context.Background(), tt.lotID)

			// Assert
			if tt.expectedError {
				require.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)
				assert.Equal(t, tt.expectedItem.LotID, result.LotID)
				assert.Equal(t, tt.expectedItem.ItemName, result.ItemName)
			}
		})
	}
}

func TestInventoryService_UpdateItem(t *testing.T) {
	testItem := helpers.CreateTestInventoryItem()

	tests := []struct {
		name          string
		lotID         uuid.UUID
		item          *domain.InventoryItem
		setupMocks    func(*mocks.MockInventoryRepository)
		expectedError bool
		errorContains string
	}{
		{
			name:  "successfully_updates_item",
			lotID: testItem.LotID,
			item:  testItem,
			setupMocks: func(m *mocks.MockInventoryRepository) {
				m.EXPECT().
					Update(gomock.Any(), gomock.Any()).
					Return(nil)
			},
			expectedError: false,
		},
		{
			name:  "validation_fails_for_invalid_data",
			lotID: testItem.LotID,
			item: helpers.CreateTestInventoryItem(func(i *domain.InventoryItem) {
				i.ItemName = ""
			}),
			setupMocks:    func(m *mocks.MockInventoryRepository) {},
			expectedError: true,
			errorContains: "validation failed",
		},
		{
			name:  "repository_update_error",
			lotID: testItem.LotID,
			item:  testItem,
			setupMocks: func(m *mocks.MockInventoryRepository) {
				m.EXPECT().
					Update(gomock.Any(), gomock.Any()).
					Return(errors.New("update failed"))
			},
			expectedError: true,
			errorContains: "failed to update item",
		},
		{
			name:  "recalculates_total_cost",
			lotID: testItem.LotID,
			item: helpers.CreateTestInventoryItem(func(i *domain.InventoryItem) {
				i.BidAmount = decimal.NewFromFloat(200.00)
				i.Quantity = 2
			}),
			setupMocks: func(m *mocks.MockInventoryRepository) {
				m.EXPECT().
					Update(gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, item *domain.InventoryItem) error {
						// Verify that total cost was recalculated
						assert.True(t, item.TotalCost.GreaterThan(decimal.Zero))
						assert.True(t, item.CostPerItem.GreaterThan(decimal.Zero))
						return nil
					})
			},
			expectedError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockRepo := mocks.NewMockInventoryRepository(ctrl)
			mockDB := mocks.NewMockPgxPool(ctrl)
			logger := helpers.TestLogger()

			service := services.NewInventoryService(mockRepo, mockDB, logger)

			// Setup mocks
			tt.setupMocks(mockRepo)

			// Execute
			err := service.UpdateItem(context.Background(), tt.lotID, tt.item)

			// Assert
			if tt.expectedError {
				require.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.lotID, tt.item.LotID)
			}
		})
	}
}

func TestInventoryService_DeleteItem(t *testing.T) {
	testLotID := uuid.New()

	tests := []struct {
		name          string
		lotID         uuid.UUID
		permanent     bool
		setupMocks    func(*mocks.MockInventoryRepository)
		expectedError bool
		errorContains string
	}{
		{
			name:      "successfully_soft_deletes_item",
			lotID:     testLotID,
			permanent: false,
			setupMocks: func(m *mocks.MockInventoryRepository) {
				m.EXPECT().Exists(gomock.Any(), testLotID).Return(true, nil)
				m.EXPECT().SoftDelete(gomock.Any(), testLotID).Return(nil)
			},
			expectedError: false,
		},
		{
			name:      "successfully_permanently_deletes_item",
			lotID:     testLotID,
			permanent: true,
			setupMocks: func(m *mocks.MockInventoryRepository) {
				m.EXPECT().Exists(gomock.Any(), testLotID).Return(true, nil)
				m.EXPECT().Delete(gomock.Any(), testLotID).Return(nil)
			},
			expectedError: false,
		},
		{
			name:      "item_not_found",
			lotID:     testLotID,
			permanent: false,
			setupMocks: func(m *mocks.MockInventoryRepository) {
				m.EXPECT().Exists(gomock.Any(), testLotID).Return(false, nil)
			},
			expectedError: true,
			errorContains: "inventory item not found",
		},
		{
			name:      "repository_exists_error",
			lotID:     testLotID,
			permanent: false,
			setupMocks: func(m *mocks.MockInventoryRepository) {
				m.EXPECT().Exists(gomock.Any(), testLotID).Return(false, errors.New("database error"))
			},
			expectedError: true,
			errorContains: "failed to check item existence",
		},
		{
			name:      "repository_delete_error",
			lotID:     testLotID,
			permanent: true,
			setupMocks: func(m *mocks.MockInventoryRepository) {
				m.EXPECT().Exists(gomock.Any(), testLotID).Return(true, nil)
				m.EXPECT().Delete(gomock.Any(), testLotID).Return(errors.New("delete failed"))
			},
			expectedError: true,
			errorContains: "failed to delete item",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockRepo := mocks.NewMockInventoryRepository(ctrl)
			mockDB := mocks.NewMockPgxPool(ctrl)
			logger := helpers.TestLogger()

			service := services.NewInventoryService(mockRepo, mockDB, logger)

			// Setup mocks
			tt.setupMocks(mockRepo)

			// Execute
			err := service.DeleteItem(context.Background(), tt.lotID, tt.permanent)

			// Assert
			if tt.expectedError {
				require.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestInventoryService_List validates the refactored List method which delegates querying to the repository.
func TestInventoryService_List(t *testing.T) {
	ctx := context.Background()
	testItems := []*domain.InventoryItem{helpers.CreateTestInventoryItem()}

	tests := []struct {
		name               string
		inputParams        ports.ListParams
		mockRepoResponse   []*domain.InventoryItem
		mockRepoTotal      int64
		mockRepoErr        error
		expectedResult     *ports.ListResult
		expectedError      bool
		expectedErrorMsg   string
		expectedRepoParams ports.ListParams
	}{
		{
			name:             "successfully_lists_items_on_first_page",
			inputParams:      ports.ListParams{Page: 1, PageSize: 10, Category: "antiques"},
			mockRepoResponse: testItems,
			mockRepoTotal:    1,
			mockRepoErr:      nil,
			expectedResult: &ports.ListResult{
				Items:      testItems,
				Page:       1,
				PageSize:   10,
				TotalCount: 1,
				TotalPages: 1,
			},
			expectedError:      false,
			expectedRepoParams: ports.ListParams{Page: 1, PageSize: 10, Category: "antiques"},
		},
		{
			name:             "successfully_lists_items_with_multiple_pages",
			inputParams:      ports.ListParams{Page: 2, PageSize: 50},
			mockRepoResponse: testItems,
			mockRepoTotal:    101, // 3 pages total
			mockRepoErr:      nil,
			expectedResult: &ports.ListResult{
				Items:      testItems,
				Page:       2,
				PageSize:   50,
				TotalCount: 101,
				TotalPages: 3,
			},
			expectedError:      false,
			expectedRepoParams: ports.ListParams{Page: 2, PageSize: 50},
		},
		{
			name:             "normalizes_invalid_page_and_pageSize",
			inputParams:      ports.ListParams{Page: 0, PageSize: 2000}, // Page < 1, PageSize > max
			mockRepoResponse: testItems,
			mockRepoTotal:    1,
			mockRepoErr:      nil,
			expectedResult: &ports.ListResult{
				Items:      testItems,
				Page:       1,
				PageSize:   1000,
				TotalCount: 1,
				TotalPages: 1,
			},
			expectedError:      false,
			expectedRepoParams: ports.ListParams{Page: 1, PageSize: 1000},
		},
		{
			name:               "handles_repository_error",
			inputParams:        ports.ListParams{Page: 1, PageSize: 10},
			mockRepoErr:        errors.New("database connection failed"),
			expectedError:      true,
			expectedErrorMsg:   "failed to list inventory items",
			expectedRepoParams: ports.ListParams{Page: 1, PageSize: 10},
		},
		{
			name:             "handles_zero_results",
			inputParams:      ports.ListParams{Page: 1, PageSize: 10},
			mockRepoResponse: []*domain.InventoryItem{},
			mockRepoTotal:    0,
			mockRepoErr:      nil,
			expectedResult: &ports.ListResult{
				Items:      []*domain.InventoryItem{},
				Page:       1,
				PageSize:   10,
				TotalCount: 0,
				TotalPages: 0,
			},
			expectedError:      false,
			expectedRepoParams: ports.ListParams{Page: 1, PageSize: 10},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockRepo := mocks.NewMockInventoryRepository(ctrl)
			mockDB := mocks.NewMockPgxPool(ctrl)
			logger := helpers.TestLogger()

			service := services.NewInventoryService(mockRepo, mockDB, logger)

			// Setup mock to expect the normalized parameters
			mockRepo.EXPECT().
				FindAll(ctx, tt.expectedRepoParams).
				Return(tt.mockRepoResponse, tt.mockRepoTotal, tt.mockRepoErr)

			// Execute
			result, err := service.List(ctx, tt.inputParams)

			// Assert
			if tt.expectedError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedErrorMsg)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedResult, result)
			}
		})
	}
}

func TestInventoryService_BulkUpsert(t *testing.T) {
	// Create test items
	items := helpers.CreateTestInventoryItems(250) // More than batch size

	t.Run("processes_items_in_batches", func(t *testing.T) {
		// Setup
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockRepo := mocks.NewMockInventoryRepository(ctrl)
		mockDB := mocks.NewMockPgxPool(ctrl)
		logger := helpers.TestLogger()

		service := services.NewInventoryService(mockRepo, mockDB, logger)

		// Expect multiple batch saves (250 items / 100 batch size = 3 batches)
		mockRepo.EXPECT().
			SaveBatch(gomock.Any(), gomock.Any()).
			Times(3).
			Return(nil)

		// Execute
		err := service.BulkUpsert(context.Background(), items)

		// Assert
		require.NoError(t, err)
	})

	t.Run("handles_batch_errors", func(t *testing.T) {
		// Setup
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockRepo := mocks.NewMockInventoryRepository(ctrl)
		mockDB := mocks.NewMockPgxPool(ctrl)
		logger := helpers.TestLogger()

		service := services.NewInventoryService(mockRepo, mockDB, logger)

		// First batch succeeds, second batch fails
		gomock.InOrder(
			mockRepo.EXPECT().
				SaveBatch(gomock.Any(), gomock.Any()).
				Return(nil),
			mockRepo.EXPECT().
				SaveBatch(gomock.Any(), gomock.Any()).
				Return(errors.New("batch 2 failed")),
		)

		// Execute
		err := service.BulkUpsert(context.Background(), items)

		// Assert
		require.Error(t, err)
		assert.Contains(t, err.Error(), "batch 2 failed")
		assert.Contains(t, err.Error(), "100-200") // Batch range
	})
}

// Benchmarks

func BenchmarkInventoryService_SaveItem(b *testing.B) {
	// Setup
	ctrl := gomock.NewController(b)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockInventoryRepository(ctrl)
	mockDB := mocks.NewMockPgxPool(ctrl)
	logger := helpers.TestLogger()

	service := services.NewInventoryService(mockRepo, mockDB, logger)
	item := helpers.CreateTestInventoryItem()

	mockRepo.EXPECT().
		Save(gomock.Any(), gomock.Any()).
		AnyTimes().
		Return(nil)

	ctx := context.Background()

	// Benchmark
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = service.SaveItem(ctx, item)
	}
}

func BenchmarkInventoryService_SaveItems(b *testing.B) {
	// Setup
	ctrl := gomock.NewController(b)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockInventoryRepository(ctrl)
	mockDB := mocks.NewMockPgxPool(ctrl)
	logger := helpers.TestLogger()

	service := services.NewInventoryService(mockRepo, mockDB, logger)
	items := helpers.CreateTestInventoryItems(100)

	mockRepo.EXPECT().
		SaveBatch(gomock.Any(), gomock.Any()).
		AnyTimes().
		Return(nil)

	ctx := context.Background()

	// Benchmark
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = service.SaveItems(ctx, items)
	}
}
