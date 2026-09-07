package processing

import (
	"testing"

	"procrastinator-backend/commons/entity"
)

// ptr returns a pointer to the given string.
func ptr(s string) *string {
	return &s
}

func TestInferCategory(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		nameIn    string
		brandIn   string
		descIn    string
		llmCat    *string
		wantCat   string
		wantConf  float64
	}{
		{
			name:     "name-only appliance, no LLM",
			nameIn:   "Microwave",
			wantCat:  entity.AssetCategoryAppliance,
			wantConf: 0.6,
		},
		{
			name:     "brand vs name ambiguity, either source",
			nameIn:   "Cabinet",
			brandIn:  "Cooler Master",
			wantCat:  entity.AssetCategoryComputing,
			wantConf: 0.6,
		},
		{
			name:     "llm and classifier agree",
			nameIn:   "Microwave",
			llmCat:   ptr(entity.AssetCategoryAppliance),
			wantCat:  entity.AssetCategoryAppliance,
			wantConf: 0.9,
		},
		{
			name:     "no signal at all",
			wantCat:  entity.AssetCategoryOther,
			wantConf: 0.5,
		},
		{
			name:     "llm only, classifier no signal",
			llmCat:   ptr(entity.AssetCategoryFurniture),
			wantCat:  entity.AssetCategoryFurniture,
			wantConf: 0.6,
		},
		{
			name:     "llm out of vocabulary ignored, classifier decides",
			nameIn:   "Microwave",
			llmCat:   ptr("furniture_typo"),
			wantCat:  entity.AssetCategoryAppliance,
			wantConf: 0.6,
		},
		{
			name:     "llm disagrees with classifier, llm preferred",
			nameIn:   "Microwave",
			llmCat:   ptr(entity.AssetCategoryFurniture),
			wantCat:  entity.AssetCategoryFurniture,
			wantConf: 0.6,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			gotCat, gotConf := InferCategory(tc.nameIn, tc.brandIn, tc.descIn, tc.llmCat)
			if gotCat != tc.wantCat {
				t.Fatalf("category = %q, want %q", gotCat, tc.wantCat)
			}
			if gotConf != tc.wantConf {
				t.Fatalf("confidence = %v, want %v", gotConf, tc.wantConf)
			}
		})
	}
}

// TestInferCategoryAmbiguousWordBoundary ensures single-word keywords match
// only at word boundaries (e.g. "saw" must not match inside "newsaw").
func TestInferCategoryAmbiguousWordBoundary(t *testing.T) {
	t.Parallel()

	// "saw" embedded mid-word in "newsaw" must NOT classify as tool.
	gotCat, _ := InferCategory("newsaw", "", "", nil)
	if gotCat == entity.AssetCategoryTool {
		t.Fatalf("saw matched inside newsaw; category = %q, want a non-tool category", gotCat)
	}

	// "saw" at a word boundary DOES classify as tool.
	gotCat, _ = InferCategory("Saw", "", "", nil)
	if gotCat != entity.AssetCategoryTool {
		t.Fatalf("category = %q, want %q", gotCat, entity.AssetCategoryTool)
	}

	// "cpu" at a word boundary DOES classify as computing.
	gotCat, _ = InferCategory("CPU", "", "", nil)
	if gotCat != entity.AssetCategoryComputing {
		t.Fatalf("category = %q, want %q", gotCat, entity.AssetCategoryComputing)
	}
}

// TestInferCategoryMultiWordKeyword ensures multi-word keywords match as
// plain substrings (no word-boundary restriction) while short ambiguous tokens
// do not leak in via single-word matches.
func TestInferCategoryMultiWordKeyword(t *testing.T) {
	t.Parallel()

	// "washing machine" is a multi-word appliance keyword.
	gotCat, _ := InferCategory("Washing Machine", "", "", nil)
	if gotCat != entity.AssetCategoryAppliance {
		t.Fatalf("category = %q, want %q", gotCat, entity.AssetCategoryAppliance)
	}

	// "air conditioner" as a substring in a longer description.
	gotCat, _ = InferCategory("", "", "1 ton air conditioner", nil)
	if gotCat != entity.AssetCategoryAppliance {
		t.Fatalf("category = %q, want %q", gotCat, entity.AssetCategoryAppliance)
	}
}
