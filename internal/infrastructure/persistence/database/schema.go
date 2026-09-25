package database

import (
	"fmt"

	ormdialect "github.com/domainry/domainry-orm/dialect"
	ormmigration "github.com/domainry/domainry-orm/migration"
	ormschema "github.com/domainry/domainry-orm/schema"
)

const (
	TableProducts                = "products"
	TableProductRevisions        = "product_revisions"
	TableFeatures                = "features"
	TableFeatureRevisions        = "feature_revisions"
	TableRuns                    = "runs"
	TableDeliveryUnits           = "units"
	TableTestCases               = "test_cases"
	TableQualityRuns             = "quality_runs"
	TableAcceptanceCases         = "acceptance_cases"
	TableAcceptanceConfirmations = "acceptance_confirmations"
	TableReleaseChecks           = "release_checks"
	TableReleases                = "releases"
	TableActivity                = "activity"
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
// Every DDL statement is rendered by domainry-orm; repositories never own
// handwritten SQL or dialect branches.
func SchemaStatements(driver string) ([]string, error) {
	renderer, err := ormdialect.ParseRenderer(driver, "", "")
	if err != nil {
		return nil, err
	}
	return schemaStatements(driver, renderer)
}

func schemaStatements(driver string, renderer baseSQLRenderer) ([]string, error) {
	named, err := deliveryRenderer(driver, renderer)
	if err != nil {
		return nil, err
	}
	profile, err := deliveryProfile(driver)
	if err != nil {
		return nil, err
	}
	tables := []struct {
		name    string
		builder *ormschema.TableBuilder
	}{
		{name: TableProducts, builder: productsTable(named)},
		{name: TableProductRevisions, builder: productRevisionsTable(named)},
		{name: TableFeatures, builder: featuresTable(named)},
		{name: TableFeatureRevisions, builder: featureRevisionsTable(named)},
		{name: TableRuns, builder: runsTable(named)},
		{name: TableDeliveryUnits, builder: componentTable(named, TableDeliveryUnits, "unit_id")},
		{name: TableTestCases, builder: componentTable(named, TableTestCases, "test_case_id")},
		{name: TableQualityRuns, builder: componentTable(named, TableQualityRuns, "quality_run_id")},
		{name: TableAcceptanceCases, builder: componentTable(named, TableAcceptanceCases, "acceptance_case_id")},
		{name: TableAcceptanceConfirmations, builder: componentTable(named, TableAcceptanceConfirmations, "confirmation_id")},
		{name: TableReleaseChecks, builder: componentTable(named, TableReleaseChecks, "check_id")},
		{name: TableReleases, builder: componentTable(named, TableReleases, "release_id")},
		{name: TableActivity, builder: componentTable(named, TableActivity, "event_id")},
	}
	statements := make([]string, 0, len(tables)+3)
	for _, table := range tables {
		statement, arguments, buildErr := table.builder.Build()
		if buildErr != nil {
			return nil, fmt.Errorf("build Delivery table %s: %w", table.name, buildErr)
		}
		if len(arguments) != 0 {
			return nil, fmt.Errorf("Delivery table %s unexpectedly contains bound DDL values", table.name)
		}
		statements = append(statements, statement)
	}
	for _, index := range []struct {
		name    string
		table   string
		columns []string
	}{
		{name: "products_updated", table: TableProducts, columns: []string{"workspace_id", "updated_at", "product_id"}},
		{name: "features_queue", table: TableFeatures, columns: []string{"workspace_id", "product_id", "status", "delivery_sequence"}},
		{name: "runs_product_stage", table: TableRuns, columns: []string{"workspace_id", "product_id", "stage", "updated_at"}},
	} {
		builder := profile.ApplyCreateIndex(ormschema.NewIndex(named, index.name, index.table).Columns(index.columns...))
		statement, arguments, buildErr := builder.Build()
		if buildErr != nil {
			return nil, fmt.Errorf("build Delivery index %s: %w", index.name, buildErr)
		}
		if len(arguments) != 0 {
			return nil, fmt.Errorf("Delivery index %s unexpectedly contains bound DDL values", index.name)
		}
		statements = append(statements, statement)
	}
	return statements, nil
}

func productsTable(renderer ormschema.Renderer) *ormschema.TableBuilder {
	return ormschema.NewTable(renderer, TableProducts).IfNotExists().Columns(
		requiredColumn("workspace_id", ormschema.TextKey(191)),
		requiredColumn("product_id", ormschema.TextKey(191)),
		requiredColumn("name", ormschema.LongText()),
		requiredColumn("code", ormschema.TextKey(191)),
		requiredColumn("goal", ormschema.LongText()),
		requiredColumn("industry", ormschema.LongText()),
		requiredColumn("engineering_json", ormschema.Binary()),
		requiredColumn("status", ormschema.TextKey(191)),
		requiredColumn("revision", ormschema.BigInt()),
		requiredColumn("current_definition_revision", ormschema.BigInt()),
		requiredColumn("current_release_revision", ormschema.BigInt()),
		ormschema.Column("current_deployment_json", ormschema.Binary()),
		requiredColumn("created_at", ormschema.TextKey(40)),
		requiredColumn("updated_at", ormschema.TextKey(40)),
	).PrimaryKey("workspace_id", "product_id").Unique("workspace_id", "code")
}

func productRevisionsTable(renderer ormschema.Renderer) *ormschema.TableBuilder {
	return ormschema.NewTable(renderer, TableProductRevisions).IfNotExists().Columns(
		requiredColumn("workspace_id", ormschema.TextKey(191)),
		requiredColumn("product_id", ormschema.TextKey(191)),
		requiredColumn("revision_number", ormschema.BigInt()),
		requiredColumn("revision_json", ormschema.Binary()),
		requiredColumn("created_at", ormschema.TextKey(40)),
	).PrimaryKey("workspace_id", "product_id", "revision_number")
}

func featuresTable(renderer ormschema.Renderer) *ormschema.TableBuilder {
	return ormschema.NewTable(renderer, TableFeatures).IfNotExists().Columns(
		requiredColumn("workspace_id", ormschema.TextKey(191)),
		requiredColumn("product_id", ormschema.TextKey(191)),
		requiredColumn("feature_id", ormschema.TextKey(191)),
		requiredColumn("code", ormschema.TextKey(191)),
		requiredColumn("status", ormschema.TextKey(191)),
		requiredColumn("current_revision", ormschema.BigInt()),
		requiredColumn("confirmed_revision", ormschema.BigInt()),
		requiredColumn("delivery_sequence", ormschema.BigInt()),
		ormschema.Column("queued_at", ormschema.TextKey(40)),
		requiredColumn("delivery_run_id", ormschema.TextKey(191)),
		requiredColumn("installed_release_id", ormschema.TextKey(191)),
		ormschema.Column("draft_json", ormschema.Binary()),
		requiredColumn("created_at", ormschema.TextKey(40)),
		requiredColumn("updated_at", ormschema.TextKey(40)),
	).PrimaryKey("workspace_id", "product_id", "feature_id").Unique("workspace_id", "product_id", "code")
}

func featureRevisionsTable(renderer ormschema.Renderer) *ormschema.TableBuilder {
	return ormschema.NewTable(renderer, TableFeatureRevisions).IfNotExists().Columns(
		requiredColumn("workspace_id", ormschema.TextKey(191)),
		requiredColumn("product_id", ormschema.TextKey(191)),
		requiredColumn("feature_id", ormschema.TextKey(191)),
		requiredColumn("revision_number", ormschema.BigInt()),
		requiredColumn("revision_json", ormschema.Binary()),
		requiredColumn("created_at", ormschema.TextKey(40)),
	).PrimaryKey("workspace_id", "product_id", "feature_id", "revision_number")
}

func runsTable(renderer ormschema.Renderer) *ormschema.TableBuilder {
	return ormschema.NewTable(renderer, TableRuns).IfNotExists().Columns(
		requiredColumn("workspace_id", ormschema.TextKey(191)),
		requiredColumn("run_id", ormschema.TextKey(191)),
		requiredColumn("product_id", ormschema.TextKey(191)),
		requiredColumn("name", ormschema.LongText()),
		requiredColumn("code", ormschema.TextKey(191)),
		requiredColumn("goal", ormschema.LongText()),
		requiredColumn("target_date", ormschema.TextKey(191)),
		requiredColumn("stage", ormschema.TextKey(191)),
		requiredColumn("revision", ormschema.BigInt()),
		requiredColumn("product_snapshot_json", ormschema.Binary()),
		requiredColumn("feature_snapshot_json", ormschema.Binary()),
		requiredColumn("members_json", ormschema.Binary()),
		requiredColumn("active_delivery_unit_id", ormschema.TextKey(191)),
		ormschema.Column("executable_revision_json", ormschema.Binary()),
		requiredColumn("created_at", ormschema.TextKey(40)),
		requiredColumn("updated_at", ormschema.TextKey(40)),
	).PrimaryKey("workspace_id", "run_id")
}

func componentTable(renderer ormschema.Renderer, table, idColumn string) *ormschema.TableBuilder {
	return ormschema.NewTable(renderer, table).IfNotExists().Columns(
		requiredColumn("workspace_id", ormschema.TextKey(191)),
		requiredColumn("run_id", ormschema.TextKey(191)),
		requiredColumn(idColumn, ormschema.TextKey(191)),
		requiredColumn("ordinal", ormschema.BigInt()),
		requiredColumn("value_json", ormschema.Binary()),
	).PrimaryKey("workspace_id", "run_id", idColumn)
}

func requiredColumn(name string, kind ormschema.ColumnType) ormschema.ColumnDefinition {
	return ormschema.Column(name, kind).NotNull()
}

func Migrations(driver string) ([]ormmigration.Migration, error) {
	renderer, err := ormdialect.ParseRenderer(driver, "", "")
	if err != nil {
		return nil, err
	}
	return migrationsForRenderer(driver, renderer)
}

func migrationsForRenderer(driver string, renderer baseSQLRenderer) ([]ormmigration.Migration, error) {
	statements, err := schemaStatements(driver, renderer)
	if err != nil {
		return nil, err
	}
	tables := make([]ormmigration.Table, 0, len(OwnedTables()))
	for _, table := range OwnedTables() {
		tables = append(tables, ormmigration.Table{Name: table})
	}
	return []ormmigration.Migration{{
		Version: 1, Name: "delivery_relational_baseline", Statements: statements,
		Baseline: &ormmigration.Baseline{Tables: tables},
	}}, nil
}
