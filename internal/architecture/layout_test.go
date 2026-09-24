package architecture_test

import (
	"bufio"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"

	commanddomain "github.com/domainry/domainry-delivery/internal/domain/command"
	deliverydb "github.com/domainry/domainry-delivery/internal/infrastructure/persistence/database"
)

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate architecture test")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
}

func TestDeliveryImplementationHasExplicitModuleBoundaries(t *testing.T) {
	root := repositoryRoot(t)
	required := []string{
		"internal/domain/product",
		"internal/domain/deliveryrun",
		"internal/domain/lifecycle",
		"internal/application",
		"internal/assembly/module",
		"internal/assembly/saas",
		"internal/infrastructure/persistence/database",
		"internal/transport/http",
	}
	for _, name := range required {
		info, err := os.Stat(filepath.Join(root, name))
		if err != nil || !info.IsDir() {
			t.Fatalf("required module boundary %s is missing", name)
		}
	}
	for _, name := range []string{
		"internal/domain/delivery",
		"internal/application/delivery",
		"internal/infrastructure/delivery",
		"internal/transport/httpapi",
		"remote",
	} {
		if _, err := os.Stat(filepath.Join(root, name)); !os.IsNotExist(err) {
			t.Fatalf("obsolete catch-all or implementation facade %s still exists", name)
		}
	}
}

func TestProductionFilesRemainOwnedAndBounded(t *testing.T) {
	root := repositoryRoot(t)
	err := filepath.WalkDir(filepath.Join(root, "internal"), func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			return nil
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()
		lines := 0
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			lines++
		}
		if err := scanner.Err(); err != nil {
			return err
		}
		if lines > 500 {
			t.Errorf("%s has %d lines; split it by invariant or owner", strings.TrimPrefix(path, root+string(filepath.Separator)), lines)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	moduleFacade, err := os.ReadFile(filepath.Join(root, "module", "module.go"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes := strings.Count(string(moduleFacade), "\n") + 1; bytes > 20 || !strings.Contains(string(moduleFacade), "internal/assembly/module") {
		t.Fatalf("module/module.go must remain a thin assembly facade; got %d lines", bytes)
	}
}

func TestDomainAndApplicationDoNotChooseConcreteIO(t *testing.T) {
	root := repositoryRoot(t)
	for _, directory := range []string{"internal/domain", "internal/application"} {
		err := filepath.WalkDir(filepath.Join(root, directory), func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil || entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
				return walkErr
			}
			parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
			if err != nil {
				return err
			}
			for _, imported := range parsed.Imports {
				name, err := strconv.Unquote(imported.Path.Value)
				if err != nil {
					return err
				}
				if name == "database/sql" || name == "net/http" || strings.Contains(name, "/internal/infrastructure/") || strings.Contains(name, "/internal/transport/") || strings.Contains(name, "/internal/assembly/") {
					t.Errorf("%s imports concrete I/O or assembly package %s", strings.TrimPrefix(path, root+string(filepath.Separator)), name)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestDomainErrorsRemainSemantic(t *testing.T) {
	root := repositoryRoot(t)
	err := filepath.WalkDir(filepath.Join(root, "internal", "domain"), func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			return walkErr
		}
		files := token.NewFileSet()
		parsed, err := parser.ParseFile(files, path, nil, 0)
		if err != nil {
			return err
		}
		ast.Inspect(parsed, func(node ast.Node) bool {
			switch current := node.(type) {
			case *ast.CallExpr:
				name := ""
				switch function := current.Fun.(type) {
				case *ast.Ident:
					name = function.Name
				case *ast.SelectorExpr:
					name = function.Sel.Name
				}
				if name == "Invalid" && len(current.Args) != 1 {
					position := files.Position(current.Pos())
					t.Errorf("%s:%d domain errors must carry only a stable code", strings.TrimPrefix(path, root+string(filepath.Separator)), position.Line)
				}
			case *ast.CompositeLit:
				identifier, ok := current.Type.(*ast.Ident)
				if !ok || identifier.Name != "Error" {
					return true
				}
				for _, element := range current.Elts {
					field, ok := element.(*ast.KeyValueExpr)
					if !ok {
						continue
					}
					key, keyOK := field.Key.(*ast.Ident)
					if keyOK && key.Name == "Message" {
						position := files.Position(field.Pos())
						t.Errorf("%s:%d domain errors must not choose presentation messages", strings.TrimPrefix(path, root+string(filepath.Separator)), position.Line)
					}
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestTypedCommandCatalogIsTheCompleteMutationInventory(t *testing.T) {
	expected := []string{
		commanddomain.AcceptanceConfirm,
		commanddomain.DeliveryUnitBackendComplete,
		commanddomain.DeliveryUnitContractVerify,
		commanddomain.DeliveryUnitFrontendComplete,
		commanddomain.DeliveryUnitGapReport,
		commanddomain.DeliveryUnitInteractionComplete,
		commanddomain.DeliveryUnitJourneyComplete,
		commanddomain.DeliveryUnitModelComplete,
		commanddomain.DeliveryUnitModelVerify,
		commanddomain.FeatureConfirm,
		commanddomain.FeatureDeliveryStart,
		commanddomain.FeatureDiscoveryOpen,
		commanddomain.FeatureDiscoveryReplace,
		commanddomain.ProductCreate,
		commanddomain.ProductDelete,
		commanddomain.ProductFoundationComplete,
		commanddomain.ProductFoundationFailed,
		commanddomain.ProductFoundationStarted,
		commanddomain.ProductFrontendComplete,
		commanddomain.ProductFrontendStart,
		commanddomain.ProductRevisionRecord,
		commanddomain.QualityRecord,
		commanddomain.ReleaseApprove,
		commanddomain.ReleaseCheckRecord,
		commanddomain.ReleaseChecksReplace,
		commanddomain.ReleaseDeployResult,
		commanddomain.ReleasePrepare,
		commanddomain.ReleaseReconcile,
	}
	slices.Sort(expected)
	definitions := commanddomain.Definitions()
	actual := make([]string, 0, len(definitions))
	for _, definition := range definitions {
		actual = append(actual, definition.Key)
		if definition.Target == "" || definition.PayloadType == "" || definition.ReceiptType == "" || definition.Permission == "" || len(definition.AllowedActors) == 0 || len(definition.LegalStates) == 0 {
			t.Errorf("command %s has incomplete typed mutation metadata: %#v", definition.Key, definition)
		}
	}
	if !slices.Equal(actual, expected) {
		t.Fatalf("command inventory changed without updating the architecture gate:\nactual=%v\nexpected=%v", actual, expected)
	}
}

func TestHTTPDocumentationCoversTheTypedCommandCatalogAndLocales(t *testing.T) {
	root := repositoryRoot(t)
	documentation, err := os.ReadFile(filepath.Join(root, "docs", "api.md"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(documentation)
	for _, definition := range commanddomain.Definitions() {
		if !strings.Contains(text, "`"+definition.Key+"`") {
			t.Errorf("docs/api.md does not list typed command %s", definition.Key)
		}
	}
	for _, locale := range []string{"en", "zh", "zh-hant", "ja", "ko", "es", "pt", "fr", "de", "it", "tr", "ar"} {
		if !strings.Contains(text, "`"+locale+"`") {
			t.Errorf("docs/api.md does not list supported locale %s", locale)
		}
	}
}

func TestOwnedSchemaHasNoAggregateBlobsOrPrivateReceipts(t *testing.T) {
	for _, driver := range []string{"sqlite", "mysql"} {
		joined := strings.ToLower(strings.Join(deliverydb.SchemaStatements(driver), "\n"))
		for _, forbidden := range []string{"state_json", "command_receipt", "feature_attachment"} {
			if strings.Contains(joined, forbidden) {
				t.Fatalf("%s Delivery schema retained forbidden ownership %q", driver, forbidden)
			}
		}
	}
}

func TestStrongWritesUseTheSharedCommandExecutor(t *testing.T) {
	root := repositoryRoot(t)
	applicationService, err := os.ReadFile(filepath.Join(root, "internal/application/service.go"))
	if err != nil {
		t.Fatal(err)
	}
	if count := strings.Count(string(applicationService), "executeMutation("); count != 3 {
		t.Fatalf("all three application write entrypoints must use executeMutation; got %d", count)
	}
	executor, err := os.ReadFile(filepath.Join(root, "internal/infrastructure/persistence/database/command_executor.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"BeginTx", "claimOperation", "completeOperation", "transaction.Commit"} {
		if !strings.Contains(string(executor), required) {
			t.Fatalf("shared command executor no longer owns %s", required)
		}
	}
	for _, filename := range []string{"product_repository.go", "deliveryrun_repository.go", "lifecycle_repository.go"} {
		contents, err := os.ReadFile(filepath.Join(root, "internal/infrastructure/persistence/database", filename))
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{"claimOperation(", "completeOperation("} {
			if strings.Contains(string(contents), forbidden) {
				t.Fatalf("%s bypasses the shared command executor through %s", filename, forbidden)
			}
		}
		if !strings.Contains(string(contents), "executeCommand(") {
			t.Fatalf("%s has no strong write routed through executeCommand", filename)
		}
	}
	err = filepath.WalkDir(filepath.Join(root, "internal/infrastructure/persistence/database"), func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") || entry.Name() == "command_executor.go" || entry.Name() == "migration_host.go" {
			return walkErr
		}
		contents, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(contents), "BeginTx(") {
			t.Errorf("%s opens a write transaction outside the shared command executor", strings.TrimPrefix(path, root+string(filepath.Separator)))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// Keep ast imported through a compile-time assertion so this test fails if the
// parser API shape changes instead of silently weakening the import gate.
var _ ast.Node = (*ast.File)(nil)
