package database

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	sharedoperation "github.com/domainry/domainry-foundation/operation"
	"github.com/domainry/domainry-orm/sqlhost"

	"github.com/domainry/domainry-delivery/internal/application"
	"github.com/domainry/domainry-delivery/internal/domain"
)

const operationOwner = "delivery"

func (store *Store) claimOperation(ctx context.Context, transaction sqlhost.DBTX, workspaceID, resourceType, resourceID, kind string, mutation application.Mutation) (sharedoperation.Receipt, bool, error) {
	operationContext := sharedoperation.WithExecutor(ctx, transaction)
	scope := sharedoperation.Scope{WorkspaceID: workspaceID, ResourceType: resourceType, ResourceID: resourceID}
	identity := operationID(workspaceID, resourceType, resourceID, kind, mutation.ClientID)
	receipt, claimed, err := store.operations.Claim(operationContext, sharedoperation.Command{
		ID: identity, Scope: scope, Owner: operationOwner, Kind: kind, ActionKey: mutation.CommandType,
		IdempotencyKey: mutation.ClientID, RequestFingerprint: mutation.Fingerprint, RequestedBy: mutation.ActorID,
		Reason: "Apply Delivery command " + mutation.CommandType, Reference: resourceType + ":" + resourceID,
		StatusURL: "delivery://operations/" + identity, CreatedAt: mutation.OccurredAt,
	})
	if errors.Is(err, sharedoperation.ErrIdempotencyConflict) {
		return sharedoperation.Receipt{}, false, domain.Invalid("idempotency_key_reused")
	}
	if err != nil {
		return sharedoperation.Receipt{}, false, storageError(err)
	}
	if claimed {
		return receipt, false, nil
	}
	if receipt.Status != sharedoperation.StatusSucceeded || !json.Valid(receipt.Result) || string(receipt.Result) == "{}" {
		return sharedoperation.Receipt{}, false, domain.Invalid("operation_in_progress")
	}
	return receipt, true, nil
}

func (store *Store) completeOperation(ctx context.Context, transaction sqlhost.DBTX, receipt sharedoperation.Receipt, result json.RawMessage, completedAt time.Time) error {
	completion := sharedoperation.Completion{
		ID: receipt.Command.ID, Scope: receipt.Command.Scope, Owner: receipt.Command.Owner, Kind: receipt.Command.Kind,
		IdempotencyKey: receipt.Command.IdempotencyKey, RequestFingerprint: receipt.Command.RequestFingerprint,
		Result: append(json.RawMessage(nil), result...), CompletedAt: completedAt,
	}
	if err := store.operations.Complete(sharedoperation.WithExecutor(ctx, transaction), completion); err != nil {
		return storageError(err)
	}
	return nil
}

func operationID(parts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return "delivery-" + hex.EncodeToString(sum[:])
}
