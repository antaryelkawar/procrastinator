package statement

import (
	"testing"
	"time"

	"procrastinator-backend/commons/entity"
)

// lineAt builds a valid ParsedLine at the given position with the given
// date, amount, norm description, and optional external reference.
func lineAt(lineRef int, day string, amount string, norm string, ref *string) ParsedLine {
	return ParsedLine{
		LineRef:           lineRef,
		RawLine:           "raw-" + itoa(lineRef),
		OccurredOn:        spTime(time.Date(2026, 8, dayNum(day), 0, 0, 0, 0, time.UTC)),
		Amount:            sp(amount),
		Direction:         DirectionIn,
		Description:       sp(norm),
		NormDescription:   sp(norm),
		ExternalReference: ref,
		Status:            entity.LineStatusValid,
	}
}

// dayNum maps a "DD" day string to an int so the table cases read naturally.
func dayNum(day string) int {
	if len(day) == 2 && day[1] >= '0' && day[1] <= '9' {
		n := int(day[0]-'0')*10 + int(day[1]-'0')
		return n
	}
	return 1
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	digits := []byte{}
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}

// movementAt builds an entity.MoneyMovement with the given date, amount,
// norm description, and optional external reference.
func movementAt(day int, amount string, norm string, ref *string) entity.MoneyMovement {
	return entity.MoneyMovement{
		ID:                "mv-" + itoa(day) + "-" + amount,
		TenantID:          "test-tenant",
		Kind:              entity.KindExpense,
		Amount:            amount,
		Currency:          "INR",
		OccurredOn:        time.Date(2026, 8, day, 12, 30, 0, 0, time.UTC),
		Description:       norm,
		NormDescription:   norm,
		Origin:            entity.OriginImport,
		ExternalReference: ref,
	}
}

// deepCopyLines returns a deep copy of lines so determinism and
// non-mutation tests run against independent inputs.
func deepCopyLines(lines []ParsedLine) []ParsedLine {
	out := make([]ParsedLine, len(lines))
	for i, l := range lines {
		out[i] = l
		if l.OccurredOn != nil {
			v := *l.OccurredOn
			out[i].OccurredOn = &v
		}
		if l.Amount != nil {
			v := *l.Amount
			out[i].Amount = &v
		}
		if l.Description != nil {
			v := *l.Description
			out[i].Description = &v
		}
		if l.NormDescription != nil {
			v := *l.NormDescription
			out[i].NormDescription = &v
		}
		if l.ExternalReference != nil {
			v := *l.ExternalReference
			out[i].ExternalReference = &v
		}
		if l.ErrorReason != nil {
			v := *l.ErrorReason
			out[i].ErrorReason = &v
		}
	}
	return out
}

// linesEqual reports whether two ParsedLine values are equal by dereferencing
// all pointer fields (Go's struct == compares pointer addresses, not values).
func linesEqual(a, b ParsedLine) bool {
	if a.LineRef != b.LineRef || a.RawLine != b.RawLine || a.Direction != b.Direction ||
		a.Status != b.Status {
		return false
	}
	if (a.OccurredOn == nil) != (b.OccurredOn == nil) ||
		(a.Amount == nil) != (b.Amount == nil) ||
		(a.Description == nil) != (b.Description == nil) ||
		(a.NormDescription == nil) != (b.NormDescription == nil) ||
		(a.ExternalReference == nil) != (b.ExternalReference == nil) ||
		(a.ErrorReason == nil) != (b.ErrorReason == nil) {
		return false
	}
	if a.OccurredOn != nil && *a.OccurredOn != *b.OccurredOn {
		return false
	}
	if a.Amount != nil && *a.Amount != *b.Amount {
		return false
	}
	if a.Description != nil && *a.Description != *b.Description {
		return false
	}
	if a.NormDescription != nil && *a.NormDescription != *b.NormDescription {
		return false
	}
	if a.ExternalReference != nil && *a.ExternalReference != *b.ExternalReference {
		return false
	}
	if a.ErrorReason != nil && *a.ErrorReason != *b.ErrorReason {
		return false
	}
	return true
}

func statusSeq(lines []ParsedLine) []string {
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = l.Status
	}
	return out
}

func wantStatuses(t *testing.T, got []ParsedLine, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i].Status != want[i] {
			t.Fatalf("line %d status = %q, want %q (all: %v)", i, got[i].Status, want[i], statusSeq(got))
		}
	}
}

// TestClassifyExternalRefMatchIsDuplicate: a line whose external reference
// exists in the ledger is a duplicate even when date, amount, and
// description all differ.
func TestClassifyExternalRefMatchIsDuplicate(t *testing.T) {
	t.Parallel()

	lines := []ParsedLine{
		lineAt(1, "20", "100.00", "some shop", sp("TXN-123")),
	}
	existing := []entity.MoneyMovement{
		movementAt(1, "999.99", "totally different", sp("TXN-123")),
	}

	got := Classify(lines, existing)
	wantStatuses(t, got, []string{entity.LineStatusDuplicate})
}

// TestClassifyFingerprintMatchIsPossibleDuplicate: same date, amount, and
// normalized description as an existing movement (no ext refs anywhere) is a
// possible duplicate, not a confirmed duplicate.
func TestClassifyFingerprintMatchIsPossibleDuplicate(t *testing.T) {
	t.Parallel()

	lines := []ParsedLine{
		lineAt(1, "20", "1250.50", "reliance digital", nil),
	}
	existing := []entity.MoneyMovement{
		movementAt(20, "1250.50", "reliance digital", nil),
	}

	got := Classify(lines, existing)
	wantStatuses(t, got, []string{entity.LineStatusPossibleDuplicate})
}

// TestClassifyFingerprintMismatchStaysValid: same date but a different amount
// does not match the fingerprint and stays valid.
func TestClassifyFingerprintMismatchStaysValid(t *testing.T) {
	t.Parallel()

	lines := []ParsedLine{
		lineAt(1, "20", "999.99", "reliance digital", nil),
	}
	existing := []entity.MoneyMovement{
		movementAt(20, "1250.50", "reliance digital", nil),
	}

	got := Classify(lines, existing)
	wantStatuses(t, got, []string{entity.LineStatusValid})
}

// TestClassifyNormComparedExactly: NormDescription is already normalized at
// extraction; Classify compares the norm strings exactly. A deliberately
// different norm string (different casing) is NOT a fingerprint match.
func TestClassifyNormComparedExactly(t *testing.T) {
	t.Parallel()

	lines := []ParsedLine{
		lineAt(1, "20", "1250.50", "reliance digital", nil),
	}
	existing := []entity.MoneyMovement{
		movementAt(20, "1250.50", "RELIANCE DIGITAL", nil),
	}

	got := Classify(lines, existing)
	wantStatuses(t, got, []string{entity.LineStatusValid})
}

// TestClassifyWithinBatchRepeatExtRef: two lines sharing an external
// reference with no existing movement: first is valid on its own merits,
// second is duplicate.
func TestClassifyWithinBatchRepeatExtRef(t *testing.T) {
	t.Parallel()

	lines := []ParsedLine{
		lineAt(1, "20", "100.00", "shop a", sp("TXN-77")),
		lineAt(2, "21", "200.00", "shop b", sp("TXN-77")),
	}

	got := Classify(lines, nil)
	wantStatuses(t, got, []string{entity.LineStatusValid, entity.LineStatusDuplicate})
}

// TestClassifyWithinBatchRepeatFingerprint: two lines with identical
// date/amount/norm and no ext refs: first valid, second duplicate.
func TestClassifyWithinBatchRepeatFingerprint(t *testing.T) {
	t.Parallel()

	lines := []ParsedLine{
		lineAt(1, "20", "1250.50", "reliance digital", nil),
		lineAt(2, "20", "1250.50", "reliance digital", nil),
	}

	got := Classify(lines, nil)
	wantStatuses(t, got, []string{entity.LineStatusValid, entity.LineStatusDuplicate})
}

// TestClassifyWithinBatchShadowsExistingFingerprint: existing contains the
// fingerprint; two batch lines share it. First is possible-duplicate on its
// own merits; second is duplicate (within-batch repeat takes priority over
// possible-duplicate).
func TestClassifyWithinBatchShadowsExistingFingerprint(t *testing.T) {
	t.Parallel()

	lines := []ParsedLine{
		lineAt(1, "20", "1250.50", "reliance digital", nil),
		lineAt(2, "20", "1250.50", "reliance digital", nil),
	}
	existing := []entity.MoneyMovement{
		movementAt(20, "1250.50", "reliance digital", nil),
	}

	got := Classify(lines, existing)
	wantStatuses(t, got, []string{entity.LineStatusPossibleDuplicate, entity.LineStatusDuplicate})
}

// TestClassifyReimportOfCommittedSetIsAllDuplicate: re-importing a
// previously committed statement classifies every line as duplicate. The
// committed batch is three lines that a user can re-upload (same ext ref
// repeated on all three, so the first is an exact duplicate on its own
// merits and the rest are within-batch repeats); the ledger holds the
// movements that commit created (ext ref + fingerprint).
func TestClassifyReimportOfCommittedSetIsAllDuplicate(t *testing.T) {
	t.Parallel()

	lines := []ParsedLine{
		lineAt(1, "20", "100.00", "shop a", sp("TXN-1")),
		lineAt(2, "20", "100.00", "shop a", sp("TXN-1")),
		lineAt(3, "20", "100.00", "shop a", nil),
	}
	existing := []entity.MoneyMovement{
		movementAt(20, "100.00", "shop a", sp("TXN-1")),
	}

	got := Classify(lines, existing)
	wantStatuses(t, got, []string{
		entity.LineStatusDuplicate,
		entity.LineStatusDuplicate,
		entity.LineStatusDuplicate,
	})
}

// TestClassifyErrorLinesNotReclassified: an error line passes through
// unchanged, even when its external reference matches an existing movement.
func TestClassifyErrorLinesNotReclassified(t *testing.T) {
	t.Parallel()

	reason := "missing amount"
	errored := ParsedLine{
		LineRef:           1,
		RawLine:           "bad,raw",
		OccurredOn:        spTime(time.Date(2026, 8, 20, 0, 0, 0, 0, time.UTC)),
		ExternalReference: sp("TXN-9"),
		Status:            entity.LineStatusError,
		ErrorReason:       &reason,
	}
	lines := []ParsedLine{errored}
	existing := []entity.MoneyMovement{
		movementAt(20, "100.00", "shop a", sp("TXN-9")),
	}

	got := Classify(lines, existing)
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	g := got[0]
	if !linesEqual(g, errored) { // value equality across every field
		t.Fatalf("error line changed: got %+v, want %+v", g, errored)
	}
}

// TestClassifyNoExistingUniqueLinesAllValid: unique lines against an empty
// ledger stay valid.
func TestClassifyNoExistingUniqueLinesAllValid(t *testing.T) {
	t.Parallel()

	lines := []ParsedLine{
		lineAt(1, "20", "100.00", "shop a", nil),
		lineAt(2, "21", "200.00", "shop b", sp("TXN-1")),
		lineAt(3, "22", "300.00", "shop c", nil),
	}

	got := Classify(lines, nil)
	wantStatuses(t, got, []string{
		entity.LineStatusValid,
		entity.LineStatusValid,
		entity.LineStatusValid,
	})
}

// TestClassifyDeterministic: the same inputs (deep-copied between runs)
// produce the same status sequence.
func TestClassifyDeterministic(t *testing.T) {
	t.Parallel()

	lines := []ParsedLine{
		lineAt(1, "20", "100.00", "shop a", sp("TXN-1")),
		lineAt(2, "21", "200.00", "shop b", nil),
		lineAt(3, "21", "200.00", "shop b", nil),
		lineAt(4, "22", "300.00", "shop c", sp("TXN-2")),
	}
	existing := []entity.MoneyMovement{
		movementAt(20, "100.00", "shop a", sp("TXN-1")),
		movementAt(22, "300.00", "shop c", sp("TXN-2")),
	}

	first := statusSeq(Classify(deepCopyLines(lines), existing))
	second := statusSeq(Classify(deepCopyLines(lines), existing))

	if len(first) != len(second) {
		t.Fatalf("status lengths differ: %v vs %v", first, second)
	}
	for i := range first {
		if first[i] != second[i] {
			t.Fatalf("run %d line %d status = %q, first run = %q", i+1, i, second[i], first[i])
		}
	}
	wantStatuses(t, Classify(deepCopyLines(lines), existing), first)
}

// TestClassifyDoesNotMutateInput: Classify returns a new slice and leaves
// the input slice and its element statuses untouched.
func TestClassifyDoesNotMutateInput(t *testing.T) {
	t.Parallel()

	lines := []ParsedLine{
		lineAt(1, "20", "100.00", "shop a", sp("TXN-1")),
		lineAt(2, "21", "200.00", "shop b", nil),
		lineAt(3, "21", "200.00", "shop b", nil),
	}
	existing := []entity.MoneyMovement{
		movementAt(1, "999.99", "unrelated", sp("TXN-1")),
		movementAt(21, "200.00", "shop b", nil),
	}

	before := deepCopyLines(lines)
	got := Classify(lines, existing)

	// Output must carry the new statuses: line 1 is duplicate via the
	// existing ext ref (which a committed batch would have persisted),
	// line 2 possible-duplicate via the existing fingerprint, line 3
	// duplicate as a within-batch repeat of line 2.
	wantStatuses(t, got, []string{
		entity.LineStatusDuplicate,
		entity.LineStatusPossibleDuplicate,
		entity.LineStatusDuplicate,
	})

	// Input must be untouched: every element still value-equal to before.
	if len(lines) != len(before) {
		t.Fatalf("input slice length changed: %d, want %d", len(lines), len(before))
	}
	for i := range lines {
		if !linesEqual(lines[i], before[i]) {
			t.Fatalf("input line %d mutated: got %+v, want %+v", i, lines[i], before[i])
		}
		if lines[i].Status != entity.LineStatusValid {
			t.Fatalf("input line %d status = %q, want unchanged %q", i, lines[i].Status, entity.LineStatusValid)
		}
	}
}

// TestClassifyLineWithoutExtRefNeverMatchesExistingExtRef: a line with no
// external reference is never a duplicate solely because an existing
// movement has one, and its fingerprint differs anyway.
func TestClassifyLineWithoutExtRefNeverMatchesExistingExtRef(t *testing.T) {
	t.Parallel()

	lines := []ParsedLine{
		lineAt(1, "20", "100.00", "shop a", nil),
	}
	existing := []entity.MoneyMovement{
		movementAt(1, "999.99", "unrelated", sp("TXN-42")),
	}

	got := Classify(lines, existing)
	wantStatuses(t, got, []string{entity.LineStatusValid})
}

// TestClassifyMovementWithoutNormDoesNotParticipateInFingerprint: a
// movement whose NormDescription is empty does not match any line's
// fingerprint, even with the same date and amount.
func TestClassifyMovementWithoutNormDoesNotParticipateInFingerprint(t *testing.T) {
	t.Parallel()

	lines := []ParsedLine{
		lineAt(1, "20", "1250.50", "reliance digital", nil),
	}
	mv := movementAt(20, "1250.50", "", nil)
	mv.Description = ""
	mv.NormDescription = ""
	existing := []entity.MoneyMovement{mv}

	got := Classify(lines, existing)
	wantStatuses(t, got, []string{entity.LineStatusValid})
}
