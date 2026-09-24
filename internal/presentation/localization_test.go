package presentation

import (
	"reflect"
	"testing"
)

func TestNormalizeLocaleUsesSupportedLanguageAndFallsBackToEnglish(t *testing.T) {
	tests := map[string]string{
		"fr-CA,fr;q=0.9,en;q=0.8": "fr",
		"JA-jp":                   "ja",
		"zh-CN,en;q=0.8":          "zh",
		"zh-Hant-TW,en;q=0.8":     "zh-hant",
		"zh_TW,en;q=0.8":          "zh-hant",
		"ar-SA,en;q=0.8":          "ar",
		"":                        "en",
	}
	for input, expected := range tests {
		if actual := NormalizeLocale(input); actual != expected {
			t.Fatalf("NormalizeLocale(%q) = %q; want %q", input, actual, expected)
		}
	}
}

func TestProjectionForLocaleLocalizesDeliveryOwnedReleaseGates(t *testing.T) {
	run := DeliveryRun{
		Feature:       FeatureSnapshot{ID: "feature-1", Source: FeatureRevisionRef{FeatureRevision: 1}},
		DeliveryUnits: []DeliveryUnit{{ID: "feature-1"}},
	}

	projection := ProjectionForLocale(run, "es-MX")
	actual := []ReleaseGate{
		projection.Workflow.ReleaseGates[0],
		projection.Workflow.ReleaseGates[4],
		projection.Workflow.ReleaseGates[7],
	}
	expected := []ReleaseGate{
		{Code: "feature_frozen", Label: "FeatureRevision fijada", Detail: "feature-1", OK: true},
		{Code: "product_revision_bound", Label: "ProductRevision ejecutable vinculada", Detail: "not recorded", OK: false},
		{Code: "release_checks_passed", Label: "Comprobaciones de publicación superadas", Detail: "0 / 0", OK: false},
	}
	if !reflect.DeepEqual(actual, expected) {
		t.Fatalf("localized gates differ:\nactual: %#v\nexpected: %#v", actual, expected)
	}
}

func TestLocalizeErrorPreservesStableCodeAndDetails(t *testing.T) {
	original := &Error{
		Code:    "revision_conflict",
		Message: "The revision changed.",
		Details: map[string]any{"actual_revision": uint64(7)},
	}
	expected := &Error{
		Code:    "revision_conflict",
		Message: "La ressource a changé ; relisez-la avant de soumettre.",
		Details: map[string]any{"actual_revision": uint64(7)},
	}
	if actual := LocalizeError(original, "fr-FR"); !reflect.DeepEqual(actual, expected) {
		t.Fatalf("localized error differs:\nactual: %#v\nexpected: %#v", actual, expected)
	}
}

func TestEverySupportedNonEnglishLocaleHasCompleteDeliveryCopy(t *testing.T) {
	gateCodes := []string{
		"feature_frozen",
		"product_revision_bound",
		"acceptance_passed",
		"release_checks_passed",
	}
	errorKinds := []string{"authentication", "permission", "not_found", "conflict", "internal", "validation"}
	for _, locale := range SupportedLocales[1:] {
		for _, code := range gateCodes {
			if releaseGateLabels[locale][code] == "" {
				t.Fatalf("locale %q is missing release gate %q", locale, code)
			}
		}
		for _, kind := range errorKinds {
			if errorMessages[locale][kind] == "" {
				t.Fatalf("locale %q is missing error kind %q", locale, kind)
			}
		}
	}
}
