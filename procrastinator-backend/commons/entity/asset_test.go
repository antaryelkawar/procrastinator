package entity

import "testing"

func TestValidAssetCategory(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   string
		want bool
	}{
		{"appliance", "appliance", true},
		{"electronics", "electronics", true},
		{"computing", "computing", true},
		{"furniture", "furniture", true},
		{"vehicle", "vehicle", true},
		{"tool", "tool", true},
		{"clothing", "clothing", true},
		{"document_only", "document_only", true},
		{"other", "other", true},
		{"empty", "", false},
		{"unknown", "unknown", false},
		{"Appliance-capitalized", "Appliance", false},
		{"APPLIANCE-uppercase", "APPLIANCE", false},
		{"trailing-space", "appliance ", false},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := ValidAssetCategory(tc.in)
			if got != tc.want {
				t.Fatalf("ValidAssetCategory(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestAssetCategoryConstantValues(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		got  string
		want string
	}{
		{"AssetCategoryAppliance", AssetCategoryAppliance, "appliance"},
		{"AssetCategoryElectronics", AssetCategoryElectronics, "electronics"},
		{"AssetCategoryComputing", AssetCategoryComputing, "computing"},
		{"AssetCategoryFurniture", AssetCategoryFurniture, "furniture"},
		{"AssetCategoryVehicle", AssetCategoryVehicle, "vehicle"},
		{"AssetCategoryTool", AssetCategoryTool, "tool"},
		{"AssetCategoryClothing", AssetCategoryClothing, "clothing"},
		{"AssetCategoryDocumentOnly", AssetCategoryDocumentOnly, "document_only"},
		{"AssetCategoryOther", AssetCategoryOther, "other"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if tc.got != tc.want {
				t.Fatalf("constant %s = %q, want %q", tc.name, tc.got, tc.want)
			}
		})
	}
}
