package statement

import (
	"time"

	"procrastinator-backend/commons/entity"
)

// fingerprint is the complete content fingerprint of a line or movement:
// (calendar date, exact amount string, exact normalized description string).
type fingerprint struct {
	day    string
	amount string
	norm   string
}

// Classify assigns a duplicate-detection status to every parsed line of an
// import batch before persistence, so the preview can show the user which
// lines are new and which are duplicates.
//
// It is pure and deterministic: it performs no I/O, reads no clock, and
// contains no tenant logic. The caller pre-scopes existing to the target
// account AND the uploading tenant; the same lines and the same existing
// ledger state always produce the same per-line statuses. Re-importing a
// previously committed statement therefore classifies every line as
// duplicate.
//
// Lines with Status entity.LineStatusError pass through UNCHANGED: an error
// line is never reclassified. Every non-error line (any status, including
// statuses that should not occur in practice) is re-classified by the same
// total rule, in this priority order:
//
//  1. entity.LineStatusDuplicate when EITHER:
//     - the line's non-nil ExternalReference equals the ExternalReference of
//     some existing movement (movements with nil ExternalReference do not
//     participate), OR
//     - within-batch repeat: an EARLIER line (lower position in lines) already
//     has the same non-nil ExternalReference, OR an earlier line has the
//     same complete content fingerprint. The first occurrence keeps the
//     status the priority order gives it on its own merits; each later
//     occurrence is duplicate. (For a fingerprint-only repeat whose first
//     occurrence itself was a possible-duplicate of an existing movement,
//     the later occurrence is duplicate: the within-batch repeat outranks
//     possible-duplicate.)
//  2. entity.LineStatusPossibleDuplicate when (not a duplicate) the line's
//     complete content fingerprint equals the fingerprint of some existing
//     movement, of any origin (manual or import).
//  3. entity.LineStatusValid otherwise.
//
// The complete content fingerprint is the tuple (calendar date of OccurredOn,
// Amount string, NormDescription string), considered complete only when all
// three are non-nil/non-empty on the line. Against an existing movement it is
// calendar-date equality, exact string equality on amount, and exact string
// equality on norm description; a movement participates in fingerprint
// matching only when its NormDescription is non-empty.
//
// Classify returns a new slice; the input slice and its elements are not
// mutated.
func Classify(lines []ParsedLine, existing []entity.MoneyMovement) []ParsedLine {
	out := make([]ParsedLine, len(lines))
	copy(out, lines)

	// Pre-computed existing sets (frozen, caller-scoped).
	existingRefs := make(map[string]struct{}, len(existing))
	existingFingerprints := make(map[fingerprint]struct{}, len(existing))
	for _, mv := range existing {
		if mv.ExternalReference != nil && *mv.ExternalReference != "" {
			existingRefs[*mv.ExternalReference] = struct{}{}
		}
		if mv.NormDescription != "" {
			fp := fingerprint{
				day:    mv.OccurredOn.Format(time.DateOnly),
				amount: mv.Amount,
				norm:   mv.NormDescription,
			}
			existingFingerprints[fp] = struct{}{}
		}
	}

	// Within-batch sets, populated with EARLIER lines only as we walk in
	// order, so a line is never compared against itself or a later line.
	seenRefs := make(map[string]struct{})
	seenFingerprints := make(map[fingerprint]struct{})

	for i, line := range lines {
		if line.Status == entity.LineStatusError {
			continue // error lines pass through unchanged
		}

		var fp *fingerprint
		if line.OccurredOn != nil && line.Amount != nil && *line.Amount != "" &&
			line.NormDescription != nil && *line.NormDescription != "" {
			f := fingerprint{
				day:    line.OccurredOn.Format(time.DateOnly),
				amount: *line.Amount,
				norm:   *line.NormDescription,
			}
			fp = &f
		}

		status := entity.LineStatusValid
		if line.ExternalReference != nil && *line.ExternalReference != "" {
			if _, ok := existingRefs[*line.ExternalReference]; ok {
				status = entity.LineStatusDuplicate
			} else if _, ok := seenRefs[*line.ExternalReference]; ok {
				status = entity.LineStatusDuplicate
			}
		}
		if status != entity.LineStatusDuplicate && fp != nil {
			if _, ok := seenFingerprints[*fp]; ok {
				status = entity.LineStatusDuplicate
			} else if _, ok := existingFingerprints[*fp]; ok {
				status = entity.LineStatusPossibleDuplicate
			}
		}

		out[i].Status = status

		// Record THIS line for later lines, regardless of the status it
		// received: an earlier line participates in within-batch matching
		// no matter what status it ended up with.
		if line.ExternalReference != nil && *line.ExternalReference != "" {
			seenRefs[*line.ExternalReference] = struct{}{}
		}
		if fp != nil {
			seenFingerprints[*fp] = struct{}{}
		}
	}

	return out
}
