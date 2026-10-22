package database

import (
	"database/sql"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/domainry/domainry-foundation/schemaownership"
	_ "modernc.org/sqlite"
)

func TestOwnedTableNamesDoNotRepeatDatabaseName(t *testing.T) {
	want := []string{
		"products", "product_revisions", "features", "feature_revisions",
		"runs", "units", "test_cases", "quality_runs", "release_checks", "releases", "activity",
		"feature_messages", "feature_attachments",
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

func TestSchemaOwnershipEnforcesPhysicalAndBusinessIdentityBoundaries(t *testing.T) {
	if err := schemaownership.ValidateAll(SchemaOwnership()); err != nil {
		t.Fatal(err)
	}
	entities := map[string]string{
		TableProducts: "product_id", TableFeatures: "feature_id", TableRuns: "run_id",
		TableFeatureMessages: "message_id", TableFeatureAttachments: "attachment_id",
	}
	for _, table := range SchemaOwnership() {
		if !reflect.DeepEqual(table.PrimaryKey, []string{"id"}) {
			t.Fatalf("table %s primary key=%v", table.Name, table.PrimaryKey)
		}
		wantEntityID, entity := entities[table.Name]
		if entity && (table.Kind != schemaownership.KindEntity || table.EntityIDColumn != wantEntityID) {
			t.Fatalf("table %s entity identity=%q kind=%s", table.Name, table.EntityIDColumn, table.Kind)
		}
		if !entity && (table.Kind != schemaownership.KindDependent || table.EntityIDColumn != "" || len(table.BusinessKey) != 0) {
			t.Fatalf("dependent table %s manufactured an entity identity: %#v", table.Name, table)
		}
	}
}

func TestSchemaOwnershipMatchesExecutableSQLiteSchema(t *testing.T) {
	database, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	statements, err := SchemaStatements("sqlite")
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range statements {
		if _, err := database.Exec(statement); err != nil {
			t.Fatalf("execute Delivery schema: %v\n%s", err, statement)
		}
	}
	rows, err := database.Query(`SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' ORDER BY name`)
	if err != nil {
		t.Fatal(err)
	}
	physicalNames := []string{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		physicalNames = append(physicalNames, name)
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	ownedNames := OwnedTables()
	sort.Strings(ownedNames)
	if !reflect.DeepEqual(physicalNames, ownedNames) {
		t.Fatalf("physical tables=%v ownership=%v", physicalNames, ownedNames)
	}
	for _, table := range SchemaOwnership() {
		columns, createSQL := sqliteTableEvidence(t, database, table.Name)
		id, exists := columns["id"]
		if !exists || id.kind != "INTEGER" || id.primaryKey != 1 || !strings.Contains(strings.ToUpper(createSQL), `"ID" INTEGER PRIMARY KEY AUTOINCREMENT`) {
			t.Fatalf("table %s physical id=%#v sql=%s", table.Name, id, createSQL)
		}
		if table.Kind == schemaownership.KindEntity {
			if _, exists := columns[table.EntityIDColumn]; !exists || !sqliteHasUniqueKey(t, database, table.Name, []string{table.EntityIDColumn}) {
				t.Fatalf("entity table %s is missing unique %s", table.Name, table.EntityIDColumn)
			}
		} else if len(table.UniqueKey) > 0 && !sqliteHasUniqueKey(t, database, table.Name, table.UniqueKey) {
			t.Fatalf("non-entity table %s is missing natural unique key %v", table.Name, table.UniqueKey)
		}
	}
}

type sqliteColumnEvidence struct {
	kind       string
	primaryKey int
}

func sqliteTableEvidence(t *testing.T, database *sql.DB, table string) (map[string]sqliteColumnEvidence, string) {
	t.Helper()
	rows, err := database.Query(fmt.Sprintf(`PRAGMA table_info(%q)`, table))
	if err != nil {
		t.Fatal(err)
	}
	columns := map[string]sqliteColumnEvidence{}
	for rows.Next() {
		var position, required, primaryKey int
		var name, kind string
		var defaultValue sql.NullString
		if err := rows.Scan(&position, &name, &kind, &required, &defaultValue, &primaryKey); err != nil {
			t.Fatal(err)
		}
		columns[name] = sqliteColumnEvidence{kind: strings.ToUpper(kind), primaryKey: primaryKey}
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	var createSQL string
	if err := database.QueryRow(`SELECT sql FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&createSQL); err != nil {
		t.Fatal(err)
	}
	return columns, createSQL
}

func sqliteHasUniqueKey(t *testing.T, database *sql.DB, table string, expected []string) bool {
	t.Helper()
	rows, err := database.Query(fmt.Sprintf(`PRAGMA index_list(%q)`, table))
	if err != nil {
		t.Fatal(err)
	}
	type indexEvidence struct {
		name   string
		unique bool
	}
	indexes := []indexEvidence{}
	for rows.Next() {
		var sequence, unique, partial int
		var name, origin string
		if err := rows.Scan(&sequence, &name, &unique, &origin, &partial); err != nil {
			t.Fatal(err)
		}
		indexes = append(indexes, indexEvidence{name: name, unique: unique == 1})
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	for _, index := range indexes {
		if !index.unique {
			continue
		}
		indexRows, err := database.Query(fmt.Sprintf(`PRAGMA index_info(%q)`, index.name))
		if err != nil {
			t.Fatal(err)
		}
		columns := []string{}
		for indexRows.Next() {
			var sequence, columnID int
			var name string
			if err := indexRows.Scan(&sequence, &columnID, &name); err != nil {
				t.Fatal(err)
			}
			columns = append(columns, name)
		}
		if err := indexRows.Close(); err != nil {
			t.Fatal(err)
		}
		if reflect.DeepEqual(columns, expected) {
			return true
		}
	}
	return false
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
		if strings.Contains(joined, "content_bytes") {
			t.Fatalf("%s schema stores attachment bytes in Delivery SQL", driver)
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
		if len(migrations) != 1 || migrations[0].Version != 1 || migrations[0].Baseline == nil {
			t.Fatalf("%s migration baseline=%#v", driver, migrations)
		}
		statements, err := SchemaStatements(driver)
		if err != nil || !reflect.DeepEqual(migrations[0].Statements, statements) {
			t.Fatalf("%s migration differs from physical baseline: %v", driver, err)
		}
	}
}
