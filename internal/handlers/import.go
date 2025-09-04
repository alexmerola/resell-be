// internal/handlers/import.go
package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"

	"github.com/ammerola/resell-be/internal/workers"
)

// ImportHandler handles import operations
type ImportHandler struct {
	asynqClient *asynq.Client
	logger      *slog.Logger
	maxFileSize int64
	uploadDir   string
}

// NewImportHandler creates a new import handler
func NewImportHandler(asynqClient *asynq.Client, logger *slog.Logger, maxFileSize int64, uploadDir string) *ImportHandler {
	return &ImportHandler{
		asynqClient: asynqClient,
		logger:      logger.With(slog.String("handler", "import")),
		maxFileSize: maxFileSize,
		uploadDir:   uploadDir,
	}
}

// ImportPDF handles POST /api/v1/import/pdf
func (h *ImportHandler) ImportPDF(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Parse multipart form (50MB max)
	if err := r.ParseMultipartForm(h.maxFileSize); err != nil {
		h.respondError(w, http.StatusBadRequest, "Failed to parse form data")
		return
	}

	// Get file from form
	file, header, err := r.FormFile("file")
	if err != nil {
		h.respondError(w, http.StatusBadRequest, "File is required")
		return
	}
	defer file.Close()

	// Validate file type
	if header.Header.Get("Content-Type") != "application/pdf" {
		h.respondError(w, http.StatusBadRequest, "Only PDF files are allowed")
		return
	}

	// Get form values
	invoiceID := r.FormValue("invoice_id")
	auctionID := 0
	if aid := r.FormValue("auction_id"); aid != "" {
		fmt.Sscanf(aid, "%d", &auctionID)
	}

	if invoiceID == "" {
		h.respondError(w, http.StatusBadRequest, "invoice_id is required")
		return
	}

	// Create upload directory if it doesn't exist
	if err := os.MkdirAll(h.uploadDir, 0755); err != nil {
		h.logger.ErrorContext(ctx, "failed to create upload directory", err)
		h.respondError(w, http.StatusInternalServerError, "Failed to prepare upload")
		return
	}

	// Save uploaded file
	tempFile := filepath.Join(h.uploadDir, fmt.Sprintf("%s_%s", uuid.New().String(), header.Filename))
	dst, err := os.Create(tempFile)
	if err != nil {
		h.logger.ErrorContext(ctx, "failed to create temp file", err)
		h.respondError(w, http.StatusInternalServerError, "Failed to save upload")
		return
	}
	defer dst.Close()

	if _, err := io.Copy(dst, file); err != nil {
		os.Remove(tempFile)
		h.logger.ErrorContext(ctx, "failed to save file", err)
		h.respondError(w, http.StatusInternalServerError, "Failed to save upload")
		return
	}

	// Create job record
	jobID := uuid.New().String()
	if err := h.createAsyncJob(ctx, jobID, "pdf_import", map[string]interface{}{
		"file_path":  tempFile,
		"invoice_id": invoiceID,
		"auction_id": auctionID,
	}); err != nil {
		os.Remove(tempFile)
		h.logger.ErrorContext(ctx, "failed to create job record", err)
		h.respondError(w, http.StatusInternalServerError, "Failed to create import job")
		return
	}

	// Queue PDF processing task
	payload := workers.PDFJobPayload{
		JobID:     jobID,
		FilePath:  tempFile,
		InvoiceID: invoiceID,
		AuctionID: auctionID,
	}

	b, err := json.Marshal(payload)
	if err != nil {
		os.Remove(tempFile)
		h.logger.ErrorContext(ctx, "failed to marshal PDFJobPayload", err)
		h.respondError(w, http.StatusInternalServerError, "Failed to queue import job")
		return
	}

	task := asynq.NewTask(workers.TypePDFProcess, b)
	if err != nil {
		os.Remove(tempFile)
		h.logger.ErrorContext(ctx, "failed to create task", err)
		h.respondError(w, http.StatusInternalServerError, "Failed to queue import job")
		return
	}

	info, err := h.asynqClient.Enqueue(task,
		asynq.Queue("default"),
		asynq.MaxRetry(3),
		asynq.Retention(24*time.Hour))
	if err != nil {
		os.Remove(tempFile)
		h.logger.ErrorContext(ctx, "failed to enqueue task", err)
		h.respondError(w, http.StatusInternalServerError, "Failed to queue import job")
		return
	}

	h.logger.InfoContext(ctx, "PDF import queued",
		slog.String("job_id", jobID),
		slog.String("task_id", info.ID),
		slog.String("invoice_id", invoiceID))

	h.respondJSON(w, http.StatusAccepted, map[string]interface{}{
		"job_id":  jobID,
		"status":  "queued",
		"message": "PDF import has been queued for processing",
	})
}

// ImportExcel handles POST /api/v1/import/excel
func (h *ImportHandler) ImportExcel(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Similar implementation to ImportPDF but for Excel files
	// Parse multipart form
	if err := r.ParseMultipartForm(h.maxFileSize); err != nil {
		h.respondError(w, http.StatusBadRequest, "Failed to parse form data")
		return
	}

	// Get file from form
	file, header, err := r.FormFile("file")
	if err != nil {
		h.respondError(w, http.StatusBadRequest, "File is required")
		return
	}
	defer file.Close()

	// Validate file type
	contentType := header.Header.Get("Content-Type")
	if contentType != "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet" &&
		contentType != "application/vnd.ms-excel" {
		h.respondError(w, http.StatusBadRequest, "Only Excel files are allowed")
		return
	}

	// Save file and queue for processing
	tempFile := filepath.Join(h.uploadDir, fmt.Sprintf("%s_%s", uuid.New().String(), header.Filename))
	dst, err := os.Create(tempFile)
	if err != nil {
		h.logger.ErrorContext(ctx, "failed to create temp file", err)
		h.respondError(w, http.StatusInternalServerError, "Failed to save upload")
		return
	}
	defer dst.Close()

	if _, err := io.Copy(dst, file); err != nil {
		os.Remove(tempFile)
		h.respondError(w, http.StatusInternalServerError, "Failed to save upload")
		return
	}

	// Create and queue Excel import task
	jobID := uuid.New().String()
	payload := map[string]interface{}{
		"job_id":    jobID,
		"file_path": tempFile,
		"type":      "inventory",
	}

	b, err := json.Marshal(payload)
	if err != nil {
		os.Remove(tempFile)
		h.logger.ErrorContext(ctx, "failed to marshal PDFJobPayload", err)
		h.respondError(w, http.StatusInternalServerError, "Failed to queue import job")
		return
	}

	task := asynq.NewTask(workers.TypeExcelImport, b)
	if err != nil {
		os.Remove(tempFile)
		h.respondError(w, http.StatusInternalServerError, "Failed to create import task")
		return
	}

	info, err := h.asynqClient.Enqueue(task, asynq.Queue("default"))
	if err != nil {
		os.Remove(tempFile)
		h.respondError(w, http.StatusInternalServerError, "Failed to queue import job")
		return
	}

	h.logger.InfoContext(ctx, "Excel import queued",
		slog.String("job_id", jobID),
		slog.String("task_id", info.ID))

	h.respondJSON(w, http.StatusAccepted, map[string]interface{}{
		"job_id":  jobID,
		"status":  "queued",
		"message": "Excel import has been queued for processing",
	})
}

// ImportBatch handles POST /api/v1/import/batch
func (h *ImportHandler) ImportBatch(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Parse multipart form
	if err := r.ParseMultipartForm(h.maxFileSize * 10); err != nil { // Allow larger size for batch
		h.respondError(w, http.StatusBadRequest, "Failed to parse form data")
		return
	}

	fileType := r.FormValue("type")
	if fileType != "pdf" && fileType != "excel" && fileType != "csv" {
		h.respondError(w, http.StatusBadRequest, "Invalid file type. Must be pdf, excel, or csv")
		return
	}

	// Get all files
	files := r.MultipartForm.File["files"]
	if len(files) == 0 {
		h.respondError(w, http.StatusBadRequest, "No files provided")
		return
	}

	batchID := uuid.New().String()
	var jobIDs []string

	for _, fileHeader := range files {
		file, err := fileHeader.Open()
		if err != nil {
			h.logger.WarnContext(ctx, "failed to open file in batch",
				slog.String("filename", fileHeader.Filename),
				err)
			continue
		}
		defer file.Close()

		// Save file
		tempFile := filepath.Join(h.uploadDir, fmt.Sprintf("%s_%s", uuid.New().String(), fileHeader.Filename))
		dst, err := os.Create(tempFile)
		if err != nil {
			h.logger.WarnContext(ctx, "failed to create temp file",
				slog.String("filename", fileHeader.Filename),
				err)
			continue
		}

		if _, err := io.Copy(dst, file); err != nil {
			dst.Close()
			os.Remove(tempFile)
			continue
		}
		dst.Close()

		// Queue processing task based on type
		jobID := uuid.New().String()
		var taskType string

		switch fileType {
		case "pdf":
			taskType = workers.TypePDFProcess
		case "excel", "csv":
			taskType = workers.TypeExcelImport
		}

		payload := map[string]interface{}{
			"job_id":    jobID,
			"batch_id":  batchID,
			"file_path": tempFile,
			"file_type": fileType,
		}

		b, err := json.Marshal(payload)
		if err != nil {
			os.Remove(tempFile)
			h.logger.ErrorContext(ctx, "failed to marshal PDFJobPayload", err)
			h.respondError(w, http.StatusInternalServerError, "Failed to queue import job")
			return
		}

		task := asynq.NewTask(taskType, b)
		if err != nil {
			os.Remove(tempFile)
			continue
		}

		if _, err := h.asynqClient.Enqueue(task, asynq.Queue("low")); err != nil {
			os.Remove(tempFile)
			continue
		}

		jobIDs = append(jobIDs, jobID)
	}

	h.logger.InfoContext(ctx, "Batch import queued",
		slog.String("batch_id", batchID),
		slog.Int("total_files", len(files)),
		slog.Int("queued_jobs", len(jobIDs)))

	h.respondJSON(w, http.StatusAccepted, map[string]interface{}{
		"batch_id":    batchID,
		"job_ids":     jobIDs,
		"total_files": len(files),
		"queued_jobs": len(jobIDs),
		"status":      "queued",
		"message":     fmt.Sprintf("Batch import of %d files has been queued", len(jobIDs)),
	})
}

// ImportStatus handles GET /api/v1/import/status/{jobId}
func (h *ImportHandler) ImportStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	jobID := r.PathValue("jobId")

	// Query job status from database
	status, err := h.getJobStatus(ctx, jobID)
	if err != nil {
		h.logger.ErrorContext(ctx, "failed to get job status",
			slog.String("job_id", jobID),
			err)
		h.respondError(w, http.StatusInternalServerError, "Failed to get job status")
		return
	}

	if status == nil {
		h.respondError(w, http.StatusNotFound, "Job not found")
		return
	}

	h.respondJSON(w, http.StatusOK, status)
}

// Helper methods
func (h *ImportHandler) createAsyncJob(ctx context.Context, jobID string, jobType string, payload interface{}) error {
	// This would insert a job record into the async_jobs table
	// Implementation depends on your database setup
	return nil
}

func (h *ImportHandler) getJobStatus(ctx context.Context, jobID string) (map[string]interface{}, error) {
	// This would query the async_jobs table for status
	// Placeholder implementation
	return map[string]interface{}{
		"job_id":     jobID,
		"status":     "processing",
		"progress":   50,
		"created_at": time.Now().Add(-5 * time.Minute),
		"started_at": time.Now().Add(-4 * time.Minute),
		"message":    "Processing items...",
	}, nil
}

func (h *ImportHandler) respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func (h *ImportHandler) respondError(w http.ResponseWriter, status int, message string) {
	h.respondJSON(w, status, map[string]string{"error": message})
}
