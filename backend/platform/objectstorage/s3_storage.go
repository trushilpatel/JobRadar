package objectstorage

import (
	"context"
	"errors"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// Config holds the connection details for an S3-compatible bucket.
// Env vars: OCI_S3_ENDPOINT, OCI_S3_REGION, OCI_S3_BUCKET, OCI_S3_ACCESS_KEY,
// OCI_S3_SECRET_KEY (see common/config.ObjectStorageConfig).
type Config struct {
	Endpoint  string
	Region    string
	Bucket    string
	AccessKey string
	SecretKey string
}

// objectAPI is a narrow interface over only the S3 client methods this
// package calls. Depending on this instead of the concrete *s3.Client lets
// tests substitute a hand-written fake with zero real network calls
// (Interface Segregation — callers only see what they use).
type objectAPI interface {
	PutObject(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error)
	DeleteObject(ctx context.Context, params *s3.DeleteObjectInput, optFns ...func(*s3.Options)) (*s3.DeleteObjectOutput, error)
	GetObject(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error)
}

// S3Storage implements Storage against any S3-compatible endpoint.
type S3Storage struct {
	client objectAPI
	bucket string
}

// NewS3Storage builds a client pointed at cfg.Endpoint using path-style
// addressing (required by OCI's S3-compatible API) and static Customer
// Secret Key credentials.
func NewS3Storage(ctx context.Context, cfg Config) (*S3Storage, error) {
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(cfg.Region),
		awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(cfg.AccessKey, cfg.SecretKey, ""),
		),
	)
	if err != nil {
		return nil, err
	}
	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(cfg.Endpoint)
		o.UsePathStyle = true
		// aws-sdk-go-v2 defaults to sending a flexible-checksum trailer over
		// aws-chunked transfer encoding on PutObject. OCI's S3-compatible
		// endpoint rejects that encoding outright (501 NotImplemented: "AWS
		// chunked encoding not supported"), so both must be pinned to
		// WhenRequired — the pre-flexible-checksum behavior every
		// S3-compatible provider actually supports.
		o.RequestChecksumCalculation = aws.RequestChecksumCalculationWhenRequired
		o.ResponseChecksumValidation = aws.ResponseChecksumValidationWhenRequired
	})
	return &S3Storage{
		client: client,
		bucket: cfg.Bucket,
	}, nil
}

func (s *S3Storage) Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error {
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(s.bucket),
		Key:           aws.String(key),
		Body:          r,
		ContentLength: aws.Int64(size),
		ContentType:   aws.String(contentType),
	})
	return err
}

func (s *S3Storage) Delete(ctx context.Context, key string) error {
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	return err
}

func (s *S3Storage) Get(ctx context.Context, key string) (io.ReadCloser, int64, error) {
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		var noSuchKey *s3types.NoSuchKey
		if errors.As(err, &noSuchKey) {
			return nil, 0, ErrNotFound
		}
		return nil, 0, err
	}
	size := int64(0)
	if out.ContentLength != nil {
		size = *out.ContentLength
	}
	return out.Body, size, nil
}

var _ Storage = (*S3Storage)(nil)
