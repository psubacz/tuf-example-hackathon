package storage

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// S3Backend implements storage backend for AWS S3
type S3Backend struct {
	client   *s3.Client
	bucket   string
	prefix   string
	region   string
	endpoint string // For S3-compatible services
}

// NewS3Backend creates a new S3 storage backend
func NewS3Backend(cfg Config) (Backend, error) {
	bucket, ok := cfg.Properties["bucket"].(string)
	if !ok || bucket == "" {
		return nil, fmt.Errorf("S3 backend requires 'bucket' property")
	}
	
	prefix, _ := cfg.Properties["prefix"].(string)
	region, _ := cfg.Properties["region"].(string)
	if region == "" {
		region = "us-east-1"
	}
	
	endpoint, _ := cfg.Properties["endpoint"].(string)
	
	// Load AWS configuration
	awsCfg, err := config.LoadDefaultConfig(context.TODO(),
		config.WithRegion(region),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}
	
	// Create S3 client options
	clientOptions := func(o *s3.Options) {
		if endpoint != "" {
			o.BaseEndpoint = aws.String(endpoint)
			o.UsePathStyle = true // Often needed for S3-compatible services
		}
	}
	
	// Create S3 client
	client := s3.NewFromConfig(awsCfg, clientOptions)
	
	return &S3Backend{
		client:   client,
		bucket:   bucket,
		prefix:   prefix,
		region:   region,
		endpoint: endpoint,
	}, nil
}

// Get retrieves a file from S3
func (s *S3Backend) Get(ctx context.Context, path string) (io.ReadCloser, error) {
	key := s.fullKey(path)
	
	result, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		// Check if object doesn't exist
		if isNotFoundError(err) {
			return nil, &ErrNotFound{Path: path}
		}
		return nil, fmt.Errorf("failed to get object: %w", err)
	}
	
	return result.Body, nil
}

// GetWithInfo retrieves a file with metadata from S3
func (s *S3Backend) GetWithInfo(ctx context.Context, path string) (*Object, error) {
	key := s.fullKey(path)
	
	result, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		if isNotFoundError(err) {
			return nil, &ErrNotFound{Path: path}
		}
		return nil, fmt.Errorf("failed to get object: %w", err)
	}
	
	metadata := make(map[string]string)
	for k, v := range result.Metadata {
		metadata[k] = v
	}
	
	var etag string
	if result.ETag != nil {
		etag = strings.Trim(*result.ETag, "\"")
	}
	
	var lastModified time.Time
	if result.LastModified != nil {
		lastModified = *result.LastModified
	}
	
	var contentType string
	if result.ContentType != nil {
		contentType = *result.ContentType
	}
	
	var size int64
	if result.ContentLength != nil {
		size = *result.ContentLength
	}
	
	return &Object{
		ReadCloser: result.Body,
		Info: ObjectInfo{
			Path:         path,
			Size:         size,
			LastModified: lastModified,
			ETag:         etag,
			ContentType:  contentType,
			Metadata:     metadata,
		},
	}, nil
}

// Put stores a file in S3
func (s *S3Backend) Put(ctx context.Context, path string, reader io.Reader) error {
	key := s.fullKey(path)
	
	// Read content into buffer (needed for S3)
	buf := new(bytes.Buffer)
	size, err := io.Copy(buf, reader)
	if err != nil {
		return fmt.Errorf("failed to read content: %w", err)
	}
	
	_, err = s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(s.bucket),
		Key:           aws.String(key),
		Body:          bytes.NewReader(buf.Bytes()),
		ContentLength: aws.Int64(size),
		ContentType:   aws.String(getContentType(path)),
	})
	if err != nil {
		return fmt.Errorf("failed to put object: %w", err)
	}
	
	return nil
}

// PutWithMetadata stores a file with metadata in S3
func (s *S3Backend) PutWithMetadata(ctx context.Context, path string, reader io.Reader, metadata map[string]string) error {
	key := s.fullKey(path)
	
	// Read content into buffer
	buf := new(bytes.Buffer)
	size, err := io.Copy(buf, reader)
	if err != nil {
		return fmt.Errorf("failed to read content: %w", err)
	}
	
	_, err = s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(s.bucket),
		Key:           aws.String(key),
		Body:          bytes.NewReader(buf.Bytes()),
		ContentLength: aws.Int64(size),
		ContentType:   aws.String(getContentType(path)),
		Metadata:      metadata,
	})
	if err != nil {
		return fmt.Errorf("failed to put object: %w", err)
	}
	
	return nil
}

// Delete removes a file from S3
func (s *S3Backend) Delete(ctx context.Context, path string) error {
	key := s.fullKey(path)
	
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("failed to delete object: %w", err)
	}
	
	return nil
}

// Exists checks if a file exists in S3
func (s *S3Backend) Exists(ctx context.Context, path string) (bool, error) {
	key := s.fullKey(path)
	
	_, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		if isNotFoundError(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to head object: %w", err)
	}
	
	return true, nil
}

// List lists files in S3 with a prefix
func (s *S3Backend) List(ctx context.Context, prefix string) ([]string, error) {
	fullPrefix := s.fullKey(prefix)
	
	var files []string
	paginator := s3.NewListObjectsV2Paginator(s.client, &s3.ListObjectsV2Input{
		Bucket: aws.String(s.bucket),
		Prefix: aws.String(fullPrefix),
	})
	
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list objects: %w", err)
		}
		
		for _, obj := range page.Contents {
			if obj.Key != nil {
				// Remove the backend prefix from the key
				path := strings.TrimPrefix(*obj.Key, s.prefix)
				path = strings.TrimPrefix(path, "/")
				files = append(files, path)
			}
		}
	}
	
	return files, nil
}

// ListWithInfo lists files with metadata from S3
func (s *S3Backend) ListWithInfo(ctx context.Context, prefix string) ([]*Object, error) {
	fullPrefix := s.fullKey(prefix)
	
	var objects []*Object
	paginator := s3.NewListObjectsV2Paginator(s.client, &s3.ListObjectsV2Input{
		Bucket: aws.String(s.bucket),
		Prefix: aws.String(fullPrefix),
	})
	
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list objects: %w", err)
		}
		
		for _, obj := range page.Contents {
			if obj.Key == nil {
				continue
			}
			
			path := strings.TrimPrefix(*obj.Key, s.prefix)
			path = strings.TrimPrefix(path, "/")
			
			var etag string
			if obj.ETag != nil {
				etag = strings.Trim(*obj.ETag, "\"")
			}
			
			var lastModified time.Time
			if obj.LastModified != nil {
				lastModified = *obj.LastModified
			}
			
			var size int64
			if obj.Size != nil {
				size = *obj.Size
			}
			
			objects = append(objects, &Object{
				ReadCloser: nil, // Not providing content in list operation
				Info: ObjectInfo{
					Path:         path,
					Size:         size,
					LastModified: lastModified,
					ETag:         etag,
					ContentType:  getContentType(path),
					Metadata:     make(map[string]string),
				},
			})
		}
	}
	
	return objects, nil
}

// Stat gets file metadata without downloading from S3
func (s *S3Backend) Stat(ctx context.Context, path string) (*ObjectInfo, error) {
	key := s.fullKey(path)
	
	result, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		if isNotFoundError(err) {
			return nil, &ErrNotFound{Path: path}
		}
		return nil, fmt.Errorf("failed to head object: %w", err)
	}
	
	metadata := make(map[string]string)
	for k, v := range result.Metadata {
		metadata[k] = v
	}
	
	var etag string
	if result.ETag != nil {
		etag = strings.Trim(*result.ETag, "\"")
	}
	
	var lastModified time.Time
	if result.LastModified != nil {
		lastModified = *result.LastModified
	}
	
	var contentType string
	if result.ContentType != nil {
		contentType = *result.ContentType
	}
	
	var size int64
	if result.ContentLength != nil {
		size = *result.ContentLength
	}
	
	return &ObjectInfo{
		Path:         path,
		Size:         size,
		LastModified: lastModified,
		ETag:         etag,
		ContentType:  contentType,
		Metadata:     metadata,
	}, nil
}

// Copy copies a file within S3
func (s *S3Backend) Copy(ctx context.Context, src, dst string) error {
	srcKey := s.fullKey(src)
	dstKey := s.fullKey(dst)
	copySource := fmt.Sprintf("%s/%s", s.bucket, srcKey)
	
	_, err := s.client.CopyObject(ctx, &s3.CopyObjectInput{
		Bucket:     aws.String(s.bucket),
		CopySource: aws.String(copySource),
		Key:        aws.String(dstKey),
	})
	if err != nil {
		if isNotFoundError(err) {
			return &ErrNotFound{Path: src}
		}
		return fmt.Errorf("failed to copy object: %w", err)
	}
	
	return nil
}

// Move moves a file within S3 (copy and delete)
func (s *S3Backend) Move(ctx context.Context, src, dst string) error {
	// Copy the object
	if err := s.Copy(ctx, src, dst); err != nil {
		return err
	}
	
	// Delete the source
	return s.Delete(ctx, src)
}

// GetURL generates a pre-signed URL for S3
func (s *S3Backend) GetURL(ctx context.Context, path string, expiry time.Duration) (string, error) {
	key := s.fullKey(path)
	
	presignClient := s3.NewPresignClient(s.client)
	
	request, err := presignClient.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	}, func(opts *s3.PresignOptions) {
		opts.Expires = expiry
	})
	if err != nil {
		return "", fmt.Errorf("failed to generate presigned URL: %w", err)
	}
	
	return request.URL, nil
}

// SupportsDirectURL returns true for S3 backend
func (s *S3Backend) SupportsDirectURL() bool {
	return true
}

// Close closes the S3 backend (no-op for S3)
func (s *S3Backend) Close() error {
	return nil
}

// Helper methods

func (s *S3Backend) fullKey(path string) string {
	path = strings.TrimPrefix(path, "/")
	if s.prefix != "" {
		return fmt.Sprintf("%s/%s", strings.TrimSuffix(s.prefix, "/"), path)
	}
	return path
}

func isNotFoundError(err error) bool {
	// Check for various not found error types
	errStr := err.Error()
	return strings.Contains(errStr, "NoSuchKey") ||
		strings.Contains(errStr, "NotFound") ||
		strings.Contains(errStr, "404")
}

func init() {
	// Register S3 backend
	Register("s3", NewS3Backend)
}