package parse

import (
	"strings"
	"testing"
	"time"
)

func TestParseExtraction_FullPayload(t *testing.T) {
	t.Parallel()
	raw := `{
		"classification": "invoice",
		"brand": "Samsung",
		"model": "WF80A",
		"serial_number": "WM-2024-001",
		"purchase_date": "2023-11-03",
		"warranty_end": "2026-11-03",
		"price": "39999.99",
		"currency": "INR",
		"metadata": {"invoice_number": "INV-123", "customer_name": "John Doe"}
	}`
	ext, err := ParseExtraction(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ext.Classification != "invoice" {
		t.Errorf("Classification = %q, want %q", ext.Classification, "invoice")
	}
	if ext.Brand == nil || *ext.Brand != "Samsung" {
		t.Errorf("Brand = %v, want pointer to %q", ext.Brand, "Samsung")
	}
	if ext.Model == nil || *ext.Model != "WF80A" {
		t.Errorf("Model = %v, want pointer to %q", ext.Model, "WF80A")
	}
	if ext.SerialNumber == nil || *ext.SerialNumber != "WM-2024-001" {
		t.Errorf("SerialNumber = %v, want pointer to %q", ext.SerialNumber, "WM-2024-001")
	}
	if ext.PurchaseDate == nil {
		t.Error("PurchaseDate = nil, want non-nil")
	} else {
		want := time.Date(2023, time.November, 3, 0, 0, 0, 0, time.UTC)
		if !ext.PurchaseDate.Equal(want) {
			t.Errorf("PurchaseDate = %v, want %v", ext.PurchaseDate, want)
		}
	}
	if ext.WarrantyEnd == nil {
		t.Error("WarrantyEnd = nil, want non-nil")
	} else {
		want := time.Date(2026, time.November, 3, 0, 0, 0, 0, time.UTC)
		if !ext.WarrantyEnd.Equal(want) {
			t.Errorf("WarrantyEnd = %v, want %v", ext.WarrantyEnd, want)
		}
	}
	if ext.Price == nil || *ext.Price != "39999.99" {
		t.Errorf("Price = %v, want pointer to %q", ext.Price, "39999.99")
	}
	if ext.Currency == nil || *ext.Currency != "INR" {
		t.Errorf("Currency = %v, want pointer to %q", ext.Currency, "INR")
	}
	if len(ext.Metadata) != 2 {
		t.Errorf("Metadata len = %d, want 2", len(ext.Metadata))
	} else {
		if ext.Metadata["invoice_number"] != "INV-123" {
			t.Errorf("Metadata[invoice_number] = %v, want %q", ext.Metadata["invoice_number"], "INV-123")
		}
		if ext.Metadata["customer_name"] != "John Doe" {
			t.Errorf("Metadata[customer_name] = %v, want %q", ext.Metadata["customer_name"], "John Doe")
		}
	}
	if ext.RawPayload != raw {
		t.Errorf("RawPayload = %q, want original raw", ext.RawPayload)
	}
}

func TestParseExtraction_ThoughtStripping(t *testing.T) {
	t.Parallel()

	pureJSON := `{"classification":"warranty","brand":"LG","model":"FHP10","serial_number":"SN001"}`

	tests := []struct {
		name string
		raw  string
	}{
		{
			name: "pure JSON no thoughts",
			raw:  pureJSON,
		},
		{
			name: "single thought before JSON",
			raw:  `<thought>Let me analyze this document...</thought>` + pureJSON,
		},
		{
			name: "multiple thought blocks",
			raw:  `<thought>first thought</thought>` + pureJSON + `<thought>second thought</thought>`,
		},
		{
			name: "thought with whitespace and newlines",
			raw:  "<thought>\n  I need to look at the dates carefully.\n  The brand is LG.\n</thought>\n  " + pureJSON,
		},
		{
			name: "thought containing braces",
			raw:  `<thought>the JSON should be like {"brand":"LG"}</thought>` + pureJSON,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ext, err := ParseExtraction(tt.raw)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if ext.Classification != "warranty" {
				t.Errorf("Classification = %q, want %q", ext.Classification, "warranty")
			}
			if ext.Brand == nil || *ext.Brand != "LG" {
				t.Errorf("Brand = %v, want pointer to %q", ext.Brand, "LG")
			}
			if ext.Model == nil || *ext.Model != "FHP10" {
				t.Errorf("Model = %v, want pointer to %q", ext.Model, "FHP10")
			}
			if ext.SerialNumber == nil || *ext.SerialNumber != "SN001" {
				t.Errorf("SerialNumber = %v, want pointer to %q", ext.SerialNumber, "SN001")
			}
		})
	}
}

func TestParseExtraction_Classification(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		json string
		want string
	}{
		{"invoice kept", `{"classification":"invoice"}`, "invoice"},
		{"warranty kept", `{"classification":"warranty"}`, "warranty"},
		{"amc kept", `{"classification":"amc"}`, "amc"},
		{"other kept", `{"classification":"other"}`, "other"},
		{"unknown degrades to other", `{"classification":"totally_invalid_value_xyz"}`, "other"},
		{"empty string degrades to other", `{"classification":""}`, "other"},
		{"missing degrades to other", `{}`, "other"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ext, err := ParseExtraction(tt.json)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if ext.Classification != tt.want {
				t.Errorf("Classification = %q, want %q", ext.Classification, tt.want)
			}
		})
	}
}

func TestParseExtraction_Dates(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		json     string
		wantNil  bool
		wantDate time.Time
	}{
		{
			name:     "ISO 8601 date",
			json:     `{"purchase_date":"2023-11-03"}`,
			wantNil:  false,
			wantDate: time.Date(2023, time.November, 3, 0, 0, 0, 0, time.UTC),
		},
		{
			name:     "DD-MMM-YY date",
			json:     `{"purchase_date":"03-NOV-23"}`,
			wantNil:  false,
			wantDate: time.Date(2023, time.November, 3, 0, 0, 0, 0, time.UTC),
		},
		{
			name:     "DD-MMM-YYYY date",
			json:     `{"warranty_end":"15-Jan-2025"}`,
			wantNil:  false,
			wantDate: time.Date(2025, time.January, 15, 0, 0, 0, 0, time.UTC),
		},
		{
			name:    "garbage date → absent",
			json:    `{"purchase_date":"not-a-date"}`,
			wantNil: true,
		},
		{
			name:    "empty date → absent",
			json:    `{"purchase_date":""}`,
			wantNil: true,
		},
		{
			name:    "missing date → absent",
			json:    `{}`,
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ext, err := ParseExtraction(tt.json)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			var got *time.Time
			if strings.Contains(tt.json, "purchase_date") {
				got = ext.PurchaseDate
			} else if strings.Contains(tt.json, "warranty_end") {
				got = ext.WarrantyEnd
			} else {
				got = ext.PurchaseDate // missing → check PurchaseDate
			}
			if tt.wantNil {
				if got != nil {
					t.Errorf("date = %v, want nil", got)
				}
			} else {
				if got == nil {
					t.Error("date = nil, want non-nil")
				} else if !got.Equal(tt.wantDate) {
					t.Errorf("date = %v, want %v", got, tt.wantDate)
				}
			}
		})
	}
}

func TestParseExtraction_Price(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		json    string
		wantNil bool
		want    string
	}{
		{
			name: "price as string",
			json: `{"price":"39999.99"}`,
			want: "39999.99",
		},
		{
			name: "price as number (UseNumber preserves exact decimal)",
			json: `{"price":39999.99}`,
			want: "39999.99",
		},
		{
			name: "price as integer",
			json: `{"price":100}`,
			want: "100",
		},
		{
			name:    "missing price → nil",
			json:    `{}`,
			wantNil: true,
		},
		{
			name:    "null price → nil",
			json:    `{"price":null}`,
			wantNil: true,
		},
		{
			name:    "boolean price → nil (invalid type)",
			json:    `{"price":true}`,
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ext, err := ParseExtraction(tt.json)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantNil {
				if ext.Price != nil {
					t.Errorf("Price = %q, want nil", *ext.Price)
				}
			} else {
				if ext.Price == nil {
					t.Error("Price = nil, want non-nil")
				} else if *ext.Price != tt.want {
					t.Errorf("Price = %q, want %q", *ext.Price, tt.want)
				}
			}
		})
	}
}

func TestParseExtraction_Currency(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		json    string
		wantNil bool
		want    string
	}{
		{"valid INR", `{"currency":"INR"}`, false, "INR"},
		{"valid USD", `{"currency":"USD"}`, false, "USD"},
		{"valid lowercase", `{"currency":"inr"}`, false, "inr"},
		{"two letters → nil", `{"currency":"IN"}`, true, ""},
		{"four letters → nil", `{"currency":"INRX"}`, true, ""},
		{"empty → nil", `{"currency":""}`, true, ""},
		{"missing → nil", `{}`, true, ""},
		{"null → nil", `{"currency":null}`, true, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ext, err := ParseExtraction(tt.json)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantNil {
				if ext.Currency != nil {
					t.Errorf("Currency = %q, want nil", *ext.Currency)
				}
			} else {
				if ext.Currency == nil {
					t.Error("Currency = nil, want non-nil")
				} else if *ext.Currency != tt.want {
					t.Errorf("Currency = %q, want %q", *ext.Currency, tt.want)
				}
			}
		})
	}
}

func TestParseExtraction_Metadata(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		json    string
		wantLen int
		wantKey string
		wantVal any
	}{
		{
			name:    "multiple keys preserved",
			json:    `{"metadata":{"invoice_number":"INV-1","customer_name":"Jane","cgst_rate":"5.125"}}`,
			wantLen: 3,
		},
		{
			name:    "empty object → empty map",
			json:    `{"metadata":{}}`,
			wantLen: 0,
		},
		{
			name:    "null metadata → empty map",
			json:    `{"metadata":null}`,
			wantLen: 0,
		},
		{
			name:    "missing metadata → empty map",
			json:    `{}`,
			wantLen: 0,
		},
		{
			name:    "nested value preserved",
			json:    `{"metadata":{"address":{"city":"Delhi"}}}`,
			wantLen: 1,
			wantKey: "address",
		},
		{
			name:    "64-char key kept (max allowed)",
			json:    `{"metadata":{"` + strings.Repeat("a", 64) + `":"v"}}`,
			wantLen: 1,
			wantKey: strings.Repeat("a", 64),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ext, err := ParseExtraction(tt.json)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if ext.Metadata == nil {
				t.Error("Metadata = nil, want non-nil map")
				return
			}
			if len(ext.Metadata) != tt.wantLen {
				t.Errorf("Metadata len = %d, want %d", len(ext.Metadata), tt.wantLen)
			}
			if tt.wantKey != "" {
				if _, ok := ext.Metadata[tt.wantKey]; !ok {
					t.Errorf("Metadata missing key %q", tt.wantKey)
				}
			}
		})
	}
}

func TestParseExtraction_MetadataKeyValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		json        string
		wantKeys    map[string]any
		wantDropped []string
	}{
		{
			name: "spec scenario: mixed valid and invalid keys",
			json: `{"metadata":{"AMC Card No":"LG2401","amc_card_number":"LG2401","icr_number":"9982"}}`,
			wantKeys: map[string]any{
				"amc_card_number": "LG2401",
				"icr_number":      "9982",
			},
			wantDropped: []string{"AMC Card No"},
		},
		{
			name: "hyphen, uppercase, leading digit, 65-char key all dropped",
			json: `{"metadata":{"bad-key":"1","UPPER":"2","1leading":"3","` + strings.Repeat("a", 65) + `":"4","valid_key":"5"}}`,
			wantKeys: map[string]any{
				"valid_key": "5",
			},
			wantDropped: []string{"bad-key", "UPPER", "1leading", strings.Repeat("a", 65)},
		},
		{
			name:        "all-invalid metadata → empty non-nil map",
			json:        `{"metadata":{"AMC Card No":"LG2401","bad-key":"1"}}`,
			wantKeys:    map[string]any{},
			wantDropped: []string{"AMC Card No", "bad-key"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ext, err := ParseExtraction(tt.json)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if ext.Metadata == nil {
				t.Fatal("Metadata = nil, want non-nil map")
			}
			if len(ext.Metadata) != len(tt.wantKeys) {
				t.Fatalf("Metadata len = %d, want %d (got %v)", len(ext.Metadata), len(tt.wantKeys), ext.Metadata)
			}
			for k, v := range tt.wantKeys {
				if got, ok := ext.Metadata[k]; !ok {
					t.Errorf("Metadata missing valid key %q", k)
				} else if got != v {
					t.Errorf("Metadata[%q] = %v, want %v", k, got, v)
				}
			}
			for _, k := range tt.wantDropped {
				if _, ok := ext.Metadata[k]; ok {
					t.Errorf("Metadata contains dropped key %q", k)
				}
			}
		})
	}
}

func TestParseExtraction_MetadataNullValues(t *testing.T) {
	t.Parallel()

	ext, err := ParseExtraction(`{"metadata":{"good_key":null,"keep":"v"}}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ext.Metadata == nil {
		t.Fatal("Metadata = nil, want non-nil map")
	}
	if len(ext.Metadata) != 1 {
		t.Fatalf("Metadata len = %d, want 1 (got %v)", len(ext.Metadata), ext.Metadata)
	}
	if got, ok := ext.Metadata["keep"]; !ok {
		t.Error(`Metadata missing key "keep"`)
	} else if got != "v" {
		t.Errorf(`Metadata["keep"] = %v, want "v"`, got)
	}
	if _, ok := ext.Metadata["good_key"]; ok {
		t.Error(`Metadata contains null-valued key "good_key"`)
	}
}

func TestParseExtraction_MetadataOversize(t *testing.T) {
	t.Parallel()

	raw := `{"metadata":{"long_key":"` + strings.Repeat("x", 9000) + `"}}`
	_, err := ParseExtraction(raw)
	if err == nil {
		t.Error("expected error for metadata over 8 KiB, got nil")
	}
}

func TestParseExtraction_RawPayload(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
	}{
		{
			name: "no thoughts",
			raw:  `{"classification":"invoice","brand":"LG"}`,
		},
		{
			name: "with thought blocks",
			raw:  `<thought>thinking here</thought>{"classification":"invoice","brand":"LG"}<thought>more</thought>`,
		},
		{
			name: "whitespace around",
			raw:  "  \n <thought>think</thought> {\"classification\":\"amc\"} \n ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ext, err := ParseExtraction(tt.raw)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if ext.RawPayload != tt.raw {
				t.Errorf("RawPayload = %q, want exact original %q", ext.RawPayload, tt.raw)
			}
		})
	}
}

func TestParseExtraction_Errors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		raw     string
		wantErr string // substring to look for in error
	}{
		{"empty string", "", ""},
		{"whitespace only", "   \n\t  ", ""},
		{"no JSON object", "hello world", ""},
		{"JSON array empty", `[]`, ""},
		// NOTE: "JSON array with object" moved to
		// TestParseExtraction_ArrayWrap — a single-element array wrap is now
		// accepted (live-provider regression, 2026-09-11).
		{"invalid JSON", `{"a":`, ""},
		{"trailing garbage after object", `{"a":"b"} trailing`, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := ParseExtraction(tt.raw)
			if err == nil {
				t.Errorf("expected error for input %q, got nil", tt.raw)
			}
		})
	}
}

// TestParseExtraction_ArrayWrap covers the live-provider regression observed on
// 2026-09-11: gemma via the Gemini OpenAI-compatible endpoint intermittently
// wraps the extraction object in a JSON array (e.g. `[ { ... } ]`) despite the
// json_object response format. Both consensus workers then hit the parser's
// "trailing data after JSON object" guard at the same time, producing
// "all extraction workers failed". A single-element array wrap must parse to
// the inner object; multi-element arrays remain ambiguous and must still error.
func TestParseExtraction_ArrayWrap(t *testing.T) {
	t.Parallel()

	t.Run("single-element array wrap parses to inner object", func(t *testing.T) {
		// Shape captured verbatim from the live server log (indentation and
		// newlines included).
		raw := `[
  {
    "classification": "receipt",
    "brand": "Sony",
    "name": "headphone",
    "model": "WH-1000XM5",
    "asset_category": "electronics",
    "serial_number": "XM5-7788990011",
    "purchase_date": "2026-09-05",
    "warranty_end": null,
    "warranty_duration": null,
    "price": "348.00",
    "currency": "USD",
    "metadata": {
      "vendor": "Target"
    },
    "confidence": 1.0
  }
]`
		ext, err := ParseExtraction(raw)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ext.Classification != "receipt" {
			t.Errorf("Classification = %q, want %q", ext.Classification, "receipt")
		}
		if ext.Brand == nil || *ext.Brand != "Sony" {
			t.Errorf("Brand = %v, want pointer to %q", ext.Brand, "Sony")
		}
		if ext.Model == nil || *ext.Model != "WH-1000XM5" {
			t.Errorf("Model = %v, want pointer to %q", ext.Model, "WH-1000XM5")
		}
		if ext.SerialNumber == nil || *ext.SerialNumber != "XM5-7788990011" {
			t.Errorf("SerialNumber = %v, want pointer to %q", ext.SerialNumber, "XM5-7788990011")
		}
		if ext.RawPayload != raw {
			t.Errorf("RawPayload = %q, want original raw", ext.RawPayload)
		}
	})

	t.Run("compact single-element array wrap parses", func(t *testing.T) {
		ext, err := ParseExtraction(`[{"classification":"warranty","brand":"LG"}]`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ext.Classification != "warranty" {
			t.Errorf("Classification = %q, want %q", ext.Classification, "warranty")
		}
	})

	t.Run("thought tags around array wrap still parse", func(t *testing.T) {
		raw := `<thought>analyzing</thought>[{"brand":"LG"}]`
		ext, err := ParseExtraction(raw)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ext.Brand == nil || *ext.Brand != "LG" {
			t.Errorf("Brand = %v, want pointer to %q", ext.Brand, "LG")
		}
	})

	t.Run("multi-element array still errors", func(t *testing.T) {
		_, err := ParseExtraction(`[{"brand":"LG"},{"brand":"Samsung"}]`)
		if err == nil {
			t.Fatal("expected error for multi-element array, got nil")
		}
	})

	t.Run("empty array still errors", func(t *testing.T) {
		_, err := ParseExtraction(`[]`)
		if err == nil {
			t.Fatal("expected error for empty array, got nil")
		}
	})

	t.Run("bare object unchanged", func(t *testing.T) {
		ext, err := ParseExtraction(`{"classification":"invoice","brand":"LG"}`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ext.Classification != "invoice" {
			t.Errorf("Classification = %q, want %q", ext.Classification, "invoice")
		}
	})
}

func TestParseExtraction_NewFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		json         string
		wantName     string // "" means nil
		wantWarranty string
		wantCategory string // "" means nil
		wantBrand    string
	}{
		// Valid values
		{
			name:         "name valid",
			json:         `{"name":"Microwave Oven","brand":"LG"}`,
			wantName:     "Microwave Oven",
			wantWarranty: "",
			wantCategory: "",
			wantBrand:    "LG",
		},
		{
			name:         "warranty_duration valid",
			json:         `{"warranty_duration":"2 years","brand":"LG"}`,
			wantName:     "",
			wantWarranty: "2 years",
			wantCategory: "",
			wantBrand:    "LG",
		},
		{
			name:         "asset_category valid appliance",
			json:         `{"asset_category":"appliance","brand":"LG"}`,
			wantName:     "",
			wantWarranty: "",
			wantCategory: "appliance",
			wantBrand:    "LG",
		},

		// Missing
		{
			name:         "all missing",
			json:         `{"brand":"LG"}`,
			wantName:     "",
			wantWarranty: "",
			wantCategory: "",
			wantBrand:    "LG",
		},

		// Malformed type (number/null/empty string)
		{
			name:         "name number → nil",
			json:         `{"name":123,"brand":"LG"}`,
			wantName:     "",
			wantWarranty: "",
			wantCategory: "",
			wantBrand:    "LG",
		},
		{
			name:         "name null → nil",
			json:         `{"name":null,"brand":"LG"}`,
			wantName:     "",
			wantWarranty: "",
			wantCategory: "",
			wantBrand:    "LG",
		},
		{
			name:         "name empty string → nil",
			json:         `{"name":"","brand":"LG"}`,
			wantName:     "",
			wantWarranty: "",
			wantCategory: "",
			wantBrand:    "LG",
		},
		{
			name:         "warranty_duration number → absent",
			json:         `{"warranty_duration":123,"brand":"LG"}`,
			wantName:     "",
			wantWarranty: "",
			wantCategory: "",
			wantBrand:    "LG",
		},
		{
			name:         "warranty_duration null → absent",
			json:         `{"warranty_duration":null,"brand":"LG"}`,
			wantName:     "",
			wantWarranty: "",
			wantCategory: "",
			wantBrand:    "LG",
		},
		{
			name:         "warranty_duration empty string → absent",
			json:         `{"warranty_duration":"","brand":"LG"}`,
			wantName:     "",
			wantWarranty: "",
			wantCategory: "",
			wantBrand:    "LG",
		},
		{
			name:         "asset_category number → nil",
			json:         `{"asset_category":123,"brand":"LG"}`,
			wantName:     "",
			wantWarranty: "",
			wantCategory: "",
			wantBrand:    "LG",
		},
		{
			name:         "asset_category null → nil",
			json:         `{"asset_category":null,"brand":"LG"}`,
			wantName:     "",
			wantWarranty: "",
			wantCategory: "",
			wantBrand:    "LG",
		},
		{
			name:         "asset_category valid furniture",
			json:         `{"asset_category":"furniture","brand":"LG"}`,
			wantName:     "",
			wantWarranty: "",
			wantCategory: "furniture",
			wantBrand:    "LG",
		},
		{
			name:         "asset_category unknown → nil",
			json:         `{"asset_category":"unknown_cat","brand":"LG"}`,
			wantName:     "",
			wantWarranty: "",
			wantCategory: "",
			wantBrand:    "LG",
		},
		{
			name:         "asset_category case-sensitive → nil",
			json:         `{"asset_category":"APPLIANCE","brand":"LG"}`,
			wantName:     "",
			wantWarranty: "",
			wantCategory: "",
			wantBrand:    "LG",
		},

		// All three present with other fields
		{
			name:         "all three present with other fields",
			json:         `{"name":"Cabinet","warranty_duration":"24 months","asset_category":"furniture","brand":"Cooler Master","model":"CD600"}`,
			wantName:     "Cabinet",
			wantWarranty: "24 months",
			wantCategory: "furniture",
			wantBrand:    "Cooler Master",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ext, err := ParseExtraction(tt.json)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.wantName == "" {
				if ext.Name != nil {
					t.Errorf("Name = %q, want nil", *ext.Name)
				}
			} else {
				if ext.Name == nil || *ext.Name != tt.wantName {
					t.Errorf("Name = %v, want %q", ext.Name, tt.wantName)
				}
			}

			if ext.WarrantyDuration != tt.wantWarranty {
				t.Errorf("WarrantyDuration = %q, want %q", ext.WarrantyDuration, tt.wantWarranty)
			}

			if tt.wantCategory == "" {
				if ext.AssetCategory != nil {
					t.Errorf("AssetCategory = %q, want nil", *ext.AssetCategory)
				}
			} else {
				if ext.AssetCategory == nil || *ext.AssetCategory != tt.wantCategory {
					t.Errorf("AssetCategory = %v, want %q", ext.AssetCategory, tt.wantCategory)
				}
			}

			if tt.wantBrand != "" {
				if ext.Brand == nil || *ext.Brand != tt.wantBrand {
					t.Errorf("Brand = %v, want %q", ext.Brand, tt.wantBrand)
				}
			}
		})
	}
}
