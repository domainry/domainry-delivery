package database

import (
	"reflect"
	"strings"
	"testing"
)

func TestOwnedTableNamesDoNotRepeatDatabaseName(t *testing.T) {
	want := []string{
		"products", "product_revisions", "features", "feature_revisions",
		"runs", "units", "test_cases", "quality_runs", "acceptance_cases",
		"acceptance_confirmations", "release_checks", "releases", "activity",
	}
	if got := OwnedTables(); !reflect.DeepEqual(got, want) {
		t.Fatalf("owned tables = %#v, want %#v", got, want)
	}
	for _, table := range OwnedTables() {
		if strings.HasPrefix(table, "delivery_") {
			t.Fatalf("table %q repeats the standalone database name", table)
		}
	}
}

func TestSchemaHasOnePortableSourceForSQLiteAndMySQL(t *testing.T) {
	for _, driver := range []string{"sqlite", "mysql"} {
		statements, err := SchemaStatements(driver)
		if err != nil {
			t.Fatal(err)
		}
		joined := strings.Join(statements, "\n")
		for _, table := range OwnedTables() {
			if !strings.Contains(joined, table) {
				t.Fatalf("%s schema is missing %s", driver, table)
			}
		}
		if strings.Contains(joined, "delivery_feature_attachments") {
			t.Fatalf("%s schema retained Agent-owned attachment bytes", driver)
		}
		if strings.Contains(joined, "TABLE IF NOT EXISTS delivery_") {
			t.Fatalf("%s schema retained a redundant delivery_ table prefix", driver)
		}
	}
	mysqlStatements, err := SchemaStatements("mysql")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(mysqlStatements, "\n"), "LONGBLOB") {
		t.Fatal("MySQL schema did not render bounded aggregate documents for MySQL")
	}
}

func TestSchemaMigrationsAreTheSamePhysicalBaseline(t *testing.T) {
	for _, driver := range []string{"sqlite", "mysql"} {
		migrations, err := Migrations(driver)
		if err != nil {
			t.Fatal(err)
		}
		if len(migrations) != 1 || migrations[0].Version != 1 || migrations[0].Baseline == nil || len(migrations[0].Baseline.Tables) != len(OwnedTables()) {
			t.Fatalf("%s migration baseline=%#v", driver, migrations)
		}
	}
}
