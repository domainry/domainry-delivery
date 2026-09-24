package database

import (
	"strings"

	ormmigration "github.com/domainry/domainry-orm/migration"
)

const (
	TableProducts                = "delivery_products"
	TableProductRevisions        = "delivery_product_revisions"
	TableFeatures                = "delivery_features"
	TableFeatureRevisions        = "delivery_feature_revisions"
	TableRuns                    = "delivery_runs"
	TableDeliveryUnits           = "delivery_units"
	TableTestCases               = "delivery_test_cases"
	TableQualityRuns             = "delivery_quality_runs"
	TableAcceptanceCases         = "delivery_acceptance_cases"
	TableAcceptanceConfirmations = "delivery_acceptance_confirmations"
	TableReleaseChecks           = "delivery_release_checks"
	TableReleases                = "delivery_releases"
	TableActivity                = "delivery_activity"
)

func OwnedTables() []string {
	return []string{
		TableProducts, TableProductRevisions, TableFeatures, TableFeatureRevisions,
		TableRuns, TableDeliveryUnits, TableTestCases, TableQualityRuns,
		TableAcceptanceCases, TableAcceptanceConfirmations, TableReleaseChecks,
		TableReleases, TableActivity,
	}
}

// SchemaStatements is the one physical baseline shared by Module and SaaS.
// Mutable aggregate headers are relational rows. Only bounded value documents
// and immutable revision payloads use JSON columns.
func SchemaStatements(dialect string) []string {
	keyType, jsonType, uintType, longText := "TEXT", "BLOB", "INTEGER", "TEXT"
	productsIndex, featuresIndex, runsIndex := "", "", ""
	if dialect == "mysql" {
		keyType, jsonType, uintType, longText = "VARCHAR(191)", "LONGBLOB", "BIGINT UNSIGNED", "LONGTEXT"
		productsIndex = ", INDEX delivery_products_updated (workspace_id, updated_at, product_id)"
		featuresIndex = ", INDEX delivery_features_queue (workspace_id, product_id, status, delivery_sequence)"
		runsIndex = ", INDEX delivery_runs_product_stage (workspace_id, product_id, stage, updated_at)"
	}
	statements := []string{
		"CREATE TABLE IF NOT EXISTS delivery_products (" +
			"workspace_id " + keyType + " NOT NULL, product_id " + keyType + " NOT NULL, name " + longText + " NOT NULL, code " + keyType + " NOT NULL, goal " + longText + " NOT NULL, industry " + longText + " NOT NULL, " +
			"engineering_json " + jsonType + " NOT NULL, status " + keyType + " NOT NULL, revision " + uintType + " NOT NULL, current_definition_revision " + uintType + " NOT NULL, current_release_revision " + uintType + " NOT NULL, current_deployment_json " + jsonType + " NULL, " +
			"created_at VARCHAR(40) NOT NULL, updated_at VARCHAR(40) NOT NULL, PRIMARY KEY (workspace_id, product_id), UNIQUE (workspace_id, code)" + productsIndex + ")",
		"CREATE TABLE IF NOT EXISTS delivery_product_revisions (" +
			"workspace_id " + keyType + " NOT NULL, product_id " + keyType + " NOT NULL, revision_number " + uintType + " NOT NULL, revision_json " + jsonType + " NOT NULL, created_at VARCHAR(40) NOT NULL, PRIMARY KEY (workspace_id, product_id, revision_number))",
		"CREATE TABLE IF NOT EXISTS delivery_features (" +
			"workspace_id " + keyType + " NOT NULL, product_id " + keyType + " NOT NULL, feature_id " + keyType + " NOT NULL, code " + keyType + " NOT NULL, status " + keyType + " NOT NULL, current_revision " + uintType + " NOT NULL, confirmed_revision " + uintType + " NOT NULL, delivery_sequence " + uintType + " NOT NULL, " +
			"queued_at VARCHAR(40) NULL, delivery_run_id " + keyType + " NOT NULL, installed_release_id " + keyType + " NOT NULL, draft_json " + jsonType + " NULL, created_at VARCHAR(40) NOT NULL, updated_at VARCHAR(40) NOT NULL, PRIMARY KEY (workspace_id, product_id, feature_id), UNIQUE (workspace_id, product_id, code)" + featuresIndex + ")",
		"CREATE TABLE IF NOT EXISTS delivery_feature_revisions (" +
			"workspace_id " + keyType + " NOT NULL, product_id " + keyType + " NOT NULL, feature_id " + keyType + " NOT NULL, revision_number " + uintType + " NOT NULL, revision_json " + jsonType + " NOT NULL, created_at VARCHAR(40) NOT NULL, PRIMARY KEY (workspace_id, product_id, feature_id, revision_number))",
		"CREATE TABLE IF NOT EXISTS delivery_runs (" +
			"workspace_id " + keyType + " NOT NULL, run_id " + keyType + " NOT NULL, product_id " + keyType + " NOT NULL, name " + longText + " NOT NULL, code " + keyType + " NOT NULL, goal " + longText + " NOT NULL, target_date " + keyType + " NOT NULL, stage " + keyType + " NOT NULL, revision " + uintType + " NOT NULL, " +
			"product_snapshot_json " + jsonType + " NOT NULL, feature_snapshot_json " + jsonType + " NOT NULL, members_json " + jsonType + " NOT NULL, active_delivery_unit_id " + keyType + " NOT NULL, executable_revision_json " + jsonType + " NULL, created_at VARCHAR(40) NOT NULL, updated_at VARCHAR(40) NOT NULL, PRIMARY KEY (workspace_id, run_id)" + runsIndex + ")",
		componentTableSQL("delivery_units", keyType, jsonType, uintType, "unit_id"),
		componentTableSQL("delivery_test_cases", keyType, jsonType, uintType, "test_case_id"),
		componentTableSQL("delivery_quality_runs", keyType, jsonType, uintType, "quality_run_id"),
		componentTableSQL("delivery_acceptance_cases", keyType, jsonType, uintType, "acceptance_case_id"),
		componentTableSQL("delivery_acceptance_confirmations", keyType, jsonType, uintType, "confirmation_id"),
		componentTableSQL("delivery_release_checks", keyType, jsonType, uintType, "check_id"),
		componentTableSQL("delivery_releases", keyType, jsonType, uintType, "release_id"),
		componentTableSQL("delivery_activity", keyType, jsonType, uintType, "event_id"),
	}
	if dialect != "mysql" {
		statements = append(statements,
			"CREATE INDEX IF NOT EXISTS delivery_products_updated ON delivery_products (workspace_id, updated_at, product_id)",
			"CREATE INDEX IF NOT EXISTS delivery_features_queue ON delivery_features (workspace_id, product_id, status, delivery_sequence)",
			"CREATE INDEX IF NOT EXISTS delivery_runs_product_stage ON delivery_runs (workspace_id, product_id, stage, updated_at)",
		)
	}
	return statements
}

func componentTableSQL(table, keyType, jsonType, uintType, idColumn string) string {
	return "CREATE TABLE IF NOT EXISTS " + table + " (" +
		"workspace_id " + keyType + " NOT NULL, run_id " + keyType + " NOT NULL, " + idColumn + " " + keyType + " NOT NULL, ordinal " + uintType + " NOT NULL, value_json " + jsonType + " NOT NULL, " +
		"PRIMARY KEY (workspace_id, run_id, " + idColumn + "))"
}

func Migrations(dialect string) []ormmigration.Migration {
	tables := make([]ormmigration.Table, 0, len(OwnedTables()))
	for _, table := range OwnedTables() {
		tables = append(tables, ormmigration.Table{Name: table})
	}
	return []ormmigration.Migration{{
		Version: 1, Name: "delivery_relational_baseline", Statements: SchemaStatements(dialect),
		Baseline: &ormmigration.Baseline{Tables: tables},
	}}
}

func (store *Store) insertIgnore(statement string) string {
	statement = strings.TrimSpace(statement)
	if store.dialect == "mysql" {
		return strings.Replace(statement, "INSERT INTO", "INSERT IGNORE INTO", 1)
	}
	return statement + " ON CONFLICT DO NOTHING"
}
