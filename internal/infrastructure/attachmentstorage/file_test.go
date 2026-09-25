package attachmentstorage

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"path/filepath"
	"testing"

	attachmentdomain "github.com/domainry/domainry-delivery/internal/domain/attachment"
)

func TestFileStoreKeepsOriginalBytesAndRejectsTraversal(t *testing.T) {
	store, err := NewFileStore(filepath.Join(t.TempDir(), "attachments"))
	if err != nil {
		t.Fatal(err)
	}
	content := []byte("original attachment bytes")
	digest := sha256.Sum256(content)
	request := attachmentdomain.PutRequest{
		WorkspaceID: "workspace-1", ObjectKey: "workspace/feature/original.txt", ContentType: "text/plain",
		Content: content, ContentSHA256: hex.EncodeToString(digest[:]), IdempotencyKey: "upload-1",
	}
	stored, err := store.Put(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	got, err := store.Get(context.Background(), "workspace-1", stored.ContentRef)
	if err != nil || !bytes.Equal(got, content) {
		t.Fatalf("read original bytes: got=%q err=%v", got, err)
	}
	_, err = store.Get(context.Background(), "workspace-1", "../outside")
	if !errors.Is(err, attachmentdomain.ErrContentNotFound) {
		t.Fatalf("traversal lookup error = %v", err)
	}
}
