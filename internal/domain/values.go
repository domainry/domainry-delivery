package domain

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"regexp"
	"strings"
)

var gitRevisionPattern = regexp.MustCompile(`^(git:)?[A-Za-z0-9][A-Za-z0-9._/@:+-]{6,255}$`)
var SHA256Pattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

func Decode(payload json.RawMessage, target any) error {
	if len(payload) == 0 {
		payload = json.RawMessage(`{}`)
	}
	decoder := json.NewDecoder(strings.NewReader(string(payload)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return Invalid("payload_invalid", "The command payload is invalid.")
	}
	return nil
}

func Clone[T any](value T) T {
	raw, _ := json.Marshal(value)
	var copied T
	_ = json.Unmarshal(raw, &copied)
	return copied
}

func CleanStrings(values []string) []string {
	result := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" && !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	return result
}

func ValidGitRevision(revision string) bool {
	revision = strings.TrimSpace(revision)
	return gitRevisionPattern.MatchString(revision) && !strings.Contains(revision, "..")
}

func NewID(prefix string) string {
	value := make([]byte, 8)
	if _, err := rand.Read(value); err != nil {
		panic(err)
	}
	return prefix + "-" + hex.EncodeToString(value)
}
