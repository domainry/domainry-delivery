package identityhost

import (
	"strings"
	"testing"
)

func TestWorkspaceTableDoesNotRepeatDatabaseName(t *testing.T) {
	if workspaceTable != "workspaces" {
		t.Fatalf("workspace table = %q, want workspaces", workspaceTable)
	}
	if strings.HasPrefix(workspaceTable, "delivery_") {
		t.Fatalf("workspace table %q repeats the standalone database name", workspaceTable)
	}
}
