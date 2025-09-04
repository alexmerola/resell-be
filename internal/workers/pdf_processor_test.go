// internal/workers/pdf_processor_test.go
package workers_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/ammerola/resell-be/internal/workers"
	"github.com/ammerola/resell-be/test/helpers"
	"github.com/ammerola/resell-be/test/mocks"
)

func TestPDFProcessor_ProcessPDF(t *testing.T) {
	// Test setup remains the same
	tests := []struct {
		name          string
		payload       workers.PDFJobPayload
		setupMocks    func(*mocks.MockInventoryService, *mocks.MockDatabase)
		setupFile     func() string
		expectedError bool
		errorContains string
	}{
		{
			name: "successfully_processes_valid_pdf",
			payload: workers.PDFJobPayload{
				JobID:     uuid.New().String(),
				FilePath:  "", // Will be set by setupFile
				InvoiceID: "TEST-001",
				AuctionID: 12345,
			},
			setupFile: func() string {
				// A minimal PDF that the parser can read without error
				content := []byte(`%PDF-1.4
1 0 obj<</Type/Catalog/Pages 2 0 R>>endobj 2 0 obj<</Type/Pages/Count 1/Kids[3 0 R]>>endobj 3 0 obj<</Type/Page/Parent 2 0 R/MediaBox[0 0 612 792]>>endobj
xref
0 4
0000000000 65535 f
0000000010 00000 n
0000000059 00000 n
0000000112 00000 n
trailer<</Size 4/Root 1 0 R>>
startxref
178
%%EOF`)
				return helpers.CreateTempFile(t, content, ".pdf")
			},
			setupMocks: func(service *mocks.MockInventoryService, db *mocks.MockDatabase) {
				// Expect job status updates (processing and completed)
				db.EXPECT().
					Exec(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Times(2).
					Return(pgconn.CommandTag{}, nil)

				// Expect the service's SaveItems method to be called once with all extracted items.
				// Since our test PDF is minimal and has no real items, we expect a call with an empty slice.
				service.EXPECT().
					SaveItems(gomock.Any(), gomock.Any()).
					Return(nil)
			},
			expectedError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			// These are now mocks of interfaces
			mockService := mocks.NewMockInventoryService(ctrl)
			mockDB := mocks.NewMockDatabase(ctrl)
			logger := helpers.TestLogger()

			// This now compiles correctly
			processor := workers.NewPDFProcessor(mockService, mockDB, logger)

			// Setup file if needed
			if tt.setupFile != nil {
				tt.payload.FilePath = tt.setupFile()
			}

			// Setup mocks
			tt.setupMocks(mockService, mockDB)

			// Create task
			payloadBytes, err := json.Marshal(tt.payload)
			require.NoError(t, err)

			task := asynq.NewTask(workers.TypePDFProcess, payloadBytes)

			// Process task
			err = processor.ProcessPDF(context.Background(), task)

			// Assertions
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
