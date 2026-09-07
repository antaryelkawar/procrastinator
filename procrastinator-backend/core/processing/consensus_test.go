package processing

import (
	"errors"
	"testing"
	"time"

	"procrastinator-backend/commons/entity"
)

// assertZeroExtraction asserts ext is the zero Extraction (all pointers nil,
// all string fields empty) — used where struct literal comparison is not
// allowed because Extraction contains a map field.
func assertZeroExtraction(t *testing.T, ext entity.Extraction) {
	t.Helper()
	if ext.Brand != nil || ext.Model != nil || ext.SerialNumber != nil ||
		ext.PurchaseDate != nil || ext.WarrantyEnd != nil ||
		ext.Price != nil || ext.Currency != nil || ext.Name != nil ||
		ext.AssetCategory != nil ||
		ext.Classification != "" || ext.WarrantyDuration != "" || ext.RawPayload != "" {
		t.Errorf("ext = %+v, want zero", ext)
	}
}

// sp returns a pointer to s, for building Extraction pointer fields in tests.
func sp(s string) *string { return &s }

// okResult wraps a successful Extraction as a WorkerResult.
func okResult(ext entity.Extraction) WorkerResult {
	return WorkerResult{Worker: Worker{Model: "m"}, Extraction: ext}
}

// failResult wraps a failure as a WorkerResult with a nil Extraction.
func failResult(err error) WorkerResult {
	return WorkerResult{Worker: Worker{Model: "m"}, Err: err}
}

// --- Verification-list scenarios ---

// TestConsensus_TwoWorkersAgree: two identical successful results agree on all
// identity fields → high confidence.
func TestConsensus_TwoWorkersAgree(t *testing.T) {
	t.Parallel()

	ext := entity.Extraction{
		Brand:        sp("LG"),
		Model:        sp("FHP10"),
		SerialNumber: sp("SN123"),
		Name:         sp("Washing Machine"),
	}
	workers := []WorkerResult{
		okResult(ext),
		okResult(entity.Extraction{
			Brand:        ext.Brand,
			Model:        ext.Model,
			SerialNumber: ext.SerialNumber,
			Name:         ext.Name,
		}),
	}

	got, conf, unresolved := Consensus(workers)

	if conf != 0.9 {
		t.Fatalf("conf = %v, want 0.9", conf)
	}
	if len(unresolved) != 0 {
		t.Fatalf("unresolved = %v, want empty", unresolved)
	}
	if got.Confidence == nil {
		t.Fatal("ext.Confidence == nil, want non-nil")
	}
	if *got.Confidence != conf {
		t.Fatalf("ext.Confidence = %v, want %v", *got.Confidence, conf)
	}
	if got.Brand == nil || *got.Brand != "LG" {
		t.Errorf("Brand = %v, want LG", got.Brand)
	}
	if got.Model == nil || *got.Model != "FHP10" {
		t.Errorf("Model = %v, want FHP10", got.Model)
	}
	if got.SerialNumber == nil || *got.SerialNumber != "SN123" {
		t.Errorf("SerialNumber = %v, want SN123", got.SerialNumber)
	}
	if got.Name == nil || *got.Name != "Washing Machine" {
		t.Errorf("Name = %v, want Washing Machine", got.Name)
	}
}

// TestConsensus_OneWorkerFails: one success + one failure → single-worker cap.
func TestConsensus_OneWorkerFails(t *testing.T) {
	t.Parallel()

	ext := entity.Extraction{
		Brand:        sp("LG"),
		Model:        sp("FHP10"),
		SerialNumber: sp("SN123"),
		Name:         sp("Washing Machine"),
	}
	workers := []WorkerResult{
		okResult(ext),
		failResult(errors.New("timeout")),
	}

	got, conf, unresolved := Consensus(workers)

	if conf != 0.6 {
		t.Fatalf("conf = %v, want 0.6", conf)
	}
	if len(unresolved) != 0 {
		t.Fatalf("unresolved = %v, want empty", unresolved)
	}
	if got.Brand == nil || *got.Brand != "LG" {
		t.Errorf("Brand = %v, want LG", got.Brand)
	}
	if got.Model == nil || *got.Model != "FHP10" {
		t.Errorf("Model = %v, want FHP10", got.Model)
	}
	if got.SerialNumber == nil || *got.SerialNumber != "SN123" {
		t.Errorf("SerialNumber = %v, want SN123", got.SerialNumber)
	}
	if got.Confidence == nil || *got.Confidence != 0.6 {
		t.Errorf("Confidence = %v, want 0.6", got.Confidence)
	}
}

// TestConsensus_SerialDisagree: serial conflict → unresolved, capped.
func TestConsensus_SerialDisagree(t *testing.T) {
	t.Parallel()

	workers := []WorkerResult{
		okResult(entity.Extraction{Brand: sp("LG"), Model: sp("FHP10"), SerialNumber: sp("SN123")}),
		okResult(entity.Extraction{Brand: sp("LG"), Model: sp("FHP10"), SerialNumber: sp("SN128")}),
	}

	got, conf, unresolved := Consensus(workers)

	if conf != 0.5 {
		t.Fatalf("conf = %v, want 0.5", conf)
	}
	if got.SerialNumber != nil {
		t.Errorf("SerialNumber = %v, want nil (conflict)", got.SerialNumber)
	}
	if len(unresolved) != 1 || unresolved[0] != "serial_number" {
		t.Fatalf("unresolved = %v, want [serial_number]", unresolved)
	}
}

// TestConsensus_AllFail_NoCanonical: all failures (and empty slice) → zero.
func TestConsensus_AllFail_NoCanonical(t *testing.T) {
	t.Parallel()

	t.Run("two failures", func(t *testing.T) {
		t.Parallel()
		workers := []WorkerResult{
			failResult(errors.New("a")),
			failResult(errors.New("b")),
		}
		got, conf, unresolved := Consensus(workers)
		assertZeroExtraction(t, got)
		if conf != 0 {
			t.Errorf("conf = %v, want 0", conf)
		}
		if unresolved != nil {
			t.Errorf("unresolved = %v, want nil", unresolved)
		}
	})

	t.Run("empty slice", func(t *testing.T) {
		t.Parallel()
		got, conf, unresolved := Consensus(nil)
		assertZeroExtraction(t, got)
		if conf != 0 {
			t.Errorf("conf = %v, want 0", conf)
		}
		if unresolved != nil {
			t.Errorf("unresolved = %v, want nil", unresolved)
		}
	})
}

// TestConsensus_NonIdentityDisagreementKeepsConfidence: non-identity fields
// may disagree without touching confidence.
func TestConsensus_NonIdentityDisagreementKeepsConfidence(t *testing.T) {
	t.Parallel()

	workers := []WorkerResult{
		okResult(entity.Extraction{
			Brand:        sp("LG"),
			Model:        sp("FHP10"),
			SerialNumber: sp("SN123"),
			Price:        sp("100"),
			Currency:     sp("INR"),
		}),
		okResult(entity.Extraction{
			Brand:        sp("LG"),
			Model:        sp("FHP10"),
			SerialNumber: sp("SN123"),
			Price:        sp("200"),
			Currency:     sp("USD"),
		}),
	}

	got, conf, unresolved := Consensus(workers)

	if conf != 0.9 {
		t.Fatalf("conf = %v, want 0.9", conf)
	}
	if len(unresolved) != 0 {
		t.Fatalf("unresolved = %v, want empty", unresolved)
	}
	if got.Price == nil || *got.Price != "100" {
		t.Errorf("Price = %v, want 100 (first non-nil)", got.Price)
	}
	if got.Currency == nil || *got.Currency != "INR" {
		t.Errorf("Currency = %v, want INR (first non-nil)", got.Currency)
	}
}

// --- Reconciliation + correction guarantees ---

// TestConsensus_BrandCorrection: differing case on a known brand is corrected
// to canonical, not a conflict.
func TestConsensus_BrandCorrection(t *testing.T) {
	t.Parallel()

	workers := []WorkerResult{
		okResult(entity.Extraction{Brand: sp("lg"), Model: sp("FHP10"), SerialNumber: sp("SN123")}),
		okResult(entity.Extraction{Brand: sp("LG"), Model: sp("FHP10"), SerialNumber: sp("SN123")}),
	}

	got, conf, unresolved := Consensus(workers)

	if conf != 0.9 {
		t.Fatalf("conf = %v, want 0.9", conf)
	}
	if len(unresolved) != 0 {
		t.Fatalf("unresolved = %v, want empty", unresolved)
	}
	if got.Brand == nil || *got.Brand != "LG" {
		t.Errorf("Brand = %v, want LG", got.Brand)
	}
}

// TestConsensus_BrandConflict: two different known brands → conflict.
func TestConsensus_BrandConflict(t *testing.T) {
	t.Parallel()

	workers := []WorkerResult{
		okResult(entity.Extraction{Brand: sp("LG"), Model: sp("FHP10"), SerialNumber: sp("SN123")}),
		okResult(entity.Extraction{Brand: sp("Samsung"), Model: sp("FHP10"), SerialNumber: sp("SN123")}),
	}

	got, conf, unresolved := Consensus(workers)

	if conf != 0.5 {
		t.Fatalf("conf = %v, want 0.5", conf)
	}
	if got.Brand != nil {
		t.Errorf("Brand = %v, want nil (conflict)", got.Brand)
	}
	if len(unresolved) != 1 || unresolved[0] != "brand" {
		t.Fatalf("unresolved = %v, want [brand]", unresolved)
	}
}

// TestConsensus_ModelDescriptionReconciled: a whole combined description stored
// in model is re-split.
func TestConsensus_ModelDescriptionReconciled(t *testing.T) {
	t.Parallel()

	workers := []WorkerResult{
		okResult(entity.Extraction{Model: sp("MICRO WAVE OVEN CONVECTION 30BRC2")}),
		okResult(entity.Extraction{Model: sp("MICRO WAVE OVEN CONVECTION 30BRC2")}),
	}

	got, conf, unresolved := Consensus(workers)

	if conf != 0.9 {
		t.Fatalf("conf = %v, want 0.9", conf)
	}
	if len(unresolved) != 0 {
		t.Fatalf("unresolved = %v, want empty", unresolved)
	}
	if got.Model == nil || *got.Model != "30BRC2" {
		t.Errorf("Model = %v, want 30BRC2", got.Model)
	}
	if got.Name == nil || *got.Name != "Microwave Oven" {
		t.Errorf("Name = %v, want Microwave Oven", got.Name)
	}
}

// TestConsensus_NameDescriptionReconciled: a combined description in name with
// no model is re-split, pulling brand out of the name.
func TestConsensus_NameDescriptionReconciled(t *testing.T) {
	t.Parallel()

	workers := []WorkerResult{
		okResult(entity.Extraction{Name: sp("CABINET COOLER MASTER CD600 BLACK")}),
		okResult(entity.Extraction{Name: sp("CABINET COOLER MASTER CD600 BLACK")}),
	}

	got, conf, unresolved := Consensus(workers)

	if conf != 0.9 {
		t.Fatalf("conf = %v, want 0.9", conf)
	}
	if len(unresolved) != 0 {
		t.Fatalf("unresolved = %v, want empty", unresolved)
	}
	if got.Brand == nil || *got.Brand != "Cooler Master" {
		t.Errorf("Brand = %v, want Cooler Master", got.Brand)
	}
	if got.Name == nil || *got.Name != "Cabinet" {
		t.Errorf("Name = %v, want Cabinet", got.Name)
	}
	if got.Model == nil || *got.Model != "CD600" {
		t.Errorf("Model = %v, want CD600", got.Model)
	}
}

// TestConsensus_NilVsValueIsNotConflict: a nil on one worker is not a conflict
// when the other worker has a value.
func TestConsensus_NilVsValueIsNotConflict(t *testing.T) {
	t.Parallel()

	workers := []WorkerResult{
		okResult(entity.Extraction{Brand: sp("LG"), Model: sp("FHP10"), SerialNumber: sp("SN123")}),
		okResult(entity.Extraction{Brand: sp("LG"), Model: sp("FHP10")}),
	}

	got, conf, unresolved := Consensus(workers)

	if conf != 0.9 {
		t.Fatalf("conf = %v, want 0.9", conf)
	}
	if len(unresolved) != 0 {
		t.Fatalf("unresolved = %v, want empty", unresolved)
	}
	if got.SerialNumber == nil || *got.SerialNumber != "SN123" {
		t.Errorf("SerialNumber = %v, want SN123", got.SerialNumber)
	}
}

// TestConsensus_FirstNonNilOrder: non-identity fields take the first non-nil
// value per field, independent across fields.
func TestConsensus_FirstNonNilOrder(t *testing.T) {
	t.Parallel()

	workers := []WorkerResult{
		okResult(entity.Extraction{
			Brand:        sp("LG"),
			Model:        sp("FHP10"),
			SerialNumber: sp("SN123"),
			Price:        nil,
			Currency:     sp("INR"),
		}),
		okResult(entity.Extraction{
			Brand:        sp("LG"),
			Model:        sp("FHP10"),
			SerialNumber: sp("SN123"),
			Price:        sp("100"),
			Currency:     sp("USD"),
		}),
	}

	got, _, _ := Consensus(workers)

	if got.Price == nil || *got.Price != "100" {
		t.Errorf("Price = %v, want 100 (W2's, first non-nil)", got.Price)
	}
	if got.Currency == nil || *got.Currency != "INR" {
		t.Errorf("Currency = %v, want INR (W1's, first non-nil)", got.Currency)
	}
}

// TestConsensus_ThreeWorkers: three workers with two distinct serial values →
// conflict.
func TestConsensus_ThreeWorkers(t *testing.T) {
	t.Parallel()

	workers := []WorkerResult{
		okResult(entity.Extraction{Brand: sp("LG"), Model: sp("FHP10"), SerialNumber: sp("A")}),
		okResult(entity.Extraction{Brand: sp("LG"), Model: sp("FHP10"), SerialNumber: sp("B")}),
		okResult(entity.Extraction{Brand: sp("LG"), Model: sp("FHP10"), SerialNumber: sp("A")}),
	}

	got, conf, unresolved := Consensus(workers)

	if conf != 0.5 {
		t.Fatalf("conf = %v, want 0.5", conf)
	}
	if got.SerialNumber != nil {
		t.Errorf("SerialNumber = %v, want nil (conflict)", got.SerialNumber)
	}
	if len(unresolved) != 1 || unresolved[0] != "serial_number" {
		t.Fatalf("unresolved = %v, want [serial_number]", unresolved)
	}
}

// TestConsensus_MultipleConflicts: brand AND model both conflict.
func TestConsensus_MultipleConflicts(t *testing.T) {
	t.Parallel()

	workers := []WorkerResult{
		okResult(entity.Extraction{Brand: sp("LG"), Model: sp("FHP10"), SerialNumber: sp("SN123")}),
		okResult(entity.Extraction{Brand: sp("Samsung"), Model: sp("FHP20"), SerialNumber: sp("SN123")}),
	}

	got, conf, unresolved := Consensus(workers)

	if conf != 0.5 {
		t.Fatalf("conf = %v, want 0.5", conf)
	}
	if got.Brand != nil {
		t.Errorf("Brand = %v, want nil", got.Brand)
	}
	if got.Model != nil {
		t.Errorf("Model = %v, want nil", got.Model)
	}
	// Assert as a set: contains both "brand" and "model".
	set := map[string]bool{}
	for _, u := range unresolved {
		set[u] = true
	}
	if !set["brand"] || !set["model"] {
		t.Fatalf("unresolved = %v, want to contain brand and model", unresolved)
	}
	if len(unresolved) != 2 {
		t.Fatalf("unresolved = %v, want exactly 2", unresolved)
	}
}

// TestConsensus_UnknownBrandPreserved: an unknown brand is preserved, not
// dropped, so agreement on it is not a conflict.
func TestConsensus_UnknownBrandPreserved(t *testing.T) {
	t.Parallel()

	workers := []WorkerResult{
		okResult(entity.Extraction{Brand: sp("Acme"), Model: sp("FHP10"), SerialNumber: sp("SN123")}),
		okResult(entity.Extraction{Brand: sp("Acme"), Model: sp("FHP10"), SerialNumber: sp("SN123")}),
	}

	got, conf, unresolved := Consensus(workers)

	if conf != 0.9 {
		t.Fatalf("conf = %v, want 0.9", conf)
	}
	if len(unresolved) != 0 {
		t.Fatalf("unresolved = %v, want empty", unresolved)
	}
	if got.Brand == nil || *got.Brand != "Acme" {
		t.Errorf("Brand = %v, want Acme", got.Brand)
	}
}

// TestConsensus_ConfidenceWrittenToExtraction: confidence is written into the
// returned Extraction's Confidence for s==1, s==2 agree, and conflict.
func TestConsensus_ConfidenceWrittenToExtraction(t *testing.T) {
	t.Parallel()

	t.Run("single", func(t *testing.T) {
		t.Parallel()
		workers := []WorkerResult{
			okResult(entity.Extraction{Brand: sp("LG"), Model: sp("FHP10"), SerialNumber: sp("SN123")}),
		}
		got, conf, _ := Consensus(workers)
		if got.Confidence == nil || *got.Confidence != conf || conf != 0.6 {
			t.Errorf("Confidence = %v, conf = %v, want non-nil equal to 0.6", got.Confidence, conf)
		}
	})

	t.Run("agree", func(t *testing.T) {
		t.Parallel()
		workers := []WorkerResult{
			okResult(entity.Extraction{Brand: sp("LG"), Model: sp("FHP10"), SerialNumber: sp("SN123")}),
			okResult(entity.Extraction{Brand: sp("LG"), Model: sp("FHP10"), SerialNumber: sp("SN123")}),
		}
		got, conf, _ := Consensus(workers)
		if got.Confidence == nil || *got.Confidence != conf || conf != 0.9 {
			t.Errorf("Confidence = %v, conf = %v, want non-nil equal to 0.9", got.Confidence, conf)
		}
	})

	t.Run("conflict", func(t *testing.T) {
		t.Parallel()
		workers := []WorkerResult{
			okResult(entity.Extraction{Brand: sp("LG"), Model: sp("FHP10"), SerialNumber: sp("A")}),
			okResult(entity.Extraction{Brand: sp("LG"), Model: sp("FHP10"), SerialNumber: sp("B")}),
		}
		got, conf, _ := Consensus(workers)
		if got.Confidence == nil || *got.Confidence != conf || conf != 0.5 {
			t.Errorf("Confidence = %v, conf = %v, want non-nil equal to 0.5", got.Confidence, conf)
		}
	})
}

// TestConsensus_DateFieldsIndependent: PurchaseDate and WarrantyEnd each take
// the first non-nil value of THEIR OWN field across workers (a worker's
// warranty end must never leak into the purchase date, and vice versa).
func TestConsensus_DateFieldsIndependent(t *testing.T) {
	t.Parallel()

	purchase := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	warranty := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)

	// W1 has only a warranty end; W2 has only a purchase date. Identity
	// fields agree, so the only question is per-field first-non-nil.
	workers := []WorkerResult{
		okResult(entity.Extraction{
			Brand:       sp("LG"),
			Model:       sp("FHP10"),
			WarrantyEnd: &warranty,
		}),
		okResult(entity.Extraction{
			Brand:        sp("LG"),
			Model:        sp("FHP10"),
			PurchaseDate: &purchase,
		}),
	}

	got, conf, unresolved := Consensus(workers)

	if conf != 0.9 {
		t.Fatalf("conf = %v, want 0.9", conf)
	}
	if len(unresolved) != 0 {
		t.Fatalf("unresolved = %v, want empty", unresolved)
	}
	if got.PurchaseDate == nil || !got.PurchaseDate.Equal(purchase) {
		t.Errorf("PurchaseDate = %v, want %v", got.PurchaseDate, purchase)
	}
	if got.WarrantyEnd == nil || !got.WarrantyEnd.Equal(warranty) {
		t.Errorf("WarrantyEnd = %v, want %v", got.WarrantyEnd, warranty)
	}
}
