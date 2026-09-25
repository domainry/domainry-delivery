package conversation

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

const MaxMessageBytes = 64 << 10

type Message struct {
	ID             string   `json:"id"`
	ConversationID string   `json:"conversation_id"`
	TurnID         string   `json:"turn_id"`
	Role           string   `json:"role"`
	Text           string   `json:"text"`
	AttachmentIDs  []string `json:"attachment_ids"`
	SourceID       string   `json:"source_id"`
	CreatedBy      string   `json:"created_by"`
	DeviceID       string   `json:"device_id"`
	CreatedAt      int64    `json:"created_at"`
}

// ConversationID is stable for one Workspace/Product/Feature scope. It does
// not disclose tenant identifiers in cross-service source references.
func ConversationID(workspaceID, productID, featureID string) string {
	digest := sha256.Sum256([]byte(workspaceID + "\x00" + productID + "\x00" + featureID))
	return "conversation-" + hex.EncodeToString(digest[:16])
}

func SourceID(conversationID, messageID string) string {
	return fmt.Sprintf("conversation://%s/message/%s", conversationID, messageID)
}

func ValidID(value string) bool {
	if value == "" || len(value) > 191 || strings.TrimSpace(value) != value {
		return false
	}
	for _, character := range value {
		if !((character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') || (character >= '0' && character <= '9') || character == '-' || character == '_') {
			return false
		}
	}
	return true
}
