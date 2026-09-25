// Package attachment owns Delivery's immutable Feature attachment contract.
package attachment

import (
	"context"
	"errors"
	"mime"
	"path"
	"strings"
	"time"

	"github.com/gabriel-vasile/mimetype"
)

const (
	MaxUploadBytes int64 = 5 << 20
	MaxPerFeature        = 10
)

var (
	ErrStorageUnavailable = errors.New("attachment storage is unavailable")
	ErrContentNotFound    = errors.New("attachment content is not found")
)

var supportedDocumentMediaTypes = map[string]string{
	"application/gzip":                                                          ".gz",
	"application/msword":                                                        ".doc",
	"application/pdf":                                                           ".pdf",
	"application/vnd.ms-excel":                                                  ".xls",
	"application/vnd.ms-powerpoint":                                             ".ppt",
	"application/vnd.ms-visio.drawing.main+xml":                                 ".vsdx",
	"application/vnd.oasis.opendocument.graphics":                               ".odg",
	"application/vnd.oasis.opendocument.presentation":                           ".odp",
	"application/vnd.oasis.opendocument.spreadsheet":                            ".ods",
	"application/vnd.oasis.opendocument.text":                                   ".odt",
	"application/vnd.openxmlformats-officedocument.presentationml.presentation": ".pptx",
	"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":         ".xlsx",
	"application/vnd.openxmlformats-officedocument.wordprocessingml.document":   ".docx",
	"application/vnd.wordperfect":                                               ".wpd",
	"application/x-7z-compressed":                                               ".7z",
	"application/x-rar-compressed":                                              ".rar",
	"application/zip":                                                           ".zip",
	"text/csv":                                                                  ".csv",
	"text/plain":                                                                ".txt",
	"text/rtf":                                                                  ".rtf",
	"text/tab-separated-values":                                                 ".tsv",
}

func init() { mimetype.SetLimit(uint32(MaxUploadBytes)) }

// ClassifyContent trusts the bytes rather than the browser-supplied MIME type.
// Active formats such as SVG, HTML, JavaScript, and executables are excluded.
func ClassifyContent(content []byte) (string, string, bool) {
	detected := mimetype.Detect(content)
	contentType, _, err := mime.ParseMediaType(detected.String())
	if err != nil {
		return "", "", false
	}
	if strings.HasPrefix(contentType, "image/") && contentType != "image/svg+xml" && detected.Extension() != "" {
		return contentType, detected.Extension(), true
	}
	extension, ok := supportedDocumentMediaTypes[contentType]
	return contentType, extension, ok
}

// BuildObjectKey groups originals by UTC upload day without exposing the
// user-supplied filename or business identifiers in the public S3 key.
func BuildObjectKey(createdAt time.Time, attachmentID, contentSHA256, extension string) string {
	return path.Join(
		createdAt.UTC().Format("20060102"),
		attachmentID+"-"+strings.ToLower(contentSHA256)+extension,
	)
}

type PutRequest struct {
	WorkspaceID    string
	ObjectKey      string
	ContentType    string
	Content        []byte
	ContentSHA256  string
	IdempotencyKey string
}

type PutResult struct {
	ContentRef string
	ETag       string
}

type Store interface {
	Put(context.Context, PutRequest) (PutResult, error)
	Get(context.Context, string, string) ([]byte, error)
}
