// internal/adapters/storage/s3.go
package storage

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/google/uuid"
)

// StorageClient defines the interface for file storage operations
type StorageClient interface {
	Upload(ctx context.Context, key string, data io.Reader, contentType string) (string, error)
	Download(ctx context.Context, key string) ([]byte, error)
	Delete(ctx context.Context, key string) error
	GetPresignedURL(ctx context.Context, key string, duration time.Duration) (string, error)
	List(ctx context.Context, prefix string) ([]string, error)
	Copy(ctx context.Context, sourceKey, destinationKey string) error
	Exists(ctx context.Context, key string) (bool, error)
}

// S3Storage implements StorageClient using AWS S3
type S3Storage struct {
	client     *s3.Client
	uploader   *manager.Uploader
	downloader *manager.Downloader
	bucket     string
	region     string
	logger     *slog.Logger
}

// S3Config holds S3 configuration
type S3Config struct {
	Region          string
	Bucket          string
	AccessKeyID     string
	SecretAccessKey string
	Endpoint        string // For MinIO/LocalStack
	UsePathStyle    bool   // For MinIO/LocalStack
}

// NewS3Storage creates a new S3 storage client
func NewS3Storage(ctx context.Context, cfg *S3Config, logger *slog.Logger) (*S3Storage, error) {
	// Build AWS config
	awsCfg, err := buildAWSConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to build AWS config: %w", err)
	}

	// Create S3 client
	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		if cfg.Endpoint != "" {
			o.EndpointResolver = s3.EndpointResolverFromURL(cfg.Endpoint)
		}
		o.UsePathStyle = cfg.UsePathStyle
	})

	// Create uploader and downloader
	uploader := manager.NewUploader(client)
	downloader := manager.NewDownloader(client)

	storage := &S3Storage{
		client:     client,
		uploader:   uploader,
		downloader: downloader,
		bucket:     cfg.Bucket,
		region:     cfg.Region,
		logger:     logger.With(slog.String("storage", "s3")),
	}

	// Verify bucket exists
	if err := storage.ensureBucket(ctx); err != nil {
		return nil, fmt.Errorf("failed to ensure bucket: %w", err)
	}

	logger.Info("S3 storage initialized",
		slog.String("bucket", cfg.Bucket),
		slog.String("region", cfg.Region))

	return storage, nil
}

// buildAWSConfig builds AWS configuration
func buildAWSConfig(ctx context.Context, cfg *S3Config) (aws.Config, error) {
	// Use custom credentials if provided
	if cfg.AccessKeyID != "" && cfg.SecretAccessKey != "" {
		return config.LoadDefaultConfig(ctx,
			config.WithRegion(cfg.Region),
			config.WithCredentialsProvider(
				credentials.NewStaticCredentialsProvider(
					cfg.AccessKeyID,
					cfg.SecretAccessKey,
					"",
				),
			),
		)
	}

	// Otherwise use default credential chain
	return config.LoadDefaultConfig(ctx, config.WithRegion(cfg.Region))
}

// ensureBucket ensures the bucket exists
func (s *S3Storage) ensureBucket(ctx context.Context) error {
	_, err := s.client.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: aws.String(s.bucket),
	})

	if err != nil {
		// Try to create bucket if it doesn't exist
		_, createErr := s.client.CreateBucket(ctx, &s3.CreateBucketInput{
			Bucket: aws.String(s.bucket),
			CreateBucketConfiguration: &types.CreateBucketConfiguration{
				LocationConstraint: types.BucketLocationConstraint(s.region),
			},
		})

		if createErr != nil {
			return fmt.Errorf("bucket %s does not exist and could not be created: %w", s.bucket, createErr)
		}

		s.logger.Info("created S3 bucket", slog.String("bucket", s.bucket))
	}

	return nil
}

// Upload uploads a file to S3
func (s *S3Storage) Upload(ctx context.Context, key string, data io.Reader, contentType string) (string, error) {
	// Determine content type if not provided
	if contentType == "" {
		contentType = mime.TypeByExtension(filepath.Ext(key))
		if contentType == "" {
			contentType = "application/octet-stream"
		}
	}

	// Prepare upload input
	input := &s3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(key),
		Body:        data,
		ContentType: aws.String(contentType),
		Metadata: map[string]string{
			"uploaded-at": time.Now().Format(time.RFC3339),
			"upload-id":   uuid.New().String(),
		},
	}

	// Perform upload
	result, err := s.uploader.Upload(ctx, input)
	if err != nil {
		return "", fmt.Errorf("failed to upload file: %w", err)
	}

	s.logger.InfoContext(ctx, "file uploaded",
		slog.String("key", key),
		slog.String("location", result.Location))

	return result.Location, nil
}

// Download downloads a file from S3
func (s *S3Storage) Download(ctx context.Context, key string) ([]byte, error) {
	// Create a buffer to write to
	buf := manager.NewWriteAtBuffer([]byte{})

	// Download the file
	_, err := s.downloader.Download(ctx, buf, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})

	if err != nil {
		return nil, fmt.Errorf("failed to download file: %w", err)
	}

	s.logger.DebugContext(ctx, "file downloaded",
		slog.String("key", key),
		slog.Int("size", len(buf.Bytes())))

	return buf.Bytes(), nil
}

// Delete deletes a file from S3
func (s *S3Storage) Delete(ctx context.Context, key string) error {
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})

	if err != nil {
		return fmt.Errorf("failed to delete file: %w", err)
	}

	s.logger.InfoContext(ctx, "file deleted", slog.String("key", key))
	return nil
}

// GetPresignedURL generates a pre-signed URL for downloading
func (s *S3Storage) GetPresignedURL(ctx context.Context, key string, duration time.Duration) (string, error) {
	presignClient := s3.NewPresignClient(s.client)

	request, err := presignClient.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	}, func(opts *s3.PresignOptions) {
		opts.Expires = duration
	})

	if err != nil {
		return "", fmt.Errorf("failed to create presigned URL: %w", err)
	}

	return request.URL, nil
}

// List lists files with a given prefix
func (s *S3Storage) List(ctx context.Context, prefix string) ([]string, error) {
	var keys []string

	paginator := s3.NewListObjectsV2Paginator(s.client, &s3.ListObjectsV2Input{
		Bucket: aws.String(s.bucket),
		Prefix: aws.String(prefix),
	})

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list objects: %w", err)
		}

		for _, obj := range page.Contents {
			keys = append(keys, *obj.Key)
		}
	}

	s.logger.DebugContext(ctx, "listed files",
		slog.String("prefix", prefix),
		slog.Int("count", len(keys)))

	return keys, nil
}

// Copy copies a file within S3
func (s *S3Storage) Copy(ctx context.Context, sourceKey, destinationKey string) error {
	copySource := fmt.Sprintf("%s/%s", s.bucket, sourceKey)

	_, err := s.client.CopyObject(ctx, &s3.CopyObjectInput{
		Bucket:     aws.String(s.bucket),
		CopySource: aws.String(copySource),
		Key:        aws.String(destinationKey),
	})

	if err != nil {
		return fmt.Errorf("failed to copy file: %w", err)
	}

	s.logger.InfoContext(ctx, "file copied",
		slog.String("source", sourceKey),
		slog.String("destination", destinationKey))

	return nil
}

// Exists checks if a file exists in S3
func (s *S3Storage) Exists(ctx context.Context, key string) (bool, error) {
	_, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})

	if err != nil {
		// Check if it's a not found error
		if strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "NotFound") {
			return false, nil
		}
		return false, fmt.Errorf("failed to check file existence: %w", err)
	}

	return true, nil
}

// UploadWithMetadata uploads a file with custom metadata
func (s *S3Storage) UploadWithMetadata(ctx context.Context, key string, data io.Reader, contentType string, metadata map[string]string) (string, error) {
	// Determine content type if not provided
	if contentType == "" {
		contentType = mime.TypeByExtension(filepath.Ext(key))
		if contentType == "" {
			contentType = "application/octet-stream"
		}
	}

	// Merge default metadata with provided metadata
	meta := map[string]string{
		"uploaded-at": time.Now().Format(time.RFC3339),
		"upload-id":   uuid.New().String(),
	}
	for k, v := range metadata {
		meta[k] = v
	}

	// Prepare upload input
	input := &s3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(key),
		Body:        data,
		ContentType: aws.String(contentType),
		Metadata:    meta,
	}

	// Perform upload
	result, err := s.uploader.Upload(ctx, input)
	if err != nil {
		return "", fmt.Errorf("failed to upload file with metadata: %w", err)
	}

	return result.Location, nil
}

// GetMetadata retrieves metadata for a file
func (s *S3Storage) GetMetadata(ctx context.Context, key string) (map[string]string, error) {
	result, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})

	if err != nil {
		return nil, fmt.Errorf("failed to get file metadata: %w", err)
	}

	return result.Metadata, nil
}

// GenerateUploadPresignedURL generates a pre-signed URL for uploading
func (s *S3Storage) GenerateUploadPresignedURL(ctx context.Context, key string, contentType string, duration time.Duration) (string, error) {
	presignClient := s3.NewPresignClient(s.client)

	request, err := presignClient.PresignPutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(key),
		ContentType: aws.String(contentType),
	}, func(opts *s3.PresignOptions) {
		opts.Expires = duration
	})

	if err != nil {
		return "", fmt.Errorf("failed to create upload presigned URL: %w", err)
	}

	return request.URL, nil
}

// DeleteMultiple deletes multiple files from S3
func (s *S3Storage) DeleteMultiple(ctx context.Context, keys []string) error {
	if len(keys) == 0 {
		return nil
	}

	var objects []types.ObjectIdentifier
	for _, key := range keys {
		objects = append(objects, types.ObjectIdentifier{
			Key: aws.String(key),
		})
	}

	_, err := s.client.DeleteObjects(ctx, &s3.DeleteObjectsInput{
		Bucket: aws.String(s.bucket),
		Delete: &types.Delete{
			Objects: objects,
			Quiet:   *aws.Bool(true),
		},
	})

	if err != nil {
		return fmt.Errorf("failed to delete multiple files: %w", err)
	}

	s.logger.InfoContext(ctx, "multiple files deleted", slog.Int("count", len(keys)))
	return nil
}

// StreamUpload uploads a file using streaming for large files
func (s *S3Storage) StreamUpload(ctx context.Context, key string, reader io.Reader, size int64, contentType string) error {
	// For large files, use multipart upload
	_, err := s.uploader.Upload(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(s.bucket),
		Key:           aws.String(key),
		Body:          reader,
		ContentType:   aws.String(contentType),
		ContentLength: *aws.Int64(size),
	})

	if err != nil {
		return fmt.Errorf("failed to stream upload file: %w", err)
	}

	s.logger.InfoContext(ctx, "file stream uploaded",
		slog.String("key", key),
		slog.Int64("size", size))

	return nil
}

// LocalStorage implements StorageClient using local filesystem (for testing)
type LocalStorage struct {
	basePath string
	logger   *slog.Logger
}

// NewLocalStorage creates a new local storage client
func NewLocalStorage(basePath string, logger *slog.Logger) *LocalStorage {
	return &LocalStorage{
		basePath: basePath,
		logger:   logger.With(slog.String("storage", "local")),
	}
}

// Upload saves a file locally
func (l *LocalStorage) Upload(ctx context.Context, key string, data io.Reader, contentType string) (string, error) {
	// Implementation for local file storage
	// This is useful for testing without AWS
	path := filepath.Join(l.basePath, key)

	// Read data
	buf := new(bytes.Buffer)
	if _, err := io.Copy(buf, data); err != nil {
		return "", err
	}

	// TODO: Save to file
	//

	return path, nil
}

// Other LocalStorage methods would be implemented similarly...
