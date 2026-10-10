package application

import "testing"

func TestValidExternalFeatureSourceRequiresPublicHTTPSPage(t *testing.T) {
	tests := map[string]bool{
		"https://www.gov.br/receitafederal/ncm": true,
		"http://www.gov.br/receitafederal/ncm":  false,
		"https://user@example.com/source":       false,
		"https:///missing-host":                 false,
		"conversation://conversation/message/1": false,
	}
	for sourceID, expected := range tests {
		if got := validExternalFeatureSource(sourceID); got != expected {
			t.Fatalf("validExternalFeatureSource(%q) = %t, want %t", sourceID, got, expected)
		}
	}
}
