package lifecycle

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/domainry/domainry-delivery/internal/domain/deliveryrun"
	"github.com/domainry/domainry-delivery/internal/domain/product"
)

func TestDeploymentRemovalPreservesInstalledProductAndIgnoresNewerReceipt(t *testing.T) {
	now := time.Date(2026, 10, 2, 1, 0, 0, 0, time.UTC)
	for _, receipt := range []string{"receipt://removed", "receipt://newer"} {
		value := Product{
			ID: "product-1", Status: ProductActive, CurrentDefinitionRevision: 3, CurrentReleaseRevision: 3,
			Features:          []product.Feature{{ID: "feature-1", Status: FeatureInstalled}},
			Revisions:         []ProductRevision{{Number: 3, CodeRevision: "accepted-code"}},
			CurrentDeployment: &ProductDeployment{ReleaseID: "REL-001", Version: "1.0.0", EnvironmentRef: "production", ReceiptRef: receipt},
		}
		expected := clone(value)
		if receipt == "receipt://removed" {
			expected.CurrentDeployment = nil
			expected.UpdatedAt = now
		}
		run := DeliveryRun{Releases: []Release{{
			ID: "REL-001", Version: "1.0.0", Status: deliveryrun.ReleaseUnpublished,
			DeploymentAttempts: []deliveryrun.DeploymentAttempt{{Outcome: deliveryrun.DeploymentUnpublished, EnvironmentRef: "production", ReceiptRef: "receipt://removed", ResolvedAt: &now}},
		}}}
		run.Product.ID = value.ID
		payload, err := json.Marshal(map[string]any{"release_id": "REL-001", "outcome": "unpublished"})
		if err != nil {
			t.Fatal(err)
		}
		removed, err := RecordDeploymentRemoval(&value, &run, Command{Actor: Actor{ID: "adapter", Kind: ActorSystem}, Payload: payload}, now)
		if err != nil || !removed {
			t.Fatalf("removal dispatch: %v, %v", removed, err)
		}
		if !reflect.DeepEqual(value, expected) {
			t.Fatalf("product changed beyond exact deployment removal: got %#v want %#v", value, expected)
		}
	}
}
