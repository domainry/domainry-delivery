package identityhost

import (
	"strings"
	"testing"

	"github.com/domainry/domainry-foundation/schemaownership"
	"github.com/domainry/domainry-orm/dialect"
)

func TestWorkspaceTableDoesNotRepeatDatabaseName(t *testing.T) {
	if workspaceTable != "workspaces" {
		t.Fatalf("workspace table = %q, want workspaces", workspaceTable)
	}
	if strings.HasPrefix(workspaceTable, "delivery_") {
		t.Fatalf("workspace table %q repeats the standalone database name", workspaceTable)
	}
}

func TestWorkspaceOwnershipMatchesPhysicalSchema(t *testing.T) {
	if err := schemaownership.ValidateAll(SchemaOwnership()); err != nil {
		t.Fatal(err)
	}
	renderer, err := dialect.ParseRenderer("mysql", "", "")
	if err != nil {
		t.Fatal(err)
	}
	statement, arguments, err := workspaceTableDDL(renderer)
	if err != nil || len(arguments) != 0 {
		t.Fatalf("workspace DDL args=%v err=%v", arguments, err)
	}
	upper := strings.ToUpper(statement)
	if !strings.Contains(upper, "`ID` BIGINT NOT NULL AUTO_INCREMENT") || !strings.Contains(upper, "PRIMARY KEY (`ID`)") || !strings.Contains(upper, "UNIQUE (`WORKSPACE_ID`)") {
		t.Fatalf("workspace physical schema does not match ownership: %s", statement)
	}
}
