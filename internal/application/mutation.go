package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/domainry/domainry-delivery/internal/domain"
	commanddomain "github.com/domainry/domainry-delivery/internal/domain/command"
)

// Mutation carries the immutable command identity required by the shared
// Foundation operation ledger. Resource identity is supplied separately by
// the repository method and is part of Fingerprint.
type Mutation struct {
	ClientID    string
	Fingerprint string
	CommandType string
	ActorID     string
	OccurredAt  time.Time
}

type commandScope struct {
	Target              commanddomain.Target
	WorkspaceID         string
	FingerprintIdentity []string
}

// executeMutation is the owner-neutral application entry point for every
// Delivery command. It freezes command identity and authorization before the
// aggregate-specific transaction begins. Pure command-specific validation may
// run through verify before commit; Delivery currently has no synchronous
// external owner read or post-commit outbound effect to dispatch.
func executeMutation[T any](
	ctx context.Context,
	service *Service,
	scope commandScope,
	command domain.Command,
	verify func(domain.Command) error,
	commit func(domain.Command, Mutation, time.Time) (T, error),
) (T, error) {
	var zero T
	if command.ClientID == "" {
		return zero, domain.Invalid("client_id_required")
	}
	definition, ok := commanddomain.DefinitionFor(command.Type)
	if !ok || definition.Target != scope.Target {
		return zero, domain.Invalid("command_unknown")
	}
	var forcedKind *domain.ActorKind
	if len(definition.AllowedActors) == 1 && definition.AllowedActors[0] == domain.ActorSystem {
		systemKind := domain.ActorSystem
		forcedKind = &systemKind
	}
	actor, err := authenticatedActor(ctx, scope.WorkspaceID, definition.Permission, forcedKind)
	if err != nil {
		return zero, err
	}
	command.Actor = actor
	if verify != nil {
		if err := verify(command); err != nil {
			return zero, err
		}
	}
	fingerprint, err := commandFingerprint(command, scope.FingerprintIdentity...)
	if err != nil {
		return zero, domain.Invalid("command_invalid")
	}
	now := service.now()
	return commit(command, mutation(command, fingerprint, now), now)
}

func mutation(command domain.Command, fingerprint string, occurredAt time.Time) Mutation {
	return Mutation{ClientID: command.ClientID, Fingerprint: fingerprint, CommandType: command.Type, ActorID: command.Actor.ID, OccurredAt: occurredAt}
}

func commandFingerprint(command domain.Command, resourceIdentity ...string) (string, error) {
	var payload any = map[string]any{}
	if len(command.Payload) > 0 {
		if err := json.Unmarshal(command.Payload, &payload); err != nil {
			return "", err
		}
	}
	canonical, err := json.Marshal(struct {
		Actor            domain.Actor `json:"actor"`
		Type             string       `json:"type"`
		Payload          any          `json:"payload"`
		ResourceIdentity []string     `json:"resource_identity"`
	}{Actor: command.Actor, Type: command.Type, Payload: payload, ResourceIdentity: resourceIdentity})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:]), nil
}
