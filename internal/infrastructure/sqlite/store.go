package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/domainry/domainry-delivery/internal/domain/delivery"
	_ "modernc.org/sqlite"
)

type Store struct {
	db           *sql.DB
	ownsDatabase bool
}

func Open(path string) (*Store, error) {
	if path == "" {
		return nil, fmt.Errorf("sqlite path is required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	for _, statement := range []string{
		"PRAGMA journal_mode = WAL",
		"PRAGMA foreign_keys = ON",
		"PRAGMA busy_timeout = 5000",
	} {
		if _, err := db.Exec(statement); err != nil {
			_ = db.Close()
			return nil, err
		}
	}
	store := &Store{db: db, ownsDatabase: true}
	if err := store.migrate(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

// OpenBorrowed attaches Delivery to a host-owned SQLite pool. Delivery applies
// only its own schema and never closes or reconfigures the borrowed pool.
func OpenBorrowed(ctx context.Context, db *sql.DB) (*Store, error) {
	if ctx == nil || db == nil {
		return nil, fmt.Errorf("a context and host database are required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	store := &Store{db: db}
	if err := store.migrate(ctx); err != nil {
		return nil, err
	}
	return store, nil
}

func (store *Store) Close() error {
	if !store.ownsDatabase {
		return nil
	}
	return store.db.Close()
}

func (store *Store) migrate(ctx context.Context) error {
	_, err := store.db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS delivery_runs (
  workspace_id TEXT NOT NULL,
  run_id TEXT NOT NULL,
  product_id TEXT NOT NULL,
  stage TEXT NOT NULL,
  revision INTEGER NOT NULL,
  state_json BLOB NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  PRIMARY KEY (workspace_id, run_id)
);
CREATE INDEX IF NOT EXISTS delivery_runs_product_stage
  ON delivery_runs (workspace_id, product_id, stage, updated_at);
CREATE TABLE IF NOT EXISTS delivery_command_receipts (
  workspace_id TEXT NOT NULL,
  run_id TEXT NOT NULL,
  client_id TEXT NOT NULL,
  command_hash TEXT NOT NULL,
  response_json BLOB NOT NULL,
  response_revision INTEGER NOT NULL,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (workspace_id, run_id, client_id),
  FOREIGN KEY (workspace_id, run_id)
    REFERENCES delivery_runs (workspace_id, run_id)
    ON DELETE CASCADE
);
CREATE TABLE IF NOT EXISTS delivery_products (
  workspace_id TEXT NOT NULL,
  product_id TEXT NOT NULL,
  code TEXT NOT NULL,
  status TEXT NOT NULL,
  revision INTEGER NOT NULL,
  state_json BLOB NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  PRIMARY KEY (workspace_id, product_id),
  UNIQUE (workspace_id, code)
);
CREATE INDEX IF NOT EXISTS delivery_products_updated
  ON delivery_products (workspace_id, updated_at DESC, product_id);
CREATE TABLE IF NOT EXISTS delivery_product_command_receipts (
  workspace_id TEXT NOT NULL,
  product_id TEXT NOT NULL,
  client_id TEXT NOT NULL,
  command_hash TEXT NOT NULL,
  response_json BLOB NOT NULL,
  response_revision INTEGER NOT NULL,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (workspace_id, product_id, client_id),
  FOREIGN KEY (workspace_id, product_id)
    REFERENCES delivery_products (workspace_id, product_id)
    ON DELETE CASCADE
);
CREATE TABLE IF NOT EXISTS delivery_feature_attachments (
  workspace_id TEXT NOT NULL,
  product_id TEXT NOT NULL,
  feature_id TEXT NOT NULL,
  attachment_id TEXT NOT NULL,
  content_sha256 TEXT NOT NULL,
  metadata_json BLOB NOT NULL,
  content BLOB NOT NULL,
  created_at TEXT NOT NULL,
  PRIMARY KEY (workspace_id, product_id, feature_id, attachment_id),
  FOREIGN KEY (workspace_id, product_id)
    REFERENCES delivery_products (workspace_id, product_id)
    ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS delivery_feature_attachments_hash
  ON delivery_feature_attachments (workspace_id, content_sha256);
`)
	return err
}

func (store *Store) Ensure(ctx context.Context, run delivery.DeliveryRun) error {
	data, err := json.Marshal(run)
	if err != nil {
		return storageError(err)
	}
	_, err = store.db.ExecContext(ctx, `
INSERT INTO delivery_runs (
  workspace_id, run_id, product_id, stage, revision, state_json, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (workspace_id, run_id) DO NOTHING
`, run.WorkspaceID, run.ID, run.Product.ID, run.Stage, run.Revision, data, run.CreatedAt.Format(timeFormat), run.UpdatedAt.Format(timeFormat))
	if err != nil {
		return storageError(err)
	}
	return nil
}

func (store *Store) Get(ctx context.Context, workspaceID, deliveryRunID string) (delivery.DeliveryRun, error) {
	var data []byte
	err := store.db.QueryRowContext(ctx, `
SELECT state_json
FROM delivery_runs
WHERE workspace_id = ? AND run_id = ?
	`, workspaceID, deliveryRunID).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return delivery.DeliveryRun{}, delivery.NotFound("delivery run", deliveryRunID)
	}
	if err != nil {
		return delivery.DeliveryRun{}, storageError(err)
	}
	var run delivery.DeliveryRun
	if err := json.Unmarshal(data, &run); err != nil {
		return delivery.DeliveryRun{}, storageError(err)
	}
	return run, nil
}

func (store *Store) ListDeliveryRuns(ctx context.Context, workspaceID, productID string) ([]delivery.DeliveryRun, error) {
	query := `
SELECT state_json
FROM delivery_runs
WHERE workspace_id = ?`
	arguments := []any{workspaceID}
	if productID != "" {
		query += " AND product_id = ?"
		arguments = append(arguments, productID)
	}
	query += " ORDER BY updated_at DESC, run_id"
	rows, err := store.db.QueryContext(ctx, query, arguments...)
	if err != nil {
		return nil, storageError(err)
	}
	defer func() { _ = rows.Close() }()
	runs := make([]delivery.DeliveryRun, 0)
	for rows.Next() {
		var stateJSON []byte
		if err := rows.Scan(&stateJSON); err != nil {
			return nil, storageError(err)
		}
		var run delivery.DeliveryRun
		if err := json.Unmarshal(stateJSON, &run); err != nil {
			return nil, storageError(err)
		}
		runs = append(runs, run)
	}
	if err := rows.Err(); err != nil {
		return nil, storageError(err)
	}
	return runs, nil
}

func (store *Store) Transact(
	ctx context.Context,
	workspaceID string,
	deliveryRunID string,
	clientID string,
	commandHash string,
	expectedRevision uint64,
	mutate func(*delivery.DeliveryRun) error,
	install func(*delivery.Product, *delivery.DeliveryRun) error,
) (delivery.DeliveryRun, error) {
	transaction, err := store.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return delivery.DeliveryRun{}, storageError(err)
	}
	defer func() { _ = transaction.Rollback() }()

	var savedHash string
	var savedResponse []byte
	err = transaction.QueryRowContext(ctx, `
SELECT command_hash, response_json
FROM delivery_command_receipts
WHERE workspace_id = ? AND run_id = ? AND client_id = ?
`, workspaceID, deliveryRunID, clientID).Scan(&savedHash, &savedResponse)
	if err == nil {
		if savedHash != commandHash {
			return delivery.DeliveryRun{}, delivery.Invalid("idempotency_key_reused", "The same client_id cannot be used for different commands.")
		}
		var saved delivery.DeliveryRun
		if err := json.Unmarshal(savedResponse, &saved); err != nil {
			return delivery.DeliveryRun{}, storageError(err)
		}
		if err := transaction.Commit(); err != nil {
			return delivery.DeliveryRun{}, storageError(err)
		}
		return saved, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return delivery.DeliveryRun{}, storageError(err)
	}

	var stateJSON []byte
	var actualRevision uint64
	err = transaction.QueryRowContext(ctx, `
SELECT state_json, revision
FROM delivery_runs
WHERE workspace_id = ? AND run_id = ?
	`, workspaceID, deliveryRunID).Scan(&stateJSON, &actualRevision)
	if errors.Is(err, sql.ErrNoRows) {
		return delivery.DeliveryRun{}, delivery.NotFound("delivery run", deliveryRunID)
	}
	if err != nil {
		return delivery.DeliveryRun{}, storageError(err)
	}
	if expectedRevision != actualRevision {
		return delivery.DeliveryRun{}, delivery.Conflict(actualRevision)
	}
	var run delivery.DeliveryRun
	if err := json.Unmarshal(stateJSON, &run); err != nil {
		return delivery.DeliveryRun{}, storageError(err)
	}
	if err := mutate(&run); err != nil {
		return delivery.DeliveryRun{}, err
	}
	if run.Stage == delivery.StageLive && install != nil {
		product, productRevision, err := loadProduct(ctx, transaction, workspaceID, run.Product.ID)
		if err != nil {
			return delivery.DeliveryRun{}, err
		}
		if err := install(&product, &run); err != nil {
			return delivery.DeliveryRun{}, err
		}
		product.Revision = productRevision + 1
		if err := updateProduct(ctx, transaction, product, productRevision); err != nil {
			return delivery.DeliveryRun{}, err
		}
	}
	run.Revision = actualRevision + 1
	responseJSON, err := json.Marshal(run)
	if err != nil {
		return delivery.DeliveryRun{}, storageError(err)
	}
	result, err := transaction.ExecContext(ctx, `
UPDATE delivery_runs
SET product_id = ?, stage = ?, revision = ?, state_json = ?, updated_at = ?
WHERE workspace_id = ? AND run_id = ? AND revision = ?
`, run.Product.ID, run.Stage, run.Revision, responseJSON, run.UpdatedAt.Format(timeFormat), workspaceID, deliveryRunID, actualRevision)
	if err != nil {
		return delivery.DeliveryRun{}, storageError(err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return delivery.DeliveryRun{}, storageError(err)
	}
	if changed != 1 {
		return delivery.DeliveryRun{}, delivery.Conflict(actualRevision)
	}
	if _, err := transaction.ExecContext(ctx, `
INSERT INTO delivery_command_receipts (
  workspace_id, run_id, client_id, command_hash, response_json, response_revision
) VALUES (?, ?, ?, ?, ?, ?)
`, workspaceID, deliveryRunID, clientID, commandHash, responseJSON, run.Revision); err != nil {
		return delivery.DeliveryRun{}, storageError(err)
	}
	if err := transaction.Commit(); err != nil {
		return delivery.DeliveryRun{}, storageError(err)
	}
	return run, nil
}

func (store *Store) StartDelivery(
	ctx context.Context,
	workspaceID string,
	productID string,
	deliveryRunID string,
	clientID string,
	commandHash string,
	expectedRevision uint64,
	create func(*delivery.Product) (delivery.DeliveryRun, error),
) (delivery.Product, delivery.DeliveryRun, error) {
	transaction, err := store.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return delivery.Product{}, delivery.DeliveryRun{}, storageError(err)
	}
	defer func() { _ = transaction.Rollback() }()

	if saved, found, err := savedProductReceipt(ctx, transaction, workspaceID, productID, clientID, commandHash); err != nil {
		return delivery.Product{}, delivery.DeliveryRun{}, err
	} else if found {
		run, err := loadDeliveryRun(ctx, transaction, workspaceID, deliveryRunID)
		if err != nil {
			return delivery.Product{}, delivery.DeliveryRun{}, err
		}
		if err := transaction.Commit(); err != nil {
			return delivery.Product{}, delivery.DeliveryRun{}, storageError(err)
		}
		return saved, run, nil
	}

	product, actualRevision, err := loadProduct(ctx, transaction, workspaceID, productID)
	if err != nil {
		return delivery.Product{}, delivery.DeliveryRun{}, err
	}
	if expectedRevision != actualRevision {
		return delivery.Product{}, delivery.DeliveryRun{}, delivery.Conflict(actualRevision)
	}
	var existingRunID string
	err = transaction.QueryRowContext(ctx, `
SELECT run_id FROM delivery_runs WHERE workspace_id = ? AND run_id = ?
`, workspaceID, deliveryRunID).Scan(&existingRunID)
	if err == nil {
		return delivery.Product{}, delivery.DeliveryRun{}, delivery.Invalid("delivery_run_exists", "The delivery run already exists.")
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return delivery.Product{}, delivery.DeliveryRun{}, storageError(err)
	}

	run, err := create(&product)
	if err != nil {
		return delivery.Product{}, delivery.DeliveryRun{}, err
	}
	if run.ID != deliveryRunID || run.WorkspaceID != workspaceID || run.Product.ID != productID {
		return delivery.Product{}, delivery.DeliveryRun{}, delivery.Invalid("delivery_run_identity_invalid", "The created delivery run does not match the requested resource identity.")
	}
	product.Revision = actualRevision + 1
	productJSON, err := json.Marshal(product)
	if err != nil {
		return delivery.Product{}, delivery.DeliveryRun{}, storageError(err)
	}
	runJSON, err := json.Marshal(run)
	if err != nil {
		return delivery.Product{}, delivery.DeliveryRun{}, storageError(err)
	}
	if _, err := transaction.ExecContext(ctx, `
INSERT INTO delivery_runs (
  workspace_id, run_id, product_id, stage, revision, state_json, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
`, run.WorkspaceID, run.ID, run.Product.ID, run.Stage, run.Revision, runJSON, run.CreatedAt.Format(timeFormat), run.UpdatedAt.Format(timeFormat)); err != nil {
		return delivery.Product{}, delivery.DeliveryRun{}, storageError(err)
	}
	if err := updateProductWithJSON(ctx, transaction, product, actualRevision, productJSON); err != nil {
		return delivery.Product{}, delivery.DeliveryRun{}, err
	}
	if err := insertProductReceipt(ctx, transaction, product, clientID, commandHash, productJSON); err != nil {
		return delivery.Product{}, delivery.DeliveryRun{}, err
	}
	if err := transaction.Commit(); err != nil {
		return delivery.Product{}, delivery.DeliveryRun{}, storageError(err)
	}
	return product, run, nil
}

func (store *Store) EnsureProduct(ctx context.Context, product delivery.Product) error {
	data, err := json.Marshal(product)
	if err != nil {
		return storageError(err)
	}
	_, err = store.db.ExecContext(ctx, `
INSERT INTO delivery_products (
  workspace_id, product_id, code, status, revision, state_json, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (workspace_id, product_id) DO NOTHING
`, product.WorkspaceID, product.ID, product.Code, product.Status, product.Revision, data, product.CreatedAt.Format(timeFormat), product.UpdatedAt.Format(timeFormat))
	if err != nil {
		return storageError(err)
	}
	return nil
}

func (store *Store) GetProduct(ctx context.Context, workspaceID, productID string) (delivery.Product, error) {
	var data []byte
	err := store.db.QueryRowContext(ctx, `
SELECT state_json
FROM delivery_products
WHERE workspace_id = ? AND product_id = ?
	`, workspaceID, productID).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return delivery.Product{}, delivery.NotFound("product", productID)
	}
	if err != nil {
		return delivery.Product{}, storageError(err)
	}
	var product delivery.Product
	if err := json.Unmarshal(data, &product); err != nil {
		return delivery.Product{}, storageError(err)
	}
	return product, nil
}

func (store *Store) ListProducts(ctx context.Context, workspaceID string) ([]delivery.Product, error) {
	rows, err := store.db.QueryContext(ctx, `
SELECT state_json
FROM delivery_products
WHERE workspace_id = ?
ORDER BY updated_at DESC, product_id
`, workspaceID)
	if err != nil {
		return nil, storageError(err)
	}
	defer func() { _ = rows.Close() }()
	products := make([]delivery.Product, 0)
	for rows.Next() {
		var data []byte
		if err := rows.Scan(&data); err != nil {
			return nil, storageError(err)
		}
		var product delivery.Product
		if err := json.Unmarshal(data, &product); err != nil {
			return nil, storageError(err)
		}
		products = append(products, product)
	}
	if err := rows.Err(); err != nil {
		return nil, storageError(err)
	}
	return products, nil
}

func (store *Store) CreateProduct(
	ctx context.Context,
	workspaceID string,
	productID string,
	clientID string,
	commandHash string,
	expectedRevision uint64,
	create func() (delivery.Product, error),
) (delivery.Product, error) {
	transaction, err := store.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return delivery.Product{}, storageError(err)
	}
	defer func() { _ = transaction.Rollback() }()

	if saved, found, err := savedProductReceipt(ctx, transaction, workspaceID, productID, clientID, commandHash); err != nil {
		return delivery.Product{}, err
	} else if found {
		if err := transaction.Commit(); err != nil {
			return delivery.Product{}, storageError(err)
		}
		return saved, nil
	}

	var actualRevision uint64
	err = transaction.QueryRowContext(ctx, `
SELECT revision FROM delivery_products WHERE workspace_id = ? AND product_id = ?
`, workspaceID, productID).Scan(&actualRevision)
	if err == nil {
		return delivery.Product{}, delivery.Conflict(actualRevision)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return delivery.Product{}, storageError(err)
	}
	if expectedRevision != 0 {
		return delivery.Product{}, delivery.Conflict(0)
	}
	product, err := create()
	if err != nil {
		return delivery.Product{}, err
	}
	var existingProductID string
	err = transaction.QueryRowContext(ctx, `
SELECT product_id FROM delivery_products WHERE workspace_id = ? AND code = ?
	`, workspaceID, product.Code).Scan(&existingProductID)
	if err == nil {
		return delivery.Product{}, delivery.Invalid("product_code_duplicate", "A Product code must be unique within a Workspace.")
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return delivery.Product{}, storageError(err)
	}
	responseJSON, err := json.Marshal(product)
	if err != nil {
		return delivery.Product{}, storageError(err)
	}
	if _, err := transaction.ExecContext(ctx, `
INSERT INTO delivery_products (
  workspace_id, product_id, code, status, revision, state_json, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
`, product.WorkspaceID, product.ID, product.Code, product.Status, product.Revision, responseJSON, product.CreatedAt.Format(timeFormat), product.UpdatedAt.Format(timeFormat)); err != nil {
		return delivery.Product{}, storageError(err)
	}
	if err := insertProductReceipt(ctx, transaction, product, clientID, commandHash, responseJSON); err != nil {
		return delivery.Product{}, err
	}
	if err := transaction.Commit(); err != nil {
		return delivery.Product{}, storageError(err)
	}
	return product, nil
}

func (store *Store) TransactProduct(
	ctx context.Context,
	workspaceID string,
	productID string,
	clientID string,
	commandHash string,
	expectedRevision uint64,
	mutate func(*delivery.Product) error,
) (delivery.Product, error) {
	transaction, err := store.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return delivery.Product{}, storageError(err)
	}
	defer func() { _ = transaction.Rollback() }()

	if saved, found, err := savedProductReceipt(ctx, transaction, workspaceID, productID, clientID, commandHash); err != nil {
		return delivery.Product{}, err
	} else if found {
		if err := transaction.Commit(); err != nil {
			return delivery.Product{}, storageError(err)
		}
		return saved, nil
	}

	var stateJSON []byte
	var actualRevision uint64
	err = transaction.QueryRowContext(ctx, `
SELECT state_json, revision
FROM delivery_products
WHERE workspace_id = ? AND product_id = ?
	`, workspaceID, productID).Scan(&stateJSON, &actualRevision)
	if errors.Is(err, sql.ErrNoRows) {
		return delivery.Product{}, delivery.NotFound("product", productID)
	}
	if err != nil {
		return delivery.Product{}, storageError(err)
	}
	if expectedRevision != actualRevision {
		return delivery.Product{}, delivery.Conflict(actualRevision)
	}
	var product delivery.Product
	if err := json.Unmarshal(stateJSON, &product); err != nil {
		return delivery.Product{}, storageError(err)
	}
	if err := mutate(&product); err != nil {
		return delivery.Product{}, err
	}
	product.Revision = actualRevision + 1
	responseJSON, err := json.Marshal(product)
	if err != nil {
		return delivery.Product{}, storageError(err)
	}
	result, err := transaction.ExecContext(ctx, `
UPDATE delivery_products
SET code = ?, status = ?, revision = ?, state_json = ?, updated_at = ?
WHERE workspace_id = ? AND product_id = ? AND revision = ?
`, product.Code, product.Status, product.Revision, responseJSON, product.UpdatedAt.Format(timeFormat), workspaceID, productID, actualRevision)
	if err != nil {
		return delivery.Product{}, storageError(err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return delivery.Product{}, storageError(err)
	}
	if changed != 1 {
		return delivery.Product{}, delivery.Conflict(actualRevision)
	}
	if err := insertProductReceipt(ctx, transaction, product, clientID, commandHash, responseJSON); err != nil {
		return delivery.Product{}, err
	}
	if err := transaction.Commit(); err != nil {
		return delivery.Product{}, storageError(err)
	}
	return product, nil
}

func (store *Store) StoreFeatureAttachment(
	ctx context.Context,
	workspaceID string,
	productID string,
	featureID string,
	attachmentID string,
	expectedRevision uint64,
	contentHash string,
	content []byte,
	mutate func(*delivery.Product) (delivery.FeatureAttachment, error),
) (delivery.Product, delivery.FeatureAttachment, error) {
	transaction, err := store.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return delivery.Product{}, delivery.FeatureAttachment{}, storageError(err)
	}
	defer func() { _ = transaction.Rollback() }()

	var savedHash string
	var metadataJSON []byte
	err = transaction.QueryRowContext(ctx, `
SELECT content_sha256, metadata_json
FROM delivery_feature_attachments
WHERE workspace_id = ? AND product_id = ? AND feature_id = ? AND attachment_id = ?
`, workspaceID, productID, featureID, attachmentID).Scan(&savedHash, &metadataJSON)
	if err == nil {
		if savedHash != contentHash {
			return delivery.Product{}, delivery.FeatureAttachment{}, delivery.Invalid("attachment_id_reused", "An attachment ID cannot identify different content.")
		}
		product, _, err := loadProduct(ctx, transaction, workspaceID, productID)
		if err != nil {
			return delivery.Product{}, delivery.FeatureAttachment{}, err
		}
		var attachment delivery.FeatureAttachment
		if err := json.Unmarshal(metadataJSON, &attachment); err != nil {
			return delivery.Product{}, delivery.FeatureAttachment{}, storageError(err)
		}
		if err := transaction.Commit(); err != nil {
			return delivery.Product{}, delivery.FeatureAttachment{}, storageError(err)
		}
		return product, attachment, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return delivery.Product{}, delivery.FeatureAttachment{}, storageError(err)
	}

	product, actualRevision, err := loadProduct(ctx, transaction, workspaceID, productID)
	if err != nil {
		return delivery.Product{}, delivery.FeatureAttachment{}, err
	}
	if expectedRevision != actualRevision {
		return delivery.Product{}, delivery.FeatureAttachment{}, delivery.Conflict(actualRevision)
	}
	attachment, err := mutate(&product)
	if err != nil {
		return delivery.Product{}, delivery.FeatureAttachment{}, err
	}
	product.Revision = actualRevision + 1
	if err := updateProduct(ctx, transaction, product, actualRevision); err != nil {
		return delivery.Product{}, delivery.FeatureAttachment{}, err
	}
	metadataJSON, err = json.Marshal(attachment)
	if err != nil {
		return delivery.Product{}, delivery.FeatureAttachment{}, storageError(err)
	}
	if _, err := transaction.ExecContext(ctx, `
INSERT INTO delivery_feature_attachments (
  workspace_id, product_id, feature_id, attachment_id, content_sha256, metadata_json, content, created_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
`, workspaceID, productID, featureID, attachmentID, contentHash, metadataJSON, content, attachment.CreatedAt.Format(timeFormat)); err != nil {
		return delivery.Product{}, delivery.FeatureAttachment{}, storageError(err)
	}
	if err := transaction.Commit(); err != nil {
		return delivery.Product{}, delivery.FeatureAttachment{}, storageError(err)
	}
	return product, attachment, nil
}

func (store *Store) GetFeatureAttachment(ctx context.Context, workspaceID, productID, featureID, attachmentID string) (delivery.FeatureAttachmentContent, error) {
	var metadataJSON []byte
	var content []byte
	err := store.db.QueryRowContext(ctx, `
SELECT metadata_json, content
FROM delivery_feature_attachments
WHERE workspace_id = ? AND product_id = ? AND feature_id = ? AND attachment_id = ?
`, workspaceID, productID, featureID, attachmentID).Scan(&metadataJSON, &content)
	if errors.Is(err, sql.ErrNoRows) {
		return delivery.FeatureAttachmentContent{}, delivery.NotFound("Feature attachment", attachmentID)
	}
	if err != nil {
		return delivery.FeatureAttachmentContent{}, storageError(err)
	}
	var attachment delivery.FeatureAttachment
	if err := json.Unmarshal(metadataJSON, &attachment); err != nil {
		return delivery.FeatureAttachmentContent{}, storageError(err)
	}
	return delivery.FeatureAttachmentContent{Attachment: attachment, Content: content}, nil
}

func (store *Store) DeleteFeatureAttachment(
	ctx context.Context,
	workspaceID string,
	productID string,
	featureID string,
	attachmentID string,
	expectedRevision uint64,
	mutate func(*delivery.Product) (delivery.FeatureAttachment, error),
) (delivery.Product, error) {
	transaction, err := store.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return delivery.Product{}, storageError(err)
	}
	defer func() { _ = transaction.Rollback() }()
	product, actualRevision, err := loadProduct(ctx, transaction, workspaceID, productID)
	if err != nil {
		return delivery.Product{}, err
	}
	if expectedRevision != actualRevision {
		return delivery.Product{}, delivery.Conflict(actualRevision)
	}
	if _, err := mutate(&product); err != nil {
		return delivery.Product{}, err
	}
	product.Revision = actualRevision + 1
	if err := updateProduct(ctx, transaction, product, actualRevision); err != nil {
		return delivery.Product{}, err
	}
	result, err := transaction.ExecContext(ctx, `
DELETE FROM delivery_feature_attachments
WHERE workspace_id = ? AND product_id = ? AND feature_id = ? AND attachment_id = ?
`, workspaceID, productID, featureID, attachmentID)
	if err != nil {
		return delivery.Product{}, storageError(err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return delivery.Product{}, storageError(err)
	}
	if changed != 1 {
		return delivery.Product{}, delivery.NotFound("Feature attachment", attachmentID)
	}
	if err := transaction.Commit(); err != nil {
		return delivery.Product{}, storageError(err)
	}
	return product, nil
}

func savedProductReceipt(
	ctx context.Context,
	transaction *sql.Tx,
	workspaceID string,
	productID string,
	clientID string,
	commandHash string,
) (delivery.Product, bool, error) {
	var savedHash string
	var savedResponse []byte
	err := transaction.QueryRowContext(ctx, `
SELECT command_hash, response_json
FROM delivery_product_command_receipts
WHERE workspace_id = ? AND product_id = ? AND client_id = ?
`, workspaceID, productID, clientID).Scan(&savedHash, &savedResponse)
	if errors.Is(err, sql.ErrNoRows) {
		return delivery.Product{}, false, nil
	}
	if err != nil {
		return delivery.Product{}, false, storageError(err)
	}
	if savedHash != commandHash {
		return delivery.Product{}, false, delivery.Invalid("idempotency_key_reused", "The same client_id cannot be used for different commands.")
	}
	var saved delivery.Product
	if err := json.Unmarshal(savedResponse, &saved); err != nil {
		return delivery.Product{}, false, storageError(err)
	}
	return saved, true, nil
}

func insertProductReceipt(
	ctx context.Context,
	transaction *sql.Tx,
	product delivery.Product,
	clientID string,
	commandHash string,
	responseJSON []byte,
) error {
	_, err := transaction.ExecContext(ctx, `
INSERT INTO delivery_product_command_receipts (
  workspace_id, product_id, client_id, command_hash, response_json, response_revision
) VALUES (?, ?, ?, ?, ?, ?)
`, product.WorkspaceID, product.ID, clientID, commandHash, responseJSON, product.Revision)
	if err != nil {
		return storageError(err)
	}
	return nil
}

func loadDeliveryRun(ctx context.Context, transaction *sql.Tx, workspaceID, deliveryRunID string) (delivery.DeliveryRun, error) {
	var stateJSON []byte
	err := transaction.QueryRowContext(ctx, `
SELECT state_json FROM delivery_runs WHERE workspace_id = ? AND run_id = ?
`, workspaceID, deliveryRunID).Scan(&stateJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return delivery.DeliveryRun{}, delivery.NotFound("delivery run", deliveryRunID)
	}
	if err != nil {
		return delivery.DeliveryRun{}, storageError(err)
	}
	var run delivery.DeliveryRun
	if err := json.Unmarshal(stateJSON, &run); err != nil {
		return delivery.DeliveryRun{}, storageError(err)
	}
	return run, nil
}

func loadProduct(ctx context.Context, transaction *sql.Tx, workspaceID, productID string) (delivery.Product, uint64, error) {
	var stateJSON []byte
	var revision uint64
	err := transaction.QueryRowContext(ctx, `
SELECT state_json, revision FROM delivery_products WHERE workspace_id = ? AND product_id = ?
`, workspaceID, productID).Scan(&stateJSON, &revision)
	if errors.Is(err, sql.ErrNoRows) {
		return delivery.Product{}, 0, delivery.NotFound("product", productID)
	}
	if err != nil {
		return delivery.Product{}, 0, storageError(err)
	}
	var product delivery.Product
	if err := json.Unmarshal(stateJSON, &product); err != nil {
		return delivery.Product{}, 0, storageError(err)
	}
	return product, revision, nil
}

func updateProduct(ctx context.Context, transaction *sql.Tx, product delivery.Product, expectedRevision uint64) error {
	stateJSON, err := json.Marshal(product)
	if err != nil {
		return storageError(err)
	}
	return updateProductWithJSON(ctx, transaction, product, expectedRevision, stateJSON)
}

func updateProductWithJSON(ctx context.Context, transaction *sql.Tx, product delivery.Product, expectedRevision uint64, stateJSON []byte) error {
	result, err := transaction.ExecContext(ctx, `
UPDATE delivery_products
SET code = ?, status = ?, revision = ?, state_json = ?, updated_at = ?
WHERE workspace_id = ? AND product_id = ? AND revision = ?
`, product.Code, product.Status, product.Revision, stateJSON, product.UpdatedAt.Format(timeFormat), product.WorkspaceID, product.ID, expectedRevision)
	if err != nil {
		return storageError(err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return storageError(err)
	}
	if changed != 1 {
		return delivery.Conflict(expectedRevision)
	}
	return nil
}

const timeFormat = "2006-01-02T15:04:05.999999999Z07:00"

func storageError(err error) error {
	return &delivery.Error{Code: "storage_failure", Message: "Delivery data storage failed.", Details: map[string]any{"cause": err.Error()}}
}
