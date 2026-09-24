package product

import (
	"encoding/json"

	"github.com/domainry/domainry-delivery/internal/domain"
	commanddomain "github.com/domainry/domainry-delivery/internal/domain/command"
)

type Actor = domain.Actor
type ActorKind = domain.ActorKind
type Command = domain.Command
type AvailableAction = domain.AvailableAction
type Error = domain.Error

var sha256ValuePattern = domain.SHA256Pattern

const (
	ActorHuman  = domain.ActorHuman
	ActorAgent  = domain.ActorAgent
	ActorSystem = domain.ActorSystem
)

func validateCommandActor(value Command, target commanddomain.Target) error {
	return commanddomain.ValidateActor(value, target)
}

func catalogAction(key, targetID string) AvailableAction {
	action, ok := commanddomain.Action(key, targetID)
	if !ok {
		panic("invalid Product command catalog action: " + key)
	}
	return action
}

func decode(payload json.RawMessage, target any) error { return domain.Decode(payload, target) }
func clone[T any](value T) T                           { return domain.Clone(value) }
func cleanStrings(values []string) []string            { return domain.CleanStrings(values) }
func validGitRevision(value string) bool               { return domain.ValidGitRevision(value) }
func Invalid(code, message string) error               { return domain.Invalid(code, message) }
func NotFound(entity, id string) error                 { return domain.NotFound(entity, id) }

const CommandTargetProduct = commanddomain.TargetProduct
