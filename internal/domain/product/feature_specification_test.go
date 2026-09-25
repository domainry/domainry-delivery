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
