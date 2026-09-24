package domain

import "encoding/json"

type ActorKind string

const (
	ActorHuman  ActorKind = "human"
	ActorAgent  ActorKind = "agent"
	ActorSystem ActorKind = "system"
)

type Actor struct {
	ID   string    `json:"id"`
	Kind ActorKind `json:"kind"`
}

// Session exposes non-secret authenticated identity facts. Authorization
// remains server-side and is evaluated for every command.
type Session struct {
	WorkspaceID string   `json:"workspace_id"`
	Actor       Actor    `json:"actor"`
	Permissions []string `json:"permissions"`
}

type Command struct {
	ClientID         string          `json:"client_id"`
	ExpectedRevision uint64          `json:"expected_revision"`
	Actor            Actor           `json:"-"`
	Type             string          `json:"type"`
	Payload          json.RawMessage `json:"payload"`
}

type AvailableAction struct {
	Command   string    `json:"command"`
	TargetID  string    `json:"target_id,omitempty"`
	ActorKind ActorKind `json:"actor_kind"`
}
