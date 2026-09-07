package processing

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindBrand(t *testing.T) {
	t.Parallel()

	lex := NewBrandLexicon()

	cases := []struct {
		name     string
		text     string
		wantBrand string
		wantOK   bool
	}{
		{"LG in description", "LG FHP10 WASHING MACHINE", "LG", true},
		{"LG not in FLAG", "FLAG FHP10", "", false},
		{"LG not in ALGOL", "ALGOL program", "", false},
		{"Cooler Master preferred over shorter", "CABINET COOLER MASTER CD600", "Cooler Master", true},
		{"Samsung found", "SAMSUNG 43 INCH TV", "Samsung", true},
		{"No brand found", "MICRO WAVE OVEN 30BRC2", "", false},
		{"Empty text", "", "", false},
		{"Brand at start", "SONY BRAVIA", "Sony", true},
		{"Brand at end", "TV SONY", "Sony", true},
		{"Multi-word brand Blue Star", "BLUE STAR AC 1TON", "Blue Star", true},
		{"V-Guard found", "V-GUARD INDUCTION COOKTOP", "V-Guard", true},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, ok := lex.FindBrand(tc.text)
			if ok != tc.wantOK {
				t.Fatalf("FindBrand(%q) ok = %v, want %v", tc.text, ok, tc.wantOK)
			}
			if got != tc.wantBrand {
				t.Fatalf("FindBrand(%q) = %q, want %q", tc.text, got, tc.wantBrand)
			}
		})
	}
}

func TestLoadBrandLexicon(t *testing.T) {
	t.Parallel()

	// Valid JSON file.
	t.Run("valid json", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		path := filepath.Join(dir, "brands.json")
		content := `["Brand One", "Brand Two", "Brand Three"]`
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("write test file: %v", err)
		}
		lex, err := LoadBrandLexicon(path)
		if err != nil {
			t.Fatalf("LoadBrandLexicon returned error: %v", err)
		}
		if len(lex.brands) != 3 {
			t.Fatalf("len(lex.brands) = %d, want 3", len(lex.brands))
		}
		brand, ok := lex.FindBrand("I have a BRAND ONE here")
		if !ok || brand != "Brand One" {
			t.Fatalf("FindBrand = %q, %v; want %q, true", brand, ok, "Brand One")
		}
	})

	// Non-existent file.
	t.Run("file not found", func(t *testing.T) {
		t.Parallel()
		_, err := LoadBrandLexicon("/nonexistent/path/brands.json")
		if err == nil {
			t.Fatalf("LoadBrandLexicon with bad path returned nil error, want error")
		}
	})

	// Invalid JSON.
	t.Run("invalid json", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		path := filepath.Join(dir, "bad.json")
		if err := os.WriteFile(path, []byte("{not valid json"), 0644); err != nil {
			t.Fatalf("write test file: %v", err)
		}
		_, err := LoadBrandLexicon(path)
		if err == nil {
			t.Fatalf("LoadBrandLexicon with invalid JSON returned nil error, want error")
		}
	})
}

type splitCase struct {
	name      string
	desc      string
	wantBrand string
	wantName  string
	wantModel string
}

func TestSplitDescription(t *testing.T) {
	t.Parallel()

	lex := NewBrandLexicon()

	cases := []splitCase{
		{
			name:      "microwave oven with convection and model",
			desc:      "MICRO WAVE OVEN CONVECTION 30BRC2",
			wantBrand: "",
			wantName:  "Microwave Oven",
			wantModel: "30BRC2",
		},
		{
			name:      "cabinet cooler master with model and color",
			desc:      "CABINET COOLER MASTER CD600 BLACK",
			wantBrand: "Cooler Master",
			wantName:  "Cabinet",
			wantModel: "CD600",
		},
		{
			name:      "LG washing machine with model",
			desc:      "LG FHP10 WASHING MACHINE",
			wantBrand: "LG",
			wantName:  "Washing Machine",
			wantModel: "FHP10",
		},
		{
			name:      "Samsung TV with digit token as model",
			desc:      "SAMSUNG 43 INCH TV",
			wantBrand: "Samsung",
			wantName:  "Inch TV",
			wantModel: "43",
		},
		{
			name:      "empty description",
			desc:      "",
			wantBrand: "",
			wantName:  "",
			wantModel: "",
		},
		{
			name:      "whitespace only",
			desc:      "   ",
			wantBrand: "",
			wantName:  "",
			wantModel: "",
		},
		{
			name:      "no model no brand",
			desc:      "WASHING MACHINE",
			wantBrand: "",
			wantName:  "Washing Machine",
			wantModel: "",
		},
		{
			name:      "model with no other tokens",
			desc:      "12345",
			wantBrand: "",
			wantName:  "",
			wantModel: "12345",
		},
		{
			name:      "multi-word brand with model",
			desc:      "BLUE STAR AC 1TON500",
			wantBrand: "Blue Star",
			wantName:  "AC",
			wantModel: "1TON500",
		},
		{
			name:      "descriptor after model dropped",
			desc:      "REFRIGERATOR 255L WHITE",
			wantBrand: "",
			wantName:  "Refrigerator",
			wantModel: "255L",
		},
		{
			name:      "full raw string never returned as model",
			desc:      "SOME LONG DESCRIPTION HERE 123",
			wantBrand: "",
			wantName:  "Some Long Description Here",
			wantModel: "123",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			brand, name, model := SplitDescription(tc.desc, lex)
			if brand != tc.wantBrand {
				t.Errorf("brand = %q, want %q", brand, tc.wantBrand)
			}
			if name != tc.wantName {
				t.Errorf("name = %q, want %q", name, tc.wantName)
			}
			if model != tc.wantModel {
				t.Errorf("model = %q, want %q", model, tc.wantModel)
			}
		})
	}
}

// TestSplitDescriptionNeverReturnsRawAsModel guards the critical constraint:
// the full raw description string (multi-token) must never be returned as the
// model. A single-token input that is itself a model number is an exception.
func TestSplitDescriptionNeverReturnsRawAsModel(t *testing.T) {
	t.Parallel()

	lex := NewBrandLexicon()

	inputs := []string{
		"SINGLE TOKEN 123",
		"TWO WORDS 456",
		"LONGER DESCRIPTION WITH MANY WORDS AND A NUMBER 789",
		"LG FHP10 WASHING MACHINE",
		"CABINET COOLER MASTER CD600 BLACK",
	}

	for _, desc := range inputs {
		_, _, model := SplitDescription(desc, lex)
		if model == desc {
			t.Errorf("model equals full raw input %q", desc)
		}
	}
}
