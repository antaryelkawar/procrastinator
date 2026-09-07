package processing

import (
	"strings"

	"procrastinator-backend/commons"
)

// descriptorWords is the set of lowercase feature/color descriptor words that
// are excluded from the product name. These are common modifiers found in
// product descriptions (colors, features) that do not form part of the
// canonical product name.
var descriptorWords = map[string]bool{
	// Colors
	"black": true, "white": true, "red": true, "blue": true,
	"green": true, "gray": true, "grey": true, "silver": true,
	"gold": true, "brown": true, "beige": true, "cream": true,
	"maroon": true, "navy": true, "ivory": true,
	// Feature/technology descriptors
	"convection": true, "digital": true, "smart": true,
	"wired": true, "wireless": true, "portable": true,
	"compact": true, "full": true, "mini": true,
}

// compounds maps adjacent lowercase token pairs to their joined form.
// This normalizes well-known product compound words that appear as
// separate tokens in raw descriptions.
var compounds = map[[2]string]string{
	{"micro", "wave"}: "microwave",
}

// SplitDescription splits a product description string into brand, name, and
// model. It collapses whitespace, extracts a known brand (via lexicon),
// identifies a trailing model-like token (containing at least one digit),
// removes descriptor words, joins known compounds, title-cases the result,
// and returns the remaining tokens as the product name.
//
// The full raw string is never returned as model. If no model-like token
// (containing a digit) is found, model is empty and all non-brand,
// non-descriptor tokens form the name.
func SplitDescription(desc string, lex *BrandLexicon) (brand, name, model string) {
	// Step 1: Collapse whitespace.
	collapsed := commons.CollapseWhitespace(desc)
	if collapsed == "" {
		return "", "", ""
	}

	// Step 2: Tokenize.
	tokens := strings.Fields(collapsed)

	// Step 3: Find brand via lexicon (search in the collapsed text).
	var brandTokens []int
	if lex != nil {
		brandName, found := lex.FindBrand(collapsed)
		if found {
			brand = brandName
			brandTokens = findBrandTokenIndices(tokens, brandName)
		}
	}

	// Step 4: Remove brand tokens from the token list.
	remaining := removeIndices(tokens, brandTokens)

	// Step 5: Find the model token — the last token (from the end) that
	// contains at least one digit.
	modelIdx := -1
	for i := len(remaining) - 1; i >= 0; i-- {
		if containsDigit(remaining[i]) {
			modelIdx = i
			break
		}
	}

	// Step 6: Extract model and remove it. The remaining tokens (before AND
	// after the model position) form the name candidate. Descriptor words
	// (colors, features) are then filtered out in the next step.
	if modelIdx >= 0 {
		model = remaining[modelIdx]
		// Remove only the model token; keep tokens before and after it.
		remaining = append(remaining[:modelIdx], remaining[modelIdx+1:]...)
	}

	// Step 7: Remove descriptor words (colors, features) from remaining.
	remaining = removeDescriptors(remaining)

	// Step 8: Join known compounds (e.g., "micro" + "wave" → "microwave").
	remaining = joinCompounds(remaining)

	// Step 9: Title-case the remaining tokens to form the name.
	name = titleCaseTokens(remaining)

	return brand, name, model
}

// findBrandTokenIndices finds the indices in tokens that match the brand name
// (case-insensitive). It handles multi-word brands by finding the contiguous
// sequence of tokens that together form the brand.
func findBrandTokenIndices(tokens []string, brand string) []int {
	lowerBrand := strings.ToLower(brand)
	brandWords := strings.Fields(lowerBrand)
	brandLen := len(brandWords)
	if brandLen == 0 || len(tokens) < brandLen {
		return nil
	}

	// Find the starting position where the contiguous sequence matches.
	lowerTokens := make([]string, len(tokens))
	for i, t := range tokens {
		lowerTokens[i] = strings.ToLower(t)
	}

	for i := 0; i+brandLen <= len(tokens); i++ {
		match := true
		for j := 0; j < brandLen; j++ {
			if lowerTokens[i+j] != brandWords[j] {
				match = false
				break
			}
		}
		if match {
			indices := make([]int, brandLen)
			for j := 0; j < brandLen; j++ {
				indices[j] = i + j
			}
			return indices
		}
	}
	return nil
}

// removeIndices returns a new slice with elements at the given indices removed.
func removeIndices(tokens []string, indices []int) []string {
	if len(indices) == 0 {
		return tokens
	}
	removeSet := make(map[int]bool, len(indices))
	for _, i := range indices {
		removeSet[i] = true
	}
	result := make([]string, 0, len(tokens)-len(indices))
	for i, t := range tokens {
		if !removeSet[i] {
			result = append(result, t)
		}
	}
	return result
}

// containsDigit reports whether s contains at least one ASCII digit.
func containsDigit(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= '0' && s[i] <= '9' {
			return true
		}
	}
	return false
}

// removeDescriptors filters out tokens whose lowercase form is in the
// descriptorWords set.
func removeDescriptors(tokens []string) []string {
	result := make([]string, 0, len(tokens))
	for _, t := range tokens {
		if !descriptorWords[strings.ToLower(t)] {
			result = append(result, t)
		}
	}
	return result
}

// joinCompounds merges adjacent token pairs that form known compound words.
// For example, ["MICRO", "WAVE", "OVEN"] → ["MICROWAVE", "OVEN"].
func joinCompounds(tokens []string) []string {
	if len(tokens) < 2 {
		return tokens
	}
	result := make([]string, 0, len(tokens))
	i := 0
	for i < len(tokens) {
		if i+1 < len(tokens) {
			key := [2]string{strings.ToLower(tokens[i]), strings.ToLower(tokens[i+1])}
			if joined, ok := compounds[key]; ok {
				// Preserve uppercase form of the joined compound.
				result = append(result, strings.ToUpper(joined))
				i += 2
				continue
			}
		}
		result = append(result, tokens[i])
		i++
	}
	return result
}

// titleCaseTokens title-cases each token: capitalize the first letter,
// lowercase the rest. Joins tokens with a single space.
func titleCaseTokens(tokens []string) string {
	if len(tokens) == 0 {
		return ""
	}
	parts := make([]string, len(tokens))
	for i, t := range tokens {
		parts[i] = titleCaseWord(t)
	}
	return strings.Join(parts, " ")
}

// titleCaseWord capitalizes the first letter and lowercases the rest of s.
// Words that are entirely uppercase (acronyms like "TV", "AC") and have
// 3 or fewer letters are preserved as-is.
func titleCaseWord(s string) string {
	if s == "" {
		return ""
	}
	// Preserve short all-caps acronyms (TV, AC, LG, etc.)
	if len(s) <= 3 && isAllCaps(s) {
		return s
	}
	runes := []rune(s)
	runes[0] = unicodeToUpper(runes[0])
	for i := 1; i < len(runes); i++ {
		runes[i] = unicodeToLower(runes[i])
	}
	return string(runes)
}

// isAllCaps reports whether s is entirely uppercase ASCII letters.
func isAllCaps(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < 'A' || s[i] > 'Z' {
			return false
		}
	}
	return len(s) > 0
}

// unicodeToUpper converts a rune to its uppercase form.
func unicodeToUpper(r rune) rune {
	if r >= 'a' && r <= 'z' {
		return r - 'a' + 'A'
	}
	return r
}

// unicodeToLower converts a rune to its lowercase form.
func unicodeToLower(r rune) rune {
	if r >= 'A' && r <= 'Z' {
		return r - 'A' + 'a'
	}
	return r
}
