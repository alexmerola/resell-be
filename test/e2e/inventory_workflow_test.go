//go:build e2e
// +build e2e

package e2e_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/ammerola/resell-be/test/helpers"
)

type InventoryE2ESuite struct {
	suite.Suite
	server    *httptest.Server
	client    *http.Client
	baseURL   string
	testDB    *helpers.TestDB
	testRedis *helpers.TestRedis
}

func (s *InventoryE2ESuite) SetupSuite() {
	// Setup test database
	s.testDB = helpers.SetupTestDB(s.T())

	// Setup test Redis
	s.testRedis = helpers.SetupTestRedis(s.T())

	// Start test server
	s.server = s.startTestServer()
	s.client = &http.Client{Timeout: 10 * time.Second}
	s.baseURL = s.server.URL + "/api/v1"
}

func (s *InventoryE2ESuite) TearDownSuite() {
	s.server.Close()
}

func (s *InventoryE2ESuite) TestCompleteInventoryWorkflow() {
	// 1. Create an inventory item
	createReq := map[string]interface{}{
		"invoice_id":  "E2E-001",
		"item_name":   "E2E Test Item",
		"description": "Item created in E2E test",
		"category":    "antiques",
		"condition":   "excellent",
		"quantity":    1,
		"bid_amount":  150.00,
	}

	resp := s.makeRequest("POST", "/inventory", createReq)
	s.Equal(http.StatusCreated, resp.StatusCode)

	var createdItem map[string]interface{}
	s.decodeResponse(resp, &createdItem)

	lotID := createdItem["lot_id"].(string)
	s.NotEmpty(lotID)

	// 2. Retrieve the created item
	resp = s.makeRequest("GET", fmt.Sprintf("/inventory/%s", lotID), nil)
	s.Equal(http.StatusOK, resp.StatusCode)

	var retrievedItem map[string]interface{}
	s.decodeResponse(resp, &retrievedItem)
	s.Equal("E2E Test Item", retrievedItem["item_name"])

	// 3. Update the item
	updateReq := map[string]interface{}{
		"invoice_id": "E2E-001",
		"item_name":  "Updated E2E Item",
		"bid_amount": 200.00,
		"quantity":   2,
	}

	resp = s.makeRequest("PUT", fmt.Sprintf("/inventory/%s", lotID), updateReq)
	s.Equal(http.StatusOK, resp.StatusCode)

	// 4. List items with filtering
	resp = s.makeRequest("GET", "/inventory?category=antiques&limit=10", nil)
	s.Equal(http.StatusOK, resp.StatusCode)

	var listResponse map[string]interface{}
	s.decodeResponse(resp, &listResponse)
	items := listResponse["items"].([]interface{})
	s.GreaterOrEqual(len(items), 1)

	// 5. Export to Excel
	resp = s.makeRequest("GET", "/export/excel?category=antiques", nil)
	s.Equal(http.StatusOK, resp.StatusCode)
	s.Equal("application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
		resp.Header.Get("Content-Type"))

	// 6. Get dashboard data
	resp = s.makeRequest("GET", "/dashboard", nil)
	s.Equal(http.StatusOK, resp.StatusCode)

	var dashboard map[string]interface{}
	s.decodeResponse(resp, &dashboard)
	s.Contains(dashboard, "summary")
	s.Contains(dashboard, "category_breakdown")

	// 7. Delete the item
	resp = s.makeRequest("DELETE", fmt.Sprintf("/inventory/%s", lotID), nil)
	s.Equal(http.StatusOK, resp.StatusCode)

	// 8. Verify item is soft deleted
	resp = s.makeRequest("GET", fmt.Sprintf("/inventory/%s", lotID), nil)
	s.Equal(http.StatusNotFound, resp.StatusCode)
}

func (s *InventoryE2ESuite) TestPDFImportWorkflow() {
	// Create a test PDF file
	pdfContent := s.createTestPDF()

	// Upload PDF for processing
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	part, err := writer.CreateFormFile("file", "test-invoice.pdf")
	s.NoError(err)

	_, err = io.Copy(part, bytes.NewReader(pdfContent))
	s.NoError(err)

	writer.WriteField("invoice_id", "PDF-E2E-001")
	writer.WriteField("auction_id", "99999")
	writer.Close()

	req, err := http.NewRequest("POST", s.baseURL+"/import/pdf", body)
	s.NoError(err)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := s.client.Do(req)
	s.NoError(err)
	defer resp.Body.Close()

	s.Equal(http.StatusAccepted, resp.StatusCode)

	var importResponse map[string]interface{}
	s.decodeResponse(resp, &importResponse)
	jobID := importResponse["job_id"].(string)
	s.NotEmpty(jobID)

	// Check job status (would need to wait for processing in real scenario)
	resp = s.makeRequest("GET", fmt.Sprintf("/import/status/%s", jobID), nil)
	s.Equal(http.StatusOK, resp.StatusCode)

	var statusResponse map[string]interface{}
	s.decodeResponse(resp, &statusResponse)
	s.Contains(statusResponse, "status")
}

func (s *InventoryE2ESuite) TestSearchFunctionality() {
	// Create items with different attributes for searching
	testItems := []map[string]interface{}{
		{
			"invoice_id":  "SEARCH-001",
			"item_name":   "Victorian Silver Teapot",
			"description": "Antique sterling silver teapot from 1890",
			"category":    "silver",
			"bid_amount":  500.00,
		},
		{
			"invoice_id":  "SEARCH-002",
			"item_name":   "Modern Glass Sculpture",
			"description": "Contemporary art glass piece",
			"category":    "glass",
			"bid_amount":  300.00,
		},
		{
			"invoice_id":  "SEARCH-003",
			"item_name":   "Vintage Silver Ring",
			"description": "Art deco silver ring with gemstones",
			"category":    "jewelry",
			"bid_amount":  150.00,
		},
	}

	// Create all test items
	for _, item := range testItems {
		resp := s.makeRequest("POST", "/inventory", item)
		s.Equal(http.StatusCreated, resp.StatusCode)
	}

	// Search for "silver"
	resp := s.makeRequest("GET", "/inventory?search=silver", nil)
	s.Equal(http.StatusOK, resp.StatusCode)

	var searchResults map[string]interface{}
	s.decodeResponse(resp, &searchResults)
	items := searchResults["items"].([]interface{})
	s.Equal(2, len(items)) // Should find teapot and ring

	// Search for "glass"
	resp = s.makeRequest("GET", "/inventory?search=glass", nil)
	s.Equal(http.StatusOK, resp.StatusCode)

	s.decodeResponse(resp, &searchResults)
	items = searchResults["items"].([]interface{})
	s.Equal(1, len(items))
}

func (s *InventoryE2ESuite) TestConcurrentRequests() {
	// Test that the API handles concurrent requests properly
	done := make(chan bool, 10)

	for i := 0; i < 10; i++ {
		go func(idx int) {
			defer func() { done <- true }()

			item := map[string]interface{}{
				"invoice_id": fmt.Sprintf("CONCURRENT-%03d", idx),
				"item_name":  fmt.Sprintf("Concurrent Item %d", idx),
				"bid_amount": float64(100 + idx*10),
			}

			resp := s.makeRequest("POST", "/inventory", item)
			s.Equal(http.StatusCreated, resp.StatusCode)
		}(i)
	}

	// Wait for all requests to complete
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify all items were created
	resp := s.makeRequest("GET", "/inventory?invoice_id=CONCURRENT", nil)
	s.Equal(http.StatusOK, resp.StatusCode)

	var listResponse map[string]interface{}
	s.decodeResponse(resp, &listResponse)
	s.Equal(int64(10), listResponse["total_count"])
}

func (s *InventoryE2ESuite) TestHealthCheck() {
	resp := s.makeRequest("GET", "/health", nil)
	s.Equal(http.StatusOK, resp.StatusCode)

	var health map[string]interface{}
	s.decodeResponse(resp, &health)
	s.Equal("healthy", health["status"])
	s.Contains(health, "services")

	services := health["services"].(map[string]interface{})
	s.Contains(services, "database")
	s.Contains(services, "redis")
}

// Helper methods

func (s *InventoryE2ESuite) startTestServer() *httptest.Server {
	// Initialize your application with test dependencies
	// This would use your actual server setup with test database/redis

	// For now, create a simple test server
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Your routing logic here
		// This should use your actual router setup
	})

	return httptest.NewServer(handler)
}

func (s *InventoryE2ESuite) makeRequest(method, path string, body interface{}) *http.Response {
	var reqBody io.Reader
	if body != nil {
		jsonBody, err := json.Marshal(body)
		s.NoError(err)
		reqBody = bytes.NewReader(jsonBody)
	}

	req, err := http.NewRequest(method, s.baseURL+path, reqBody)
	s.NoError(err)

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := s.client.Do(req)
	s.NoError(err)

	return resp
}

func (s *InventoryE2ESuite) decodeResponse(resp *http.Response, v interface{}) {
	defer resp.Body.Close()
	err := json.NewDecoder(resp.Body).Decode(v)
	s.NoError(err)
}

func (s *InventoryE2ESuite) createTestPDF() []byte {
	// Create a minimal valid PDF for testing
	return []byte(`%PDF-1.4
1 0 obj

/Type /Catalog
/Pages 2 0 R
>>
endobj

2 0 obj

/Type /Pages
/Kids [3 0 R]
/Count 1
>>
endobj

3 0 obj

/Type /Page
/Parent 2 0 R
/Resources 
/Font 
/F1 
/Type /Font
/Subtype /Type1
/BaseFont /Helvetica
>>
>>
>>
/MediaBox [0 0 612 792]
/Contents 4 0 R
>>
endobj

4 0 obj

/Length 100
>>
stream
BT
/F1 12 Tf
100 700 Td
(Test Invoice) Tj
100 680 Td
(Item: Test Product - $100.00) Tj
ET
endstream
endobj

xref
0 5
0000000000 65535 f 
0000000009 00000 n 
0000000058 00000 n 
0000000115 00000 n 
0000000358 00000 n 
trailer

/Size 5
/Root 1 0 R
>>
startxref
492
%%EOF`)
}

func TestInventoryE2ESuite(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E tests in short mode")
	}
	suite.Run(t, new(InventoryE2ESuite))
}
