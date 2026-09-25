// Package attachmentstorage implements Delivery attachment bytes on
// S3-compatible object storage. Metadata remains in Delivery's relational
// schema; object bytes never enter MySQL.
package attachmentstorage

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"

	attachmentdomain "github.com/domainry/domainry-delivery/internal/domain/attachment"
)

const defaultPrefix = "domainry-delivery"

type Config struct {
	Region         string
	Bucket         string
	Prefix         string
	Endpoint       string
	ForcePathStyle bool
}

type s3Client interface {
	PutObject(context.Context, *s3.PutObjectInput, ...func(*s3.Options)) (*s3.PutObjectOutput, error)
	GetObject(context.Context, *s3.GetObjectInput, ...func(*s3.Options)) (*s3.GetObjectOutput, error)
}

type Store struct {
	client s3Client
	bucket string
	prefix string
}

func NewFromEnvironment(ctx context.Context) (attachmentdomain.Store, error) {
	if root := strings.TrimSpace(os.Getenv("DELIVERY_ATTACHMENT_STORAGE_PATH")); root != "" {
		if strings.TrimSpace(os.Getenv("DELIVERY_ATTACHMENT_S3_BUCKET")) != "" {
			return nil, errors.New("configure either Delivery attachment file storage or S3, not both")
		}
		return NewFileStore(root)
	}
	config := Config{
		Region:   strings.TrimSpace(os.Getenv("DELIVERY_ATTACHMENT_S3_REGION")),
		Bucket:   strings.TrimSpace(os.Getenv("DELIVERY_ATTACHMENT_S3_BUCKET")),
		Prefix:   strings.TrimSpace(os.Getenv("DELIVERY_ATTACHMENT_S3_PREFIX")),
		Endpoint: strings.TrimSpace(os.Getenv("DELIVERY_ATTACHMENT_S3_ENDPOINT")),
	}
	if raw := strings.TrimSpace(os.Getenv("DELIVERY_ATTACHMENT_S3_FORCE_PATH_STYLE")); raw != "" {
		value, err := strconv.ParseBool(raw)
		if err != nil {
			return nil, fmt.Errorf("parse DELIVERY_ATTACHMENT_S3_FORCE_PATH_STYLE: %w", err)
		}
		config.ForcePathStyle = value
	}
	if config.Region == "" && config.Bucket == "" && config.Prefix == "" && config.Endpoint == "" {
		return nil, errors.New("configure DELIVERY_ATTACHMENT_STORAGE_PATH or Delivery attachment S3 storage")
	}
	if err := config.validate(); err != nil {
		return nil, err
	}
	awsConfiguration, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(config.Region))
	if err != nil {
		return nil, fmt.Errorf("load AWS attachment configuration: %w", err)
	}
	client := s3.NewFromConfig(awsConfiguration, func(options *s3.Options) {
		if config.Endpoint != "" {
			options.BaseEndpoint = aws.String(config.Endpoint)
		}
		options.UsePathStyle = config.ForcePathStyle
	})
	return New(config, client)
}

func New(config Config, client s3Client) (*Store, error) {
	if err := config.validate(); err != nil {
		return nil, err
	}
	if client == nil {
		return nil, errors.New("S3 attachment client is required")
	}
	prefix := strings.Trim(strings.TrimSpace(config.Prefix), "/")
	if prefix == "" {
		prefix = defaultPrefix
	}
	if path.Clean(prefix) != prefix || strings.HasPrefix(prefix, "..") {
		return nil, errors.New("S3 attachment prefix must be a clean relative path")
	}
	return &Store{client: client, bucket: config.Bucket, prefix: prefix}, nil
}

func (config Config) validate() error {
	if strings.TrimSpace(config.Region) == "" {
		return errors.New("DELIVERY_ATTACHMENT_S3_REGION is required")
	}
	if strings.TrimSpace(config.Bucket) == "" {
		return errors.New("DELIVERY_ATTACHMENT_S3_BUCKET is required")
	}
	if config.Endpoint != "" {
		endpoint, err := url.Parse(config.Endpoint)
		if err != nil || endpoint.Host == "" || (endpoint.Scheme != "https" && !isLoopbackHTTP(endpoint)) {
			return errors.New("DELIVERY_ATTACHMENT_S3_ENDPOINT must be HTTPS unless it targets loopback")
		}
	}
	return nil
}

func (store *Store) Put(ctx context.Context, request attachmentdomain.PutRequest) (attachmentdomain.PutResult, error) {
	if strings.TrimSpace(request.ObjectKey) == "" || path.Clean(request.ObjectKey) != request.ObjectKey || strings.HasPrefix(request.ObjectKey, "..") {
		return attachmentdomain.PutResult{}, errors.New("attachment object key must be a clean relative path")
	}
	if int64(len(request.Content)) == 0 || int64(len(request.Content)) > attachmentdomain.MaxUploadBytes {
		return attachmentdomain.PutResult{}, errors.New("attachment content size is invalid")
	}
	digest := sha256.Sum256(request.Content)
	if hex.EncodeToString(digest[:]) != request.ContentSHA256 {
		return attachmentdomain.PutResult{}, errors.New("attachment content hash does not match")
	}
	objectKey := store.prefix + "/" + request.ObjectKey
	idempotencyDigest := sha256.Sum256([]byte(request.IdempotencyKey))
	result, err := store.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(store.bucket), Key: aws.String(objectKey), Body: bytes.NewReader(request.Content),
		ContentLength: aws.Int64(int64(len(request.Content))), ContentType: aws.String(request.ContentType),
		ServerSideEncryption: types.ServerSideEncryptionAes256,
		Metadata: map[string]string{
			"content-sha256":   request.ContentSHA256,
			"idempotency-key":  hex.EncodeToString(idempotencyDigest[:]),
			"workspace-sha256": identityDigest(request.WorkspaceID),
		},
	})
	if err != nil {
		return attachmentdomain.PutResult{}, fmt.Errorf("put S3 attachment: %w", err)
	}
	return attachmentdomain.PutResult{ContentRef: objectKey, ETag: strings.Trim(aws.ToString(result.ETag), "\"")}, nil
}

func (store *Store) Get(ctx context.Context, _ string, contentRef string) ([]byte, error) {
	contentRef = strings.TrimSpace(contentRef)
	if contentRef == "" || path.Clean(contentRef) != contentRef || !strings.HasPrefix(contentRef, store.prefix+"/") {
		return nil, attachmentdomain.ErrContentNotFound
	}
	result, err := store.client.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(store.bucket), Key: aws.String(contentRef)})
	if err != nil {
		var missing *types.NoSuchKey
		if errors.As(err, &missing) {
			return nil, attachmentdomain.ErrContentNotFound
		}
		return nil, fmt.Errorf("get S3 attachment: %w", err)
	}
	defer result.Body.Close()
	content, err := io.ReadAll(io.LimitReader(result.Body, attachmentdomain.MaxUploadBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read S3 attachment: %w", err)
	}
	if int64(len(content)) > attachmentdomain.MaxUploadBytes {
		return nil, errors.New("stored attachment exceeds maximum size")
	}
	return content, nil
}

func identityDigest(value string) string {
	digest := sha256.Sum256([]byte(strings.TrimSpace(value)))
	return hex.EncodeToString(digest[:])
}

func isLoopbackHTTP(value *url.URL) bool {
	if value.Scheme != "http" {
		return false
	}
	hostname := value.Hostname()
	return hostname == "localhost" || hostname == "127.0.0.1" || hostname == "::1"
}

var _ attachmentdomain.Store = (*Store)(nil)
