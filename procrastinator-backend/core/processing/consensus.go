package processing

import (
	"strings"
	"time"

	"procrastinator-backend/commons/entity"
)

// Consensus confidence levels (D2): s>=2 workers agreeing on all identity
// fields → high; exactly one successful worker → single-worker cap; any
// identity-field conflict → conflict cap. Written to Extraction.Confidence
// so the existing auto-commit gate (confidence >= threshold) is reused.
const (
	ConfidenceHigh     = 0.9 // s >= 2, all identity fields agree
	ConfidenceSingle   = 0.6 // exactly one successful worker
	ConfidenceConflict = 0.5 // any identity-field conflict
)

// defaultConsensusLexicon is the brand lexicon consulted by Consensus to
// validate/correct extracted brands (D2). It is the embedded canonical
// list; lexicon file overrides are a composition/config concern, not a
// consensus concern.
var defaultConsensusLexicon = NewBrandLexicon()

// Consensus combines per-worker extraction results into one canonical
// extraction, a consensus confidence in [0.0, 1.0], and the names of the
// unresolved identity fields (JSON field names: "serial_number", "brand",
// "model").
//
// Let s be the number of successful (Err == nil) workers:
//   - s == 0 → zero Extraction, confidence 0, no unresolved fields.
//   - s == 1 → that worker's (reconciled) extraction, confidence 0.6.
//   - s >= 2 → identity fields (serial_number, brand, model): if two or more
//     workers returned DIFFERENT NON-NIL values the field is unresolved
//     (nil) and its name is reported; any unresolved field caps confidence
//     at 0.5, otherwise confidence is 0.9. Non-identity fields (name,
//     dates, price, currency, category, classification, warranty duration,
//     metadata, raw payload) take the first non-nil value in fixed worker
//     order; disagreement on them does not affect confidence.
//
// Before comparison, every successful worker's extraction is reconciled:
// brands are validated/corrected to their canonical lexicon spelling and
// combined product descriptions are re-split into brand/name/model.
//
// The consensus confidence is written to the returned Extraction's
// Confidence field (never nil for s >= 1).
func Consensus(workers []WorkerResult) (entity.Extraction, float64, []string) {
	// Step 1: filter to successful workers, preserving input order.
	successful := make([]entity.Extraction, 0, len(workers))
	for _, wr := range workers {
		if wr.Err == nil {
			successful = append(successful, wr.Extraction)
		}
	}

	// Step 2: s == 0 → zero result, no canonical confidence write.
	if len(successful) == 0 {
		return entity.Extraction{}, 0, nil
	}

	// Step 3: reconcile each extraction before any comparison.
	for i := range successful {
		successful[i] = reconcileIdentity(successful[i], defaultConsensusLexicon)
	}

	// Step 4: s == 1 → single-worker cap.
	if len(successful) == 1 {
		ext := successful[0]
		ext.Confidence = f64p(ConfidenceSingle)
		return ext, ConfidenceSingle, nil
	}

	// Step 5: s >= 2 → reconcile identity fields, then non-identity fields.
	var unresolved []string

	ext := entity.Extraction{
		Metadata: map[string]any{},
	}

	ext.SerialNumber = identityValue("serial_number", func(e entity.Extraction) *string { return e.SerialNumber }, successful, &unresolved)
	ext.Brand = identityValue("brand", func(e entity.Extraction) *string { return e.Brand }, successful, &unresolved)
	ext.Model = identityValue("model", func(e entity.Extraction) *string { return e.Model }, successful, &unresolved)

	// Non-identity: first non-nil value wins per field.
	ext.PurchaseDate = firstNonTimePtr(func(e entity.Extraction) *time.Time { return e.PurchaseDate }, successful)
	ext.WarrantyEnd = firstNonTimePtr(func(e entity.Extraction) *time.Time { return e.WarrantyEnd }, successful)
	ext.Price = firstNonNull(func(e entity.Extraction) *string { return e.Price }, successful)
	ext.Currency = firstNonNull(func(e entity.Extraction) *string { return e.Currency }, successful)
	ext.Name = firstNonNull(func(e entity.Extraction) *string { return e.Name }, successful)
	ext.AssetCategory = firstNonNull(func(e entity.Extraction) *string { return e.AssetCategory }, successful)

	// Non-identity: first non-empty value wins per field.
	ext.Classification = firstNonEmptyString(func(e entity.Extraction) string { return e.Classification }, successful)
	ext.WarrantyDuration = firstNonEmptyString(func(e entity.Extraction) string { return e.WarrantyDuration }, successful)
	ext.RawPayload = firstNonEmptyString(func(e entity.Extraction) string { return e.RawPayload }, successful)
	for _, e := range successful {
		if e.Metadata != nil && len(e.Metadata) > 0 {
			ext.Metadata = e.Metadata
			break
		}
	}

	conf := ConfidenceHigh
	if len(unresolved) > 0 {
		conf = ConfidenceConflict
	}
	ext.Confidence = f64p(conf)
	return ext, conf, unresolved
}

// identityValue resolves one identity field across workers in fixed order:
// the first non-nil value is the agreed value; a LATER non-nil value that
// differs marks the field unresolved (nil) and appends name to *unresolved.
// A worker's nil is not a conflict — a single value still wins.
func identityValue(name string, get func(entity.Extraction) *string, norm []entity.Extraction, unresolved *[]string) *string {
	var agreed *string
	for _, e := range norm {
		v := get(e)
		if v == nil {
			continue
		}
		if agreed == nil {
			// Preserve the pointer of the first non-nil value.
			agreed = v
			continue
		}
		if *agreed != *v {
			// A differing later value → conflict; field is unresolved.
			*unresolved = append(*unresolved, name)
			return nil
		}
	}
	return agreed
}

// firstNonNull returns the first non-nil *string across norm, or nil.
func firstNonNull(get func(entity.Extraction) *string, norm []entity.Extraction) *string {
	for _, e := range norm {
		if v := get(e); v != nil {
			return v
		}
	}
	return nil
}

// firstNonTimePtr returns the first non-nil value of get across norm, or nil.
func firstNonTimePtr(get func(entity.Extraction) *time.Time, norm []entity.Extraction) *time.Time {
	for _, e := range norm {
		if v := get(e); v != nil {
			return v
		}
	}
	return nil
}

// firstNonEmptyString returns the first non-empty value across norm, or "".
func firstNonEmptyString(get func(entity.Extraction) string, norm []entity.Extraction) string {
	for _, e := range norm {
		if v := get(e); v != "" {
			return v
		}
	}
	return ""
}

// reconcileIdentity reconciles a COPY of ext: it re-splits combined
// descriptions stored wholesale in model/name (the spec-forbidden case),
// pulls brand/model/name out of them, and corrects known brands to their
// canonical lexicon spelling. It never mutates the input's pointed-to memory.
func reconcileIdentity(ext entity.Extraction, lex *BrandLexicon) entity.Extraction {
	out := ext

	// Dereference to plain strings for manipulation.
	brand := pointerString(out.Brand)
	name := pointerString(out.Name)
	model := pointerString(out.Model)

	switch {
	case model != "" && strings.Contains(model, " "):
		// Combined description stored wholesale in model.
		b, n, m := SplitDescription(model, lex)
		if m != "" {
			model = m
		}
		if n != "" && name == "" {
			name = n
		}
		if b != "" && brand == "" {
			brand = b
		}

	case model == "" && name != "" && strings.Contains(name, " ") && containsDigit(name):
		// Combined description sitting in name with no model at all.
		b, n, m := SplitDescription(name, lex)
		name = n
		if m != "" {
			model = m
		}
		if b != "" && brand == "" {
			brand = b
		}
	}

	if brand != "" {
		brand = correctBrand(brand, lex)
	}

	// Rebuild pointer fields: non-empty → pointer, empty → nil.
	// SerialNumber passes through unchanged.
	out.Brand = nonEmptyPtr(brand)
	out.Name = nonEmptyPtr(name)
	out.Model = nonEmptyPtr(model)
	return out
}

// correctBrand returns the canonical lexicon spelling if brand names a known
// brand within it; otherwise brand is returned UNCHANGED (unknown brands are
// preserved — the consensus conflict rule handles disagreement).
func correctBrand(brand string, lex *BrandLexicon) string {
	if canonical, ok := lex.FindBrand(brand); ok {
		return canonical
	}
	return brand
}

// nonEmptyPtr returns a pointer to s if non-empty, else nil.
func nonEmptyPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// pointerString dereferences p to a string; nil pointer yields "".
func pointerString(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// f64p returns a pointer to v.
func f64p(v float64) *float64 { return &v }
