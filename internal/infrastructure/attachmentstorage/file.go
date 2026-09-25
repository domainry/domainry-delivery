package attachmentstorage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	attachmentdomain "github.com/domainry/domainry-delivery/internal/domain/attachment"
)

type FileStore struct {
	root string
}

func NewFileStore(root string) (*FileStore, error) {
	if !filepath.IsAbs(root) {
		return nil, errors.New("Delivery attachment file store requires an absolute path")
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		return nil, fmt.Errorf("create Delivery attachment directory: %w", err)
	}
	return &FileStore{root: root}, nil
}

func (store *FileStore) Put(_ context.Context, request attachmentdomain.PutRequest) (attachmentdomain.PutResult, error) {
	if !validObjectKey(request.ObjectKey) || len(request.Content) == 0 || int64(len(request.Content)) > attachmentdomain.MaxUploadBytes {
		return attachmentdomain.PutResult{}, errors.New("attachment object key or content is invalid")
	}
	digest := sha256.Sum256(request.Content)
	sha := hex.EncodeToString(digest[:])
	if sha != request.ContentSHA256 {
		return attachmentdomain.PutResult{}, errors.New("attachment content hash does not match")
	}
	ref := defaultPrefix + "/" + request.ObjectKey
	filename := filepath.Join(store.root, filepath.FromSlash(ref))
	if err := os.MkdirAll(filepath.Dir(filename), 0700); err != nil {
		return attachmentdomain.PutResult{}, fmt.Errorf("create attachment object directory: %w", err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(filename), ".attachment-*")
	if err != nil {
		return attachmentdomain.PutResult{}, fmt.Errorf("prepare attachment object: %w", err)
	}
	defer os.Remove(temporary.Name())
	if err := temporary.Chmod(0600); err != nil {
		_ = temporary.Close()
		return attachmentdomain.PutResult{}, fmt.Errorf("protect attachment object: %w", err)
	}
	if _, err := temporary.Write(request.Content); err != nil {
		_ = temporary.Close()
		return attachmentdomain.PutResult{}, fmt.Errorf("write attachment object: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return attachmentdomain.PutResult{}, fmt.Errorf("sync attachment object: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return attachmentdomain.PutResult{}, fmt.Errorf("close attachment object: %w", err)
	}
	if err := os.Rename(temporary.Name(), filename); err != nil {
		return attachmentdomain.PutResult{}, fmt.Errorf("commit attachment object: %w", err)
	}
	return attachmentdomain.PutResult{ContentRef: ref, ETag: sha}, nil
}

func (store *FileStore) Get(_ context.Context, _ string, contentRef string) ([]byte, error) {
	if !strings.HasPrefix(contentRef, defaultPrefix+"/") || !validObjectKey(contentRef) {
		return nil, attachmentdomain.ErrContentNotFound
	}
	filename := filepath.Join(store.root, filepath.FromSlash(contentRef))
	file, err := os.Open(filename)
	if errors.Is(err, os.ErrNotExist) {
		return nil, attachmentdomain.ErrContentNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("open attachment object: %w", err)
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, attachmentdomain.MaxUploadBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read attachment object: %w", err)
	}
	if int64(len(content)) > attachmentdomain.MaxUploadBytes {
		return nil, errors.New("stored attachment exceeds maximum size")
	}
	return content, nil
}

func validObjectKey(key string) bool {
	return key != "" && !strings.HasPrefix(key, "/") && !strings.HasPrefix(key, "..") && !strings.Contains(key, "\\") && path.Clean(key) == key
}

var _ attachmentdomain.Store = (*FileStore)(nil)
