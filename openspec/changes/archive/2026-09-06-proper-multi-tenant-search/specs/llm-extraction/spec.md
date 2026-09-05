# Spec: llm-extraction (delta)

Modified capability for change `proper-multi-tenant-search`. Adds a single `confidence` value to the structured extraction result so the confidence-gated resolution (see the `confidence-review` capability) can decide whether to auto-commit or hold for review. No existing requirement's behavior changes; this is additive.

## ADDED Requirements

### Requirement: Extraction carries confidence

The extraction result SHALL carry a `confidence` value: a single real number in the closed interval `[0.0, 1.0]` representing the LLM's self-assessed confidence in the extraction, in particular the identity fields (serial number, brand, model) used for resolution. The extraction request SHALL ask the LLM to include a `confidence` field in the structured JSON response. A `confidence` that is absent, non-numeric, or outside `[0.0, 1.0]` SHALL be treated as absent, consistent with the existing rule that fields failing validation are treated as absent — it SHALL NOT be an extraction failure, and the presence or absence of a confidence SHALL NOT cause other valid fields to be dropped. The extracted `confidence` (or its absence) SHALL be carried through to the confidence-gated resolution, which treats an absent confidence as below the review threshold (see the `confidence-review` capability).

#### Scenario: Confident extraction carries its confidence

- **WHEN** the LLM returns a valid extraction with `confidence` `0.92`
- **THEN** the parsed extraction carries `confidence` `0.92`

#### Scenario: Low confidence is carried

- **WHEN** the LLM returns a valid extraction with `confidence` `0.35`
- **THEN** the parsed extraction carries `confidence` `0.35`

#### Scenario: Out-of-range confidence is treated as absent

- **WHEN** the LLM returns `confidence` `1.5` or `-0.2`
- **THEN** the `confidence` is treated as absent (not an extraction failure) and the other valid fields are still used

#### Scenario: Absent confidence is not a failure

- **WHEN** the LLM returns a valid extraction with no `confidence` field
- **THEN** the extraction parses successfully with `confidence` absent, and the other fields are unaffected

#### Scenario: Confidence is not used for identity matching

- **WHEN** an extraction carries a `confidence`
- **THEN** the `confidence` is not part of the normalized identity (it does not participate in serial or brand+model matching); it is carried separately for the confidence gate
