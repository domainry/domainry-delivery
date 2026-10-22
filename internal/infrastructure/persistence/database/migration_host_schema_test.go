package database

import (
	"strings"
	"testing"

	"github.com/domainry/domainry-foundation/schemaownership"
	"github.com/domainry/domainry-orm/dialect"
)

func TestStandaloneMigrationLedgerOwnershipMatchesPhysicalSchema(t *testing.T) {
	if err := schemaownership.ValidateAll(StandaloneHostSchemaOwnership()); err != nil {
		t.Fatal(err)
	}
	renderer, err := dialect.ParseRenderer("mysql", "", "")
	if err != nil {
		t.Fatal(err)
	}
	statement, arguments, err := migrationLedgerDDL(renderer)
	if err != nil || len(arguments) != 0 {
		t.Fatalf("migration ledger DDL args=%v err=%v", arguments, err)
	}
	upper := strings.ToUpper(statement)
	if !strings.Contains(upper, "`ID` BIGINT NOT NULL AUTO_INCREMENT") || !strings.Contains(upper, "PRIMARY KEY (`ID`)") || !strings.Contains(upper, "UNIQUE (`OWNER`, `VERSION`)") {
		t.Fatalf("migration ledger physical schema does not match ownership: %s", statement)
	}
}
