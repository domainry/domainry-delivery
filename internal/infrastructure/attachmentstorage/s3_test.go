package attachmentstorage

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"

	attachmentdomain "github.com/domainry/domainry-delivery/internal/domain/attachment"
)

type attachmentS3ClientStub struct {
	put  *s3.PutObjectInput
	data []byte
}

func (client *attachmentS3ClientStub) PutObject(_ context.Context, input *s3.PutObjectInput, _ ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	data, err := io.ReadAll(input.Body)
	if err != nil {
		return nil, err
	}
	client.put = input
	client.data = data
	return &s3.PutObjectOutput{ETag: aws.String(`"etag"`)}, nil
}

func (client *attachmentS3ClientStub) GetObject(_ context.Context, input *s3.GetObjectInput, _ ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	if client.put == nil || aws.ToString(input.Bucket) != aws.ToString(client.put.Bucket) || aws.ToString(input.Key) != aws.ToString(client.put.Key) {
		return nil, &types.NoSuchKey{}
	}
	return &s3.GetObjectOutput{Body: io.NopCloser(bytes.NewReader(client.data))}, nil
}

func TestS3StoreKeepsOriginalBytesPrivate(t *testing.T) {
	client := &attachmentS3ClientStub{}
	store, err := New(Config{Region: "us-east-1", Bucket: "verdent-image", Prefix: "delivery"}, client)
	if err != nil {
		t.Fatal(err)
	}
	content := []byte("original attachment bytes")
	digest := sha256.Sum256(content)
	request := attachmentdomain.PutRequest{
		WorkspaceID: "workspace-1", ObjectKey: "20260925/attachment-123-original.txt", ContentType: "text/plain",
		Content: content, ContentSHA256: hex.EncodeToString(digest[:]), IdempotencyKey: "upload-1",
	}
	stored, err := store.Put(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if stored.ContentRef != "delivery/20260925/attachment-123-original.txt" || stored.ETag != "etag" {
		t.Fatalf("stored S3 reference = %+v", stored)
	}
	if aws.ToString(client.put.Bucket) != "verdent-image" || aws.ToString(client.put.Key) != stored.ContentRef || client.put.ServerSideEncryption != types.ServerSideEncryptionAes256 {
		t.Fatalf("S3 write parameters = %+v", client.put)
	}
	if client.put.Metadata["content-sha256"] != request.ContentSHA256 || client.put.Metadata["workspace-sha256"] != identityDigest(request.WorkspaceID) {
		t.Fatalf("S3 object metadata = %+v", client.put.Metadata)
	}
	got, err := store.Get(context.Background(), request.WorkspaceID, stored.ContentRef)
	if err != nil || !bytes.Equal(got, content) {
		t.Fatalf("read original bytes: got=%q err=%v", got, err)
	}
	_, err = store.Get(context.Background(), request.WorkspaceID, "other-prefix/20260925/attachment-123-original.txt")
	if !errors.Is(err, attachmentdomain.ErrContentNotFound) {
		t.Fatalf("other prefix lookup error = %v", err)
	}
}
