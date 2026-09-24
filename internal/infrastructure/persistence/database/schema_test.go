package database

import (
	"strings"
	"testing"
)

func TestSchemaHasOnePortableSourceForSQLiteAndMySQL(t *testing.T) {
	for _, driver := range []string{"sqlite", "mysql"} {
		statements := SchemaStatements(driver)
		joined := strings.Join(statements, "\n")
		for _, table := range OwnedTables() {
			if !strings.Contains(joined, table) {
				t.Fatalf("%s schema is missing %s", driver, table)
			}
		}
		if strings.Contains(joined, "delivery_feature_attachments") {
			t.Fatalf("%s schema retained Agent-owned attachment bytes", driver)
		}
	}
	if !strings.Contains(strings.Join(SchemaStatements("mysql"), "\n"), "LONGBLOB") {
		t.Fatal("MySQL schema did not render bounded aggregate documents for MySQL")
	}
}

func TestSchemaMigrationsAreTheSamePhysicalBaseline(t *testing.T) {
	for _, driver := range []string{"sqlite", "mysql"} {
		migrations := Migrations(driver)
		if len(migrations) != 1 || migrations[0].Version != 1 || migrations[0].Baseline == nil || len(migrations[0].Baseline.Tables) != len(OwnedTables()) {
			t.Fatalf("%s migration baseline=%#v", driver, migrations)
		}
	}
}
