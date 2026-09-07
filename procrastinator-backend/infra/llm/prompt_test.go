package llm

import (
	"strings"
	"testing"
)

func TestSystemPrompt_RequestsConfidence(t *testing.T) {
	prompt := SystemPrompt()
	if !strings.Contains(prompt, "confidence:") {
		t.Fatalf("SystemPrompt() missing \"confidence:\" — prompt does not instruct the LLM to emit a confidence field")
	}
}

func TestSystemPrompt_RequestsBrandNameModelSplit(t *testing.T) {
	prompt := SystemPrompt()
	upper := strings.ToUpper(prompt)
	if !strings.Contains(prompt, "name:") {
		t.Error("SystemPrompt() missing \"name:\" — prompt does not request a canonical product name field")
	}
	if !strings.Contains(upper, "DO NOT PUT THE WHOLE DESCRIPTION") {
		t.Error("SystemPrompt() missing brand/name/model split instruction — must tell the LLM NOT to put the whole description in model")
	}
}

func TestSystemPrompt_RequestsAssetCategory(t *testing.T) {
	prompt := SystemPrompt()
	if !strings.Contains(prompt, "asset_category") {
		t.Error("SystemPrompt() missing \"asset_category\" field")
	}
	for _, v := range []string{"appliance", "electronics"} {
		if !strings.Contains(prompt, v) {
			t.Errorf("SystemPrompt() asset_category vocabulary missing %q", v)
		}
	}
}

func TestSystemPrompt_RequestsWarrantyDuration(t *testing.T) {
	prompt := SystemPrompt()
	if !strings.Contains(prompt, "warranty_duration") {
		t.Error("SystemPrompt() missing \"warranty_duration\" field")
	}
}
