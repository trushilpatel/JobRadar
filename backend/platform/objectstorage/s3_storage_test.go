package objectstorage

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
)

type fakeObjectAPI struct {
	putErr      error
	deleteErr   error
	getOutput   *s3.GetObjectOutput
	getErr      error
	putCalls    int
	deleteCalls int
}

func (f *fakeObjectAPI) PutObject(_ context.Context, _ *s3.PutObjectInput, _ ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	f.putCalls++
	if f.putErr != nil {
		return nil, f.putErr
	}
	return &s3.PutObjectOutput{}, nil
}

func (f *fakeObjectAPI) DeleteObject(_ context.Context, _ *s3.DeleteObjectInput, _ ...func(*s3.Options)) (*s3.DeleteObjectOutput, error) {
	f.deleteCalls++
	if f.deleteErr != nil {
		return nil, f.deleteErr
	}
	return &s3.DeleteObjectOutput{}, nil
}

func (f *fakeObjectAPI) GetObject(_ context.Context, _ *s3.GetObjectInput, _ ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	return f.getOutput, nil
}

func TestS3Storage_PutSucceeds(t *testing.T) {
	fake := &fakeObjectAPI{}
	s := &S3Storage{client: fake, bucket: "b"}
	err := s.Put(context.Background(), "k", bytes.NewReader([]byte("data")), 4, "application/pdf")
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if fake.putCalls != 1 {
		t.Fatalf("expected 1 PutObject call, got %d", fake.putCalls)
	}
}

func TestS3Storage_PutPropagatesError(t *testing.T) {
	fake := &fakeObjectAPI{putErr: errors.New("network error")}
	s := &S3Storage{client: fake, bucket: "b"}
	if err := s.Put(context.Background(), "k", bytes.NewReader([]byte("d")), 1, "application/pdf"); err == nil {
		t.Fatal("expected error to propagate")
	}
}

func TestS3Storage_DeleteSucceeds(t *testing.T) {
	fake := &fakeObjectAPI{}
	s := &S3Storage{client: fake, bucket: "b"}
	if err := s.Delete(context.Background(), "k"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if fake.deleteCalls != 1 {
		t.Fatalf("expected 1 DeleteObject call, got %d", fake.deleteCalls)
	}
}

func TestS3Storage_DeletePropagatesError(t *testing.T) {
	fake := &fakeObjectAPI{deleteErr: errors.New("network error")}
	s := &S3Storage{client: fake, bucket: "b"}
	if err := s.Delete(context.Background(), "k"); err == nil {
		t.Fatal("expected error to propagate")
	}
}

func TestS3Storage_GetReturnsReaderAndSize(t *testing.T) {
	fake := &fakeObjectAPI{getOutput: &s3.GetObjectOutput{
		Body:          io.NopCloser(bytes.NewReader([]byte("hello"))),
		ContentLength: aws.Int64(5),
	}}
	s := &S3Storage{client: fake, bucket: "b"}

	r, size, err := s.Get(context.Background(), "k")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer r.Close()
	if size != 5 {
		t.Fatalf("expected size 5, got %d", size)
	}
	data, readErr := io.ReadAll(r)
	if readErr != nil {
		t.Fatalf("ReadAll: %v", readErr)
	}
	if string(data) != "hello" {
		t.Fatalf("expected %q, got %q", "hello", data)
	}
}

func TestS3Storage_GetPropagatesError(t *testing.T) {
	fake := &fakeObjectAPI{getErr: errors.New("network error")}
	s := &S3Storage{client: fake, bucket: "b"}
	if _, _, err := s.Get(context.Background(), "k"); err == nil {
		t.Fatal("expected error to propagate")
	}
}

func TestS3Storage_GetMapsNoSuchKeyToErrNotFound(t *testing.T) {
	fake := &fakeObjectAPI{getErr: &s3types.NoSuchKey{}}
	s := &S3Storage{client: fake, bucket: "b"}
	if _, _, err := s.Get(context.Background(), "k"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestNewS3Storage_BuildsClientWithoutNetworkCall(t *testing.T) {
	// LoadDefaultConfig with an explicit static credentials provider and
	// region does no network I/O — it only assembles the client config.
	s, err := NewS3Storage(context.Background(), Config{
		Endpoint:  "https://example-namespace.compat.objectstorage.us-ashburn-1.oraclecloud.com",
		Region:    "us-ashburn-1",
		Bucket:    "jobradar-resumes",
		AccessKey: "test-access-key",
		SecretKey: "test-secret-key",
	})
	if err != nil {
		t.Fatalf("NewS3Storage: %v", err)
	}
	if s.bucket != "jobradar-resumes" {
		t.Fatalf("expected bucket to be set, got %s", s.bucket)
	}
	if s.client == nil {
		t.Fatal("expected client to be initialized")
	}
}

// TestNewS3Storage_DisablesFlexibleChecksums guards against regressing to
// aws-sdk-go-v2's default flexible-checksum trailer behavior, which OCI's
// S3-compatible endpoint rejects outright (501 NotImplemented: "AWS chunked
// encoding not supported") — confirmed against a real OCI bucket.
func TestNewS3Storage_DisablesFlexibleChecksums(t *testing.T) {
	s, err := NewS3Storage(context.Background(), Config{
		Endpoint:  "https://example-namespace.compat.objectstorage.us-ashburn-1.oraclecloud.com",
		Region:    "us-ashburn-1",
		Bucket:    "jobradar-resumes",
		AccessKey: "test-access-key",
		SecretKey: "test-secret-key",
	})
	if err != nil {
		t.Fatalf("NewS3Storage: %v", err)
	}
	client, ok := s.client.(*s3.Client)
	if !ok {
		t.Fatalf("expected s.client to be *s3.Client, got %T", s.client)
	}
	opts := client.Options()
	if opts.RequestChecksumCalculation != aws.RequestChecksumCalculationWhenRequired {
		t.Fatalf("RequestChecksumCalculation = %v, want WhenRequired", opts.RequestChecksumCalculation)
	}
	if opts.ResponseChecksumValidation != aws.ResponseChecksumValidationWhenRequired {
		t.Fatalf("ResponseChecksumValidation = %v, want WhenRequired", opts.ResponseChecksumValidation)
	}
}
