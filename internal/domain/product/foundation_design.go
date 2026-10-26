package product

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"
	"time"
)

const maxFoundationModelBytes = 256 * 1024
const maxFoundationCapabilities = 32

func confirmedFoundationScope(product *Product) []FoundationFeatureRevision {
	scope := make([]FoundationFeatureRevision, 0)
	for _, feature := range product.Features {
		if feature.Status != FeatureConfirmed || len(feature.Revisions) == 0 {
			continue
		}
		revision := feature.Revisions[len(feature.Revisions)-1]
		if revision.Number == 0 || revision.Number != feature.CurrentRevision {
			continue
		}
		scope = append(scope, FoundationFeatureRevision{
			FeatureID: feature.ID, FeatureRevision: revision.Number,
		})
	}
	sort.Slice(scope, func(i, j int) bool {
		return scope[i].FeatureID < scope[j].FeatureID
	})
	return scope
}

type foundationDesignInput struct {
	Model             json.RawMessage
	Capabilities      []string
	ModuleDeployments []FoundationModuleDeployment
}

func prepareFoundationDesign(product *Product, input foundationDesignInput, recordedBy string, now time.Time) (*FoundationDesign, error) {
	if len(product.Engineering.InitialFeatureRevisions) == 0 {
		return nil, Invalid("product_initial_scope_missing")
	}
	model, modelSHA256, err := canonicalFoundationModel(input.Model)
	if err != nil {
		return nil, err
	}
	capabilities, err := normalizeFoundationCapabilities(input.Capabilities)
	if err != nil {
		return nil, err
	}
	deployments, err := normalizeFoundationDeployments(input.ModuleDeployments, capabilities)
	if err != nil {
		return nil, err
	}
	identity := struct {
		ProductRevision         uint64                       `json:"product_revision"`
		InitialFeatureRevisions []FoundationFeatureRevision  `json:"initial_feature_revisions"`
		ModelSHA256             string                       `json:"model_sha256"`
		Capabilities            []string                     `json:"capabilities"`
		ModuleDeployments       []FoundationModuleDeployment `json:"module_deployments"`
	}{
		ProductRevision:         product.CurrentDefinitionRevision,
		InitialFeatureRevisions: product.Engineering.InitialFeatureRevisions,
		ModelSHA256:             modelSHA256,
		Capabilities:            capabilities,
		ModuleDeployments:       deployments,
	}
	identityBytes, err := json.Marshal(identity)
	if err != nil {
		return nil, Invalid("product_foundation_design_invalid")
	}
	designDigest := sha256.Sum256(identityBytes)
	return &FoundationDesign{
		ProductRevision:         identity.ProductRevision,
		InitialFeatureRevisions: clone(identity.InitialFeatureRevisions),
		Model:                   model,
		ModelSHA256:             modelSHA256,
		Capabilities:            capabilities,
		ModuleDeployments:       deployments,
		DesignSHA256:            hex.EncodeToString(designDigest[:]),
		RecordedBy:              recordedBy,
		RecordedAt:              now,
	}, nil
}

func canonicalFoundationModel(raw json.RawMessage) ([]byte, string, error) {
	if len(raw) == 0 || len(raw) > maxFoundationModelBytes {
		return nil, "", Invalid("product_foundation_model_invalid")
	}
	var model map[string]any
	if err := json.Unmarshal(raw, &model); err != nil || len(model) == 0 {
		return nil, "", Invalid("product_foundation_model_invalid")
	}
	canonical, err := json.Marshal(model)
	if err != nil || len(canonical) > maxFoundationModelBytes {
		return nil, "", Invalid("product_foundation_model_invalid")
	}
	digest := sha256.Sum256(canonical)
	return canonical, hex.EncodeToString(digest[:]), nil
}

func normalizeFoundationCapabilities(values []string) ([]string, error) {
	if len(values) > maxFoundationCapabilities {
		return nil, Invalid("product_foundation_capabilities_invalid")
	}
	seen := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.ReplaceAll(strings.ToLower(strings.TrimSpace(value)), "-", "_")
		if value == "" || len(value) > 128 || seen[value] {
			return nil, Invalid("product_foundation_capabilities_invalid")
		}
		seen[value] = true
		result = append(result, value)
	}
	sort.Strings(result)
	return result, nil
}

func normalizeFoundationDeployments(values []FoundationModuleDeployment, capabilities []string) ([]FoundationModuleDeployment, error) {
	selected := make(map[string]bool, len(capabilities))
	for _, capability := range capabilities {
		selected[capability] = true
	}
	seen := map[string]bool{}
	result := make([]FoundationModuleDeployment, 0, len(values))
	for _, value := range values {
		capability := strings.ReplaceAll(strings.ToLower(strings.TrimSpace(value.Capability)), "-", "_")
		mode := strings.ToLower(strings.TrimSpace(value.Mode))
		if !selected[capability] || seen[capability] || (mode != "module" && mode != "saas") {
			return nil, Invalid("product_foundation_deployments_invalid")
		}
		seen[capability] = true
		result = append(result, FoundationModuleDeployment{Capability: capability, Mode: mode})
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Capability < result[j].Capability
	})
	return result, nil
}
