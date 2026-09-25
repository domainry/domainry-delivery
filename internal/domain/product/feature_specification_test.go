package product

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestFeatureSpecificationPreservesRecordSources(t *testing.T) {
	sourceIDs := []string{"conversation://conversation-1/message/message-1", "attachment://attachment-1"}
	specification := normalizeFeatureSpecification(FeatureSpecification{
		Scenarios:  []FeatureScenario{{ID: "scenario-1", SourceIDs: sourceIDs}},
		Impacts:    []FeatureImpact{{ID: "impact-1", SourceIDs: sourceIDs}},
		Acceptance: []FeatureAcceptanceScenario{{ID: "acceptance-1", SourceIDs: sourceIDs}},
	})
	encoded, err := json.Marshal(specification)
	if err != nil {
		t.Fatal(err)
	}
	var restored FeatureSpecification
	if err := json.Unmarshal(encoded, &restored); err != nil {
		t.Fatal(err)
	}
	got := [][]string{
		restored.Scenarios[0].SourceIDs,
		restored.Impacts[0].SourceIDs,
		restored.Acceptance[0].SourceIDs,
	}
	want := [][]string{sourceIDs, sourceIDs, sourceIDs}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("record source references were not preserved: got %v, want %v", got, want)
	}
}

func TestFeatureSpecificationRecordSourcesRequireRegisteredProvenance(t *testing.T) {
	specification := FeatureSpecification{
		Scenarios:  []FeatureScenario{{SourceIDs: []string{"source-valid"}}},
		Impacts:    []FeatureImpact{{SourceIDs: []string{"source-valid"}}},
		Acceptance: []FeatureAcceptanceScenario{{SourceIDs: []string{"source-valid"}}},
	}
	sources := []FeatureSource{{SourceIDs: []string{"source-valid"}}}
	if err := validateFeatureLineageReferences(FeatureDiscovery{}, specification, nil, sources); err != nil {
		t.Fatalf("registered record sources should be accepted: %v", err)
	}
	for _, target := range []string{"scenario", "impact", "acceptance"} {
		changed := specification
		switch target {
		case "scenario":
			changed.Scenarios = []FeatureScenario{{SourceIDs: []string{"source-invented"}}}
		case "impact":
			changed.Impacts = []FeatureImpact{{SourceIDs: []string{"source-invented"}}}
		case "acceptance":
			changed.Acceptance = []FeatureAcceptanceScenario{{SourceIDs: []string{"source-invented"}}}
		}
		if err := validateFeatureLineageReferences(FeatureDiscovery{}, changed, nil, sources); err == nil || err.Error() != "feature_source_reference_unverified" {
			t.Fatalf("%s must reject unregistered source, got %v", target, err)
		}
	}
}
