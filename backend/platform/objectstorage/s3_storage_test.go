package objectstorage

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type fakePutDeleteAPI struct {
	putErr      error
	deleteErr   error
	putCalls    int
	deleteCalls int
}

func (f *fakePutDeleteAPI) PutObject(_ context.Context, _ *s3.PutObjectInput, _ ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	f.putCalls++
	if f.putErr != nil {
		return nil, f.putErr
	}
	return &s3.PutObjectOutput{}, nil
}

func (f *fakePutDeleteAPI) DeleteObject(_ context.Context, _ *s3.DeleteObjectInput, _ ...func(*s3.Options)) (*s3.DeleteObjectOutput, error) {
	f.deleteCalls++
	if f.deleteErr != nil {
		return nil, f.deleteErr
	}
	return &s3.DeleteObjectOutput{}, nil
}

type fakePresignAPI struct {
	url string
	err error
}

func (f *fakePresignAPI) PresignGetObject(_ context.Context, _ *s3.GetObjectInput, _ ...func(*s3.PresignOptions)) (*v4.PresignedHTTPRequest, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &v4.PresignedHTTPRequest{URL: f.url}, nil
}

func TestS3Storage_PutSucceeds(t *testing.T) {
	fake := &fakePutDeleteAPI{}
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
	fake := &fakePutDeleteAPI{putErr: errors.New("network error")}
	s := &S3Storage{client: fake, bucket: "b"}
	if err := s.Put(context.Background(), "k", bytes.NewReader([]byte("d")), 1, "application/pdf"); err == nil {
		t.Fatal("expected error to propagate")
	}
}

func TestS3Storage_DeleteSucceeds(t *testing.T) {
	fake := &fakePutDeleteAPI{}
	s := &S3Storage{client: fake, bucket: "b"}
	if err := s.Delete(context.Background(), "k"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if fake.deleteCalls != 1 {
		t.Fatalf("expected 1 DeleteObject call, got %d", fake.deleteCalls)
	}
}

func TestS3Storage_DeletePropagatesError(t *testing.T) {
	fake := &fakePutDeleteAPI{deleteErr: errors.New("network error")}
	s := &S3Storage{client: fake, bucket: "b"}
	if err := s.Delete(context.Background(), "k"); err == nil {
		t.Fatal("expected error to propagate")
	}
}

func TestS3Storage_PresignGetReturnsURL(t *testing.T) {
	s := &S3Storage{presign: &fakePresignAPI{url: "https://example.test/signed"}, bucket: "b"}
	url, err := s.PresignGet(context.Background(), "k", 5*time.Minute)
	if err != nil {
		t.Fatalf("PresignGet: %v", err)
	}
	if url != "https://example.test/signed" {
		t.Fatalf("expected signed URL, got %s", url)
	}
}

func TestS3Storage_PresignGetPropagatesError(t *testing.T) {
	s := &S3Storage{presign: &fakePresignAPI{err: errors.New("sign failed")}, bucket: "b"}
	if _, err := s.PresignGet(context.Background(), "k", time.Minute); err == nil {
		t.Fatal("expected error to propagate")
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
	if s.client == nil || s.presign == nil {
		t.Fatal("expected client and presign to be initialized")
	}
}
