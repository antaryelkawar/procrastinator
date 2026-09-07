package processing

import (
	"strings"

	"procrastinator-backend/commons/entity"
)

// categoryKeyword pairs an asset category with the list of lowercase keywords
// that trigger a classification into that category. The order of this slice is
// significant: InferCategory returns the first matching category.
type categoryKeyword struct {
	category   string
	keywords   []string
}

// categoryKeywords is the ordered keyword classifier table. Categories are
// listed in descending preference; the first entry with any keyword match wins.
var categoryKeywords = []categoryKeyword{
	{entity.AssetCategoryComputing, []string{
		"cooler master", "tower", "cpu", "motherboard", "graphics card",
		"gpu", "laptop", "desktop", "monitor", "printer", "router",
	}},
	{entity.AssetCategoryAppliance, []string{
		"microwave", "fridge", "refrigerator", "stove", "oven", "washer",
		"washing machine", "dishwasher", "air conditioner", "kettle", "toaster",
		"induction", "cooktop", "dryer",
	}},
	{entity.AssetCategoryFurniture, []string{
		"cabinet", "shelf", "table", "chair", "sofa", "bed", "wardrobe", "desk",
	}},
	{entity.AssetCategoryVehicle, []string{
		"car", "bicycle", "scooter", "motorcycle", "vehicle",
	}},
	{entity.AssetCategoryTool, []string{
		"drill", "saw", "wrench", "hammer", "sander", "toolkit",
	}},
	{entity.AssetCategoryClothing, []string{
		"shirt", "shoes", "jacket", "sweater", "dress", "trousers", "clothing",
	}},
	{entity.AssetCategoryElectronics, []string{
		"television", "speaker", "headphones", "camera", "soundbar", "smartwatch",
	}},
}

// InferCategory infers the intrinsic asset category for an item and a
// confidence in that inference.
//
// Priority: the LLM's in-vocabulary asset_category (llmCategory) is preferred;
// when it is absent or out of vocabulary, a deterministic keyword classifier
// over name/brand/description decides. Confidence: 0.9 when the LLM category
// and the classifier agree, 0.6 when only a single source (LLM or classifier)
// provides a category, 0.5 when neither does (result is "other").
func InferCategory(name, brand string, desc string, llmCategory *string) (category string, confidence float64) {
	// LLM signal is only present when the pointer is non-nil AND the value is
	// in the asset-category vocabulary (case-sensitive).
	var llmHasSignal bool
	llmValue := ""
	if llmCategory != nil && entity.ValidAssetCategory(*llmCategory) {
		llmHasSignal = true
		llmValue = *llmCategory
	}

	// Classifier signal: a deterministic keyword match over name/brand/description.
	categoryText := strings.ToLower(
		strings.Join([]string{name, brand, desc}, " "),
	)
	categoryText = strings.Join(strings.Fields(categoryText), " ")
	classifierCat, classifierMatch := classifyCategory(categoryText)

	switch {
	case llmHasSignal && classifierMatch && llmValue == classifierCat:
		return llmValue, 0.9
	case llmHasSignal:
		// LLM preferred even when it disagrees with the classifier.
		return llmValue, 0.6
	case classifierMatch:
		return classifierCat, 0.6
	default:
		return entity.AssetCategoryOther, 0.5
	}
}

// classifyCategory runs the deterministic keyword classifier over combined
// (lowercased, whitespace-normalized) text and returns the first matching
// category plus whether any keyword matched. When nothing matches it returns
// "" and false. Single-word keywords match only at a word boundary;
// multi-word keywords match as plain substrings.
func classifyCategory(lowerText string) (string, bool) {
	for _, entry := range categoryKeywords {
		for _, kw := range entry.keywords {
			if strings.Contains(kw, " ") {
				if strings.Contains(lowerText, kw) {
					return entry.category, true
				}
				continue
			}
			if brandAtWordBoundary(lowerText, kw) {
				return entry.category, true
			}
		}
	}
	return "", false
}
