package module

import (
	"context"
	"reflect"
	"strings"
	"testing"

	deliverysdk "github.com/domainry/domainry-delivery-sdk"
	"github.com/domainry/domainry-delivery-sdk/contract"
	"github.com/domainry/domainry-delivery/internal/domain/deliveryrun"
)

func TestModuleReadConversionPreservesModelImpactAndItsHistory(t *testing.T) {
	receipt := deliveryrun.ModelRepairImpact{BaselineGitRevision: strings.Repeat("a", 40), BaselineModelSHA256: strings.Repeat("b", 64),
		CurrentGitRevision: strings.Repeat("c", 40), CurrentModelSHA256: strings.Repeat("d", 64), SourceImpact: "permissions_only"}
	run := deliveryrun.DeliveryRun{DeliveryUnits: []deliveryrun.DeliveryUnit{{ID: "unit-1", ModelRepairImpact: &receipt, ModelRepairImpactHistory: []deliveryrun.ModelRepairImpact{receipt}}}}
	projection, err := convert[deliverysdk.DeliveryRunProjection](context.Background(), run, nil)
	if err != nil {
		t.Fatal(err)
	}
	sdkReceipt := contract.ModelRepairImpact(receipt)
	expected := []contract.DeliveryUnit{{ID: "unit-1", ModelRepairImpact: &sdkReceipt, ModelRepairImpactHistory: []contract.ModelRepairImpact{sdkReceipt}}}
	if !reflect.DeepEqual(projection.DeliveryUnits, expected) {
		t.Fatalf("Module conversion lost model repair proof: %#v", projection.DeliveryUnits)
	}
}
